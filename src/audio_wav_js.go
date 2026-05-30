//go:build js

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Minimal PCM WAV decoder for wasm. The pure-Go beep WAV decoder is excluded
// from the js build (audio_shim_js.go stubs DecodeWav to an error), so SFX
// .snd entries failed readSound and were never stored. This parses RIFF/PCM
// (8/16/24/32-bit int, 32-bit float) into a streamer satisfying StreamSeeker,
// which is enough for readSound to accept the sound and keep its raw bytes.
// Actual audible output goes through the browser (audio_sfx_js.go); this
// decoder exists so the engine's sound table populates and format is known.

type pcmWavStreamer struct {
	samples [][2]float64 // decoded interleaved L/R
	pos     int
}

func (s *pcmWavStreamer) Stream(out [][2]float64) (int, bool) {
	if s.pos >= len(s.samples) {
		return 0, false
	}
	n := copy(out, s.samples[s.pos:])
	s.pos += n
	return n, true
}
func (s *pcmWavStreamer) Err() error      { return nil }
func (s *pcmWavStreamer) Len() int        { return len(s.samples) }
func (s *pcmWavStreamer) Position() int   { return s.pos }
func (s *pcmWavStreamer) Seek(p int) error {
	if p < 0 {
		p = 0
	}
	if p > len(s.samples) {
		p = len(s.samples)
	}
	s.pos = p
	return nil
}

func decodeWavJS(r io.Reader) (StreamSeeker, Format, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, Format{}, err
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, Format{}, fmt.Errorf("not a RIFF/WAVE file")
	}

	var (
		audioFormat   uint16
		numChannels   uint16
		sampleRate    uint32
		bitsPerSample uint16
		dataBytes     []byte
		haveFmt       bool
	)

	// Walk chunks after the 12-byte RIFF header.
	off := 12
	for off+8 <= len(data) {
		id := string(data[off : off+4])
		sz := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		body := off + 8
		if body+sz > len(data) {
			sz = len(data) - body // tolerate truncated final chunk
		}
		switch id {
		case "fmt ":
			if sz < 16 {
				return nil, Format{}, fmt.Errorf("short fmt chunk")
			}
			audioFormat = binary.LittleEndian.Uint16(data[body : body+2])
			numChannels = binary.LittleEndian.Uint16(data[body+2 : body+4])
			sampleRate = binary.LittleEndian.Uint32(data[body+4 : body+8])
			bitsPerSample = binary.LittleEndian.Uint16(data[body+14 : body+16])
			haveFmt = true
		case "data":
			dataBytes = data[body : body+sz]
		}
		// Chunks are word-aligned (pad byte if odd size).
		next := body + sz
		if sz%2 == 1 {
			next++
		}
		off = next
	}
	if !haveFmt || dataBytes == nil {
		return nil, Format{}, fmt.Errorf("missing fmt or data chunk")
	}
	// 1 = PCM int, 3 = IEEE float, 0xFFFE = extensible (assume PCM int).
	if audioFormat != 1 && audioFormat != 3 && audioFormat != 0xFFFE {
		return nil, Format{}, fmt.Errorf("unsupported WAV audio format %d", audioFormat)
	}
	ch := int(numChannels)
	if ch < 1 {
		ch = 1
	}
	bytesPerSample := int(bitsPerSample) / 8
	if bytesPerSample < 1 {
		return nil, Format{}, fmt.Errorf("bad bitsPerSample %d", bitsPerSample)
	}
	frame := bytesPerSample * ch
	if frame == 0 {
		return nil, Format{}, fmt.Errorf("zero frame size")
	}
	nFrames := len(dataBytes) / frame

	readSample := func(b []byte) float64 {
		switch {
		case audioFormat == 3 && bitsPerSample == 32: // float32
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		case bitsPerSample == 8: // unsigned 8-bit
			return (float64(b[0]) - 128) / 128
		case bitsPerSample == 16:
			return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
		case bitsPerSample == 24:
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if v&0x800000 != 0 {
				v |= ^0xFFFFFF // sign-extend
			}
			return float64(v) / 8388608
		case bitsPerSample == 32: // int32
			return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
		}
		return 0
	}

	samples := make([][2]float64, nFrames)
	for i := 0; i < nFrames; i++ {
		base := i * frame
		l := readSample(dataBytes[base : base+bytesPerSample])
		r := l
		if ch >= 2 {
			r = readSample(dataBytes[base+bytesPerSample : base+2*bytesPerSample])
		}
		samples[i] = [2]float64{l, r}
	}

	return &pcmWavStreamer{samples: samples}, Format{
		SampleRate:  SampleRate(sampleRate),
		NumChannels: ch,
		Precision:   bytesPerSample,
	}, nil
}
