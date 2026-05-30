//go:build js

package main

import (
	"fmt"
	"syscall/js"

	mgl "github.com/go-gl/mathgl/mgl32"
)

// Font_Canvas2D is the wasm TrueType renderer. The native TTF path
// (font_gl33.go etc, //go:build !js) rasterizes glyphs with freetype; on
// wasm that's unavailable, so menu/UI text using TrueType fonts rendered
// blank (DrawTtf no-op'd because f.ttf was nil). This implementation
// rasterizes each string with the browser Canvas 2D API, uploads the
// result as an RGBA texture, and draws it as a textured quad through the
// existing Renderer_WebGL sprite path. It satisfies the TtfFont interface
// (font.go:64).
type Font_Canvas2D struct {
	pxHeight   int     // nominal glyph height in game pixels
	family     string  // CSS font-family used for rasterization
	r, g, b, a float32 // current color (0..1), set via SetColor
	winW, winH int     // resolution from UpdateResolution

	// Offscreen 2D canvas + context, created lazily and reused.
	canvas js.Value
	ctx    js.Value

	// One reusable GPU texture; re-spec'd per draw with the text bitmap.
	tex    *Texture_WebGL
	texW   int
	texH   int
}

// newFontCanvas2D builds a renderer for a given nominal pixel height.
func newFontCanvas2D(pxHeight int32) *Font_Canvas2D {
	doc := js.Global().Get("document")
	cv := doc.Call("createElement", "canvas")
	ctx := cv.Call("getContext", "2d", map[string]interface{}{"willReadFrequently": true})
	h := int(pxHeight)
	if h <= 0 {
		h = 24
	}
	return &Font_Canvas2D{
		pxHeight: h,
		family:   "sans-serif",
		r:        1, g: 1, b: 1, a: 1,
		canvas:   cv,
		ctx:      ctx,
	}
}

func (f *Font_Canvas2D) SetColor(red, green, blue, alpha float32) {
	f.r, f.g, f.b, f.a = red, green, blue, alpha
}

// SetPalFX is a no-op for the Canvas2D path — color is applied directly
// during rasterization via SetColor. PalFX (neg/gray/add/mul/hue) is not
// modeled for wasm UI text in this first pass.
func (f *Font_Canvas2D) SetPalFX(neg bool, gray float32, add, mul [3]float32, hue float32) {
}

func (f *Font_Canvas2D) UpdateResolution(windowWidth int, windowHeight int) {
	f.winW, f.winH = windowWidth, windowHeight
}

// measure returns the rendered pixel width of txt at the nominal height.
func (f *Font_Canvas2D) measure(txt string) float32 {
	f.ctx.Set("font", fmt.Sprintf("%dpx %s", f.pxHeight, f.family))
	m := f.ctx.Call("measureText", txt)
	return float32(m.Get("width").Float())
}

func (f *Font_Canvas2D) Width(scale float32, spacingXAdd float32, fs string, argv ...interface{}) float32 {
	txt := fmt.Sprintf(fs, argv...)
	return f.measure(txt) * scale
}

