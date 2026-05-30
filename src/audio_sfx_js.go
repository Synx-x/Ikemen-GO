//go:build js

package main

import (
	"hash/fnv"
	"strconv"
	"syscall/js"
)

// Browser-decoded SFX path. The Go WAV decoder + mixer + speaker are stubbed on
// wasm, so .snd sound effects can't decode or play through the native chain.
// Instead keep the raw WAV bytes at load time and hand them to the harness at
// play time. The harness decodes each WAV once via AudioContext.decodeAudioData,
// caches the AudioBuffer by content hash, and fires a one-shot BufferSource.

// validateSoundJS replaces readSound's Go decode-test on wasm. It accepts the
// raw WAV bytes without decoding (the browser validates at play time) and
// returns a Sound carrying the bytes. handled=true means skip the native path.
func validateSoundJS(wavData []byte) (*Sound, bool) {
	if len(wavData) == 0 {
		return nil, true // empty: treat as handled (disabled), no native decode
	}
	return &Sound{wavData: wavData, format: Format{}, length: 0}, true
}

// sfxBrowserPlay plays a one-shot sound effect through the browser. Returns true
// (handled) on wasm so SoundChannel.Play skips the dead native chain.
func sfxBrowserPlay(wavData []byte, loop int32, freqmul float32) bool {
	if len(wavData) == 0 {
		return true
	}
	if _, ok := sys.cmdFlags["-nosound"]; ok {
		return true
	}
	audio := js.Global().Get("ikemenAudio")
	if !audio.Truthy() || audio.Get("playSfx").IsUndefined() {
		return true // bridge absent: handled (silent), don't run native path
	}

	// Content hash keys the browser-side AudioBuffer cache so each distinct WAV
	// decodes once, not on every play.
	h := fnv.New64a()
	h.Write(wavData)
	key := strconv.FormatUint(h.Sum64(), 36)

	gain := 1.0
	if mv := sys.cfg.Sound.MasterVolume; mv > 0 {
		gain *= float64(mv) / 100.0
	}
	if wv := sys.cfg.Sound.WavVolume; wv > 0 {
		gain *= float64(wv) / 100.0
	}
	if gain < 0 {
		gain = 0
	}
	if gain > 1 {
		gain = 1
	}

	// Pass bytes only when the browser hasn't cached this key yet, to avoid
	// copying every play. hasSfx(key) reports cache state.
	cached := audio.Call("hasSfx", key).Truthy()
	if cached {
		audio.Call("playSfx", key, js.Null(), gain, loop != 0)
	} else {
		u8 := js.Global().Get("Uint8Array").New(len(wavData))
		js.CopyBytesToJS(u8, wavData)
		audio.Call("playSfx", key, u8, gain, loop != 0)
	}
	return true
}
