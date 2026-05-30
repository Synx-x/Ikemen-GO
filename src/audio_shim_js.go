//go:build js

// WASM audio stubs. Types are minimal enough to satisfy sound.go and
// system.go references; methods return errors or no-ops. Real Web Audio
// integration lands in D5+. See audio_shim_native.go for the native path.
package main

import (
	"errors"
	"io"
)

type SampleRate int

type Format struct {
	SampleRate  SampleRate
	NumChannels int
	Precision   int
}

type Streamer interface {
	Stream(samples [][2]float64) (n int, ok bool)
	Err() error
}

type StreamSeeker interface {
	Streamer
	Position() int
	Len() int
	Seek(p int) error
}

type Buffer struct{}

func (b *Buffer) Format() Format          { return Format{} }
func (b *Buffer) Len() int                { return 0 }
func (b *Buffer) Streamer(from, to int) StreamSeeker { return nil }
func (b *Buffer) Append(s Streamer)       {}

type Mixer struct{}

func (m *Mixer) Add(s ...Streamer)                       {}
func (m *Mixer) Clear()                                  {}
func (m *Mixer) Len() int                                { return 0 }
func (m *Mixer) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (m *Mixer) Err() error                              { return nil }

type Ctrl struct {
	Streamer Streamer
	Paused   bool
}

func (c *Ctrl) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (c *Ctrl) Err() error                              { return nil }

type Resampler struct{}

func (r *Resampler) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (r *Resampler) Err() error                              { return nil }
func (r *Resampler) SetRatio(ratio float64)                  {}
func (r *Resampler) Ratio() float64                          { return 1.0 }

type EffectsVolume struct {
	Streamer Streamer
	Base     float64
	Volume   float64
	Silent   bool
}

func (v *EffectsVolume) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (v *EffectsVolume) Err() error                              { return nil }

type MidiSoundFont struct{}

func NewBuffer(f Format) *Buffer                                            { return &Buffer{} }
func Loop(count int, s StreamSeeker) Streamer                               { return nil }
func Resample(quality int, oldRate, newRate SampleRate, s Streamer) *Resampler {
	return &Resampler{}
}
func Take(num int, s Streamer) Streamer { return nil }

var errAudioJS = errors.New("audio decoders not available on js/wasm")

func DecodeVorbis(r io.ReadCloser) (StreamSeeker, Format, error) { return nil, Format{}, errAudioJS }
func DecodeMP3(r io.ReadCloser) (StreamSeeker, Format, error)    { return nil, Format{}, errAudioJS }
func DecodeWav(r io.Reader) (StreamSeeker, Format, error)        { return decodeWavJS(r) }
func DecodeFlac(r io.Reader) (StreamSeeker, Format, error)       { return nil, Format{}, errAudioJS }
func DecodeMidi(r io.ReadCloser, sf *MidiSoundFont, sr SampleRate) (StreamSeeker, Format, error) {
	return nil, Format{}, errAudioJS
}
func NewMidiSoundFont(r io.ReadCloser) (*MidiSoundFont, error) { return nil, errAudioJS }
