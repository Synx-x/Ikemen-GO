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
	// Buffer bits
	COLOR_BUFFER_BIT = 0x4000

	// Data types
	FLOAT        = 0x1406
	UNSIGNED_BYTE = 0x1401

	// Texture constants
	TEXTURE_2D                = 0x0DE1
	TEXTURE_0                 = 0x84C0
	TEXTURE0                  = 0x84C0
	TEXTURE1                  = 0x84C1
	TEXTURE_MIN_FILTER        = 0x2801
	TEXTURE_MAG_FILTER        = 0x2800
	TEXTURE_WRAP_S            = 0x2802
	TEXTURE_WRAP_T            = 0x2803
	NEAREST                   = 0x2600
	LINEAR                    = 0x2601
	CLAMP_TO_EDGE             = 0x812F
	REPEAT                    = 0x2901
	MIRRORED_REPEAT           = 0x8370

	// Texture formats
	RGBA = 0x1908

	// Buffer targets and usage
	ARRAY_BUFFER         = 0x8892
	ELEMENT_ARRAY_BUFFER = 0x8893
	STATIC_DRAW          = 0x88E4
	DYNAMIC_DRAW         = 0x88E8
	STREAM_DRAW          = 0x88E0

	// Shader types
	VERTEX_SHADER   = 0x8B31
	FRAGMENT_SHADER = 0x8B30

	// Shader/program statuses
	COMPILE_STATUS = 0x8B81
	LINK_STATUS    = 0x8B82
	INFO_LOG_LENGTH = 0x8B84

	// Blend functions
	ZERO                = 0
	ONE                 = 1
	SRC_COLOR           = 0x0300
	ONE_MINUS_SRC_COLOR = 0x0301
	DST_COLOR           = 0x0306
	ONE_MINUS_DST_COLOR = 0x0307
	SRC_ALPHA           = 0x0302
	ONE_MINUS_SRC_ALPHA = 0x0303
	DST_ALPHA           = 0x0304
	ONE_MINUS_DST_ALPHA = 0x0305

	// Blend modes
	BLEND = 0x0BE2
)

// setWebGLContext stores the canvas and GL context for use by render_webgl.go methods.
// Called from main_js.go's setCanvas handler.
func setWebGLContext(canvas, gl js.Value) {
	webglCanvas = canvas
	webglContext = gl
}
