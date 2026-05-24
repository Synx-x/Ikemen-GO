//go:build js

package main

import (
	"io"
	"syscall/js"
)

// Log writer implementation
type JsLogWriter struct {
	console_log js.Value
}

func (l JsLogWriter) Write(p []byte) (n int, err error) {
	l.console_log.Invoke(string(p))
	return len(p), nil
}

func NewLogWriter() io.Writer {
	return JsLogWriter{js.Global().Get("console").Get("log")}
}

// Message box implementation using basic JavaScript alert()
var alert = js.Global().Get("alert")

func ShowInfoDialog(message, title string) {
	alert.Invoke(title + "\n\n" + message)
}

func ShowErrorDialog(message string) {
	alert.Invoke("I.K.E.M.E.N Error\n\n" + message)
}

// TTF font loading stub — engine calls this when motif declares
// Type=TrueType. Real impl would rasterize glyphs via golang.org/x/image
// or use the FontFace API through a JS bridge. For v1 just no-op so the
// engine progresses past loadDebugFont. Menus will render any TTF text
// as blank, bitmap fonts still work, which covers the majority of
// motif text rendering.
func LoadFntTtf(f *Fnt, fontfile string, filename string, height int32) {
	// Leave f.Type / f.Size intact so callers don't crash on field reads.
	// Real glyph atlas + texture upload land in D7.
}
