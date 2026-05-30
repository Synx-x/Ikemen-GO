//go:build !js

package main

// Native builds use the real pure-Go decoder + mixer + speaker chain, so the
// browser BGM path is a no-op. Returning false makes bgm.Open fall through to
// its normal Go decode in sound.go.

func bgmBrowserPlay(filename string, loopcount int, bgmVolume int) bool { return false }
func bgmBrowserStop()                                                   {}
func bgmBrowserSetVolume(bgmVolume int)                                 {}
