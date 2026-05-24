//go:build js

package main

import "syscall/js"

// Package-level WebGL context and canvas, set by setCanvas from main_js.go.
// Guarded by .Truthy() checks to safely handle calls before canvas is set.
var (
	webglCanvas  js.Value
	webglContext js.Value
)

// WebGL constants (from Khronos spec https://registry.khronos.org/webgl/specs/latest/2.0/)
const (
	COLOR_BUFFER_BIT = 0x4000
)

// setWebGLContext stores the canvas and GL context for use by render_webgl.go methods.
// Called from main_js.go's setCanvas handler.
func setWebGLContext(canvas, gl js.Value) {
	webglCanvas = canvas
	webglContext = gl
}
