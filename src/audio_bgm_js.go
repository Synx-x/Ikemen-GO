//go:build js

package main

import "syscall/js"

// Browser-decoded BGM path. The pure-Go audio decoders + mixer + speaker are
// stubbed on wasm (audio_shim_js.go, platform_stubs_js.go), so music can't be
// decoded or output through the native chain. Instead hand the raw music bytes
// to the harness, which uses the browser's AudioContext.decodeAudioData (native
// MP3/OGG/WAV/FLAC support) and plays a looping buffer through a gain node.
//
// bgm.Open (sound.go) calls bgmBrowserPlay before its Go decode; on js this
// handles playback and returns true (skipping the dead native path). On native
// the same symbol (audio_bgm_native.go) returns false.

func bgmBrowserPlay(filename string, loopcount int, bgmVolume int) bool {
	// Respect the engine's mute flags.
	if _, ok := sys.cmdFlags["-nomusic"]; ok {
		return true
	}
	if _, ok := sys.cmdFlags["-nosound"]; ok {
		return true
	}
	audio := js.Global().Get("ikemenAudio")
	if !audio.Truthy() {
		// Harness audio bridge absent: nothing can play. Report handled so the
		// caller skips the (silent) native decode path.
		return true
	}
	data, err := engineReadFile(filename)
	if err != nil || len(data) == 0 {
		return true
	}
	u8 := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(u8, data)

	gain := float64(bgmVolume) / 100.0
	if mv := sys.cfg.Sound.MasterVolume; mv > 0 {
		gain *= float64(mv) / 100.0
	}
	if bv := sys.cfg.Sound.BGMVolume; bv > 0 {
		gain *= float64(bv) / 100.0
	}
	if gain < 0 {
		gain = 0
	}
	if gain > 1 {
		gain = 1
	}
	audio.Call("playBgm", u8, loopcount != 0, gain)
	return true
}

func bgmBrowserStop() {
	audio := js.Global().Get("ikemenAudio")
	if audio.Truthy() {
		audio.Call("stopBgm")
	}
}

// bgmBrowserSetVolume updates the playing BGM gain live (engine volume changes).
func bgmBrowserSetVolume(bgmVolume int) {
	audio := js.Global().Get("ikemenAudio")
	if !audio.Truthy() {
		return
	}
	gain := float64(bgmVolume) / 100.0
	if mv := sys.cfg.Sound.MasterVolume; mv > 0 {
		gain *= float64(mv) / 100.0
	}
	if bv := sys.cfg.Sound.BGMVolume; bv > 0 {
		gain *= float64(bv) / 100.0
	}
	if gain < 0 {
		gain = 0
	}
	if gain > 1 {
		gain = 1
	}
	audio.Call("setBgmVolume", gain)
}