// Printf rasterizes txt to the offscreen canvas, uploads it, and draws a
// textured quad. x,y are the text anchor in game/screen space (same space
// sprites use: origin top-left, y increases downward). xscl/yscl scale the
// rendered bitmap. align: -1 left, 0 center, 1 right (matches engine).
func (f *Font_Canvas2D) Printf(x, y float32, xscl, yscl float32, spacingXAdd float32,
	align int32, blend bool, window [4]int32, rxadd float32, rot Rotation,
	projectionMode int32, fLength float32, rcx, rcy float32, fs string, argv ...interface{}) error {

	if !webglContext.Truthy() {
		return nil
	}
	txt := fmt.Sprintf(fs, argv...)
	if len(txt) == 0 {
		return nil
	}

	font := fmt.Sprintf("%dpx %s", f.pxHeight, f.family)
	f.ctx.Set("font", font)
	wpx := int(f.ctx.Call("measureText", txt).Get("width").Float())
	if wpx <= 0 {
		return nil
	}
	// Pad height a little so descenders/ascenders aren't clipped.
	hpx := f.pxHeight + f.pxHeight/3
	if hpx <= 0 {
		return nil
	}

	// Size the canvas (only when it grew) and clear it transparent.
	if f.canvas.Get("width").Int() < wpx {
		f.canvas.Set("width", wpx)
	}
	if f.canvas.Get("height").Int() < hpx {
		f.canvas.Set("height", hpx)
	}
	f.ctx.Call("clearRect", 0, 0, wpx, hpx)
	// Re-set font after any resize (resize resets 2D context state).
	f.ctx.Set("font", font)
	f.ctx.Set("textBaseline", "top")
	f.ctx.Set("fillStyle", fmt.Sprintf("rgba(%d,%d,%d,%f)",
		int(clamp01(f.r)*255), int(clamp01(f.g)*255), int(clamp01(f.b)*255), clamp01(f.a)))
	f.ctx.Call("fillText", txt, 0, 0)

	// Pull the RGBA bytes out of the canvas (straight, non-premultiplied).
	imgData := f.ctx.Call("getImageData", 0, 0, wpx, hpx)
	jsBuf := imgData.Get("data") // Uint8ClampedArray
	n := jsBuf.Get("length").Int()
	pix := make([]byte, n)
	js.CopyBytesToGo(pix, jsBuf)

	// Upload to a reusable RGBA texture.
	if f.tex == nil {
		f.tex = gfx.newTexture(int32(wpx), int32(hpx), 32, true).(*Texture_WebGL)
	} else {
		f.tex.width = int32(wpx)
		f.tex.height = int32(hpx)
	}
	f.tex.SetData(pix)

	// Place the quad. align shifts the anchor horizontally.
	w := float32(wpx) * xscl
	h := float32(hpx) * yscl
	dx := x
	switch align {
	case 0: // center
		dx = x - w/2
	case 1: // right
		dx = x - w
	}
	rect := [4]float32{dx, y, w, h}

	// Project in GAME space (0..gameWidth, 0..gameHeight), not screen/scrrect
	// space. DrawTtf passes x,y in game coords (like the bitmap font path), and
	// the text bitmap is rasterized at game-pixel height. Native Font_GL33.Printf
	// maps glyphs with a resolution=gameWidth/gameHeight uniform; using scrrect
	// (1280x720) here instead shrank the text and shoved it left of its
	// container. The GL viewport stretches game space to fill the canvas.
	gw, gh := float32(sys.gameWidth), float32(sys.gameHeight)
	if gw <= 0 {
		gw = float32(sys.scrrect[2])
	}
	if gh <= 0 {
		gh = float32(sys.scrrect[3])
	}
	modelview := mgl.Translate3D(0, gh, 0)
	proj := gfx.OrthographicProjectionMatrix(0, gw, 0, gh, -65535, 65535)

	x1, y1 := rect[0], -rect[1]
	x2, y2 := rect[0]+rect[2], -(rect[1] + rect[3])

	gfx.SetSpritePipeline("")
	gfx.SetTexture("tex", f.tex)
	gfx.SetUniformMatrix("modelview", modelview[:])
	gfx.SetUniformMatrix("projection", proj[:])
	gfx.SetUniformI("isFlat", 0)
	gfx.SetUniformI("isRgba", 1)
	gfx.SetUniformI("isTrapez", 0)
	gfx.SetUniformI("mask", -1) // keep texture alpha
	gfx.SetUniformI("neg", 0)
	gfx.SetUniformF("gray", 0)
	gfx.SetUniformF("hue", 0)
	gfx.SetUniformF("alpha", 1.0)
	gfx.SetUniformFv("tint", []float32{0, 0, 0, 0})
	gfx.SetUniformFv("add", []float32{0, 0, 0})
	gfx.SetUniformFv("mult", []float32{1, 1, 1})
	// Canvas2D ImageData is straight (non-premultiplied) alpha → standard
	// SRC_ALPHA / ONE_MINUS_SRC_ALPHA blending.
	gfx.EnableBlending(BlendAdd, BlendSrcAlpha, BlendOneMinusSrcAlpha)
	gfx.SetVertexData(
		x2, y2, 1, 1,
		x2, y1, 1, 0,
		x1, y2, 0, 1,
		x1, y1, 0, 0,
	)
	gfx.RenderQuad()

	if len(ttfLog) < 16 && len(txt) > 0 && len(txt) < 20 && txt[0] != '*' && txt[0] != 'W' {
		// Count non-transparent pixels in the rasterized bitmap to confirm
		// the canvas actually drew glyphs (not blank due to color/alpha).
		nonZero := 0
		for i := 3; i < len(pix); i += 4 {
			if pix[i] != 0 {
				nonZero++
			}
		}
		ttfLog = append(ttfLog, fmt.Sprintf("'%s' rgba(%.2f,%.2f,%.2f,%.2f) %dx%d rect(%.0f,%.0f,%.0f,%.0f) nzAlpha=%d",
			txt, f.r, f.g, f.b, f.a, wpx, hpx, rect[0], rect[1], rect[2], rect[3], nonZero))
	}
	return nil
}

var ttfLog []string

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
