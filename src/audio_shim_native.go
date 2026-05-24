//go:build !js

// Native audio shim: re-exports beep types and decoder functions so
// sound.go and system.go can reference platform-neutral names. The js
// sibling (audio_shim_js.go) provides stub implementations that return
// errors, since beep transitively pulls mewkiz/pkg/term which uses
// syscall.SYS_IOCTL — undefined on js/wasm.
package main

import (
	"io"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/midi"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

type Buffer = beep.Buffer
type Ctrl = beep.Ctrl
type Format = beep.Format
type Mixer = beep.Mixer
type Resampler = beep.Resampler
type SampleRate = beep.SampleRate
type Streamer = beep.Streamer
type StreamSeeker = beep.StreamSeeker

type EffectsVolume = effects.Volume
type MidiSoundFont = midi.SoundFont

func NewBuffer(f Format) *Buffer                                      { return beep.NewBuffer(f) }
func Loop(count int, s StreamSeeker) Streamer                         { return beep.Loop(count, s) }
func Resample(quality int, old, new SampleRate, s Streamer) *Resampler { return beep.Resample(quality, old, new, s) }
func Take(num int, s Streamer) Streamer                               { return beep.Take(num, s) }

func DecodeVorbis(r io.ReadCloser) (StreamSeeker, Format, error) { return vorbis.Decode(r) }
func DecodeMP3(r io.ReadCloser) (StreamSeeker, Format, error)    { return mp3.Decode(r) }
func DecodeWav(r io.Reader) (StreamSeeker, Format, error)        { return wav.Decode(r) }
func DecodeFlac(r io.Reader) (StreamSeeker, Format, error)       { return flac.Decode(r) }
func DecodeMidi(r io.ReadCloser, sf *MidiSoundFont, sr SampleRate) (StreamSeeker, Format, error) {
	return midi.Decode(r, sf, sr)
}
func NewMidiSoundFont(r io.ReadCloser) (*MidiSoundFont, error) { return midi.NewSoundFont(r) }
