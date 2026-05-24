//go:build !js

// Re-exports SDL keymod constants so input.go can reference them
// without importing go-sdl2 directly. Native side aliases real SDL;
// js side provides matching constants.
package main

import "github.com/veandco/go-sdl2/sdl"

type sdlKeymod = sdl.Keymod

const (
	sdlKMOD_GUI   = sdl.KMOD_GUI
	sdlKMOD_CTRL  = sdl.KMOD_CTRL
	sdlKMOD_ALT   = sdl.KMOD_ALT
	sdlKMOD_SHIFT = sdl.KMOD_SHIFT
)
