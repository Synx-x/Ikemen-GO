//go:build js

package main

import "syscall/js"

// Browser-played SFX. The Go mixer/speaker is stubbed on wasm, so sound
// effects (decoded into Sound.wavData by decodeWavJS) never reach output
// through the native chain. Instead each distinct Sound's raw WAV bytes are
// handed to the harness once, the browser decodes + caches an AudioBuffer
// keyed by a serial id, and each play spawns a fresh BufferSource (polyphonic,
// low-latency). SoundChannel.Play (sound.go) calls sfxBrowserPlay on js.

var sfxNextID int
var sfxUploaded = map[*Sound]int{} // Sound -> browser cache id (uploaded once)

// sfxBrowserPlay uploads the sound's WAV bytes to the harness on first use,
// then triggers a one-shot play at the given volume (0..512 engine scale) and
// pan (-1..1). Returns true if handled on js (caller skips the dead native
// mixer path). volScale is the engine's 0..256-ish channel volume.
func sfxBrowserPlay(sound *Sound, volume float32, pan float32, loop bool) bool {
	if sound == nil {
		return true
	}
	if _, ok := sys.cmdFlags["-nosound"]; ok {
		return true
	}
	audio := js.Global().Get("ikemenAudio")
	if !audio.Truthy() || len(sound.wavData) == 0 {
		return true
	}

	id, uploaded := sfxUploaded[sound]
	if !uploaded {
		sfxNextID++
		id = sfxNextID
		sfxUploaded[sound] = id
		u8 := js.Global().Get("Uint8Array").New(len(sound.wavData))
		js.CopyBytesToJS(u8, sound.wavData)
		audio.Call("uploadSfx", id, u8)
	}

	// Engine channel volume is ~256 nominal; scale to 0..1 then apply
	// WavVolume + MasterVolume like the native Normalizer does.
	gain := float64(volume) / 256.0
	if wv := sys.cfg.Sound.WavVolume; wv > 0 {
		gain *= float64(wv) / 100.0
	}
	if mv := sys.cfg.Sound.MasterVolume; mv > 0 {
		gain *= float64(mv) / 100.0
	}
	if gain < 0 {
		gain = 0
	}
	if gain > 1.5 {
		gain = 1.5
	}
	audio.Call("playSfx", id, gain, float64(pan), loop)
	return true
}
