//go:build !js

package main

// Native builds use the real Go WAV decoder + mixer + speaker, so the browser
// SFX path is inert. validateSoundJS returns handled=false so readSound runs its
// normal decode-test; sfxBrowserPlay returns false so SoundChannel.Play uses the
// native streamer chain.

func validateSoundJS(wavData []byte) (*Sound, bool) { return nil, false }

func sfxBrowserPlay(wavData []byte, loop int32, freqmul float32) bool { return false }
