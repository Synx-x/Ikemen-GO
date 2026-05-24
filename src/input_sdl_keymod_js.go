//go:build js

// JS-side stubs for the SDL keymod constants input.go references. Values
// mirror SDL's keymod bit assignments so callers can still do bitwise
// math, even though no actual keyboard modifier source feeds them yet.
package main

type sdlKeymod uint16

const (
	sdlKMOD_LSHIFT sdlKeymod = 0x0001
	sdlKMOD_RSHIFT sdlKeymod = 0x0002
	sdlKMOD_LCTRL  sdlKeymod = 0x0040
	sdlKMOD_RCTRL  sdlKeymod = 0x0080
	sdlKMOD_LALT   sdlKeymod = 0x0100
	sdlKMOD_RALT   sdlKeymod = 0x0200
	sdlKMOD_LGUI   sdlKeymod = 0x0400
	sdlKMOD_RGUI   sdlKeymod = 0x0800

	sdlKMOD_SHIFT = sdlKMOD_LSHIFT | sdlKMOD_RSHIFT
	sdlKMOD_CTRL  = sdlKMOD_LCTRL | sdlKMOD_RCTRL
	sdlKMOD_ALT   = sdlKMOD_LALT | sdlKMOD_RALT
	sdlKMOD_GUI   = sdlKMOD_LGUI | sdlKMOD_RGUI
)
