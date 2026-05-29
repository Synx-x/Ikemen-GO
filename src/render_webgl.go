//go:build js

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"syscall/js"

	mgl "github.com/go-gl/mathgl/mgl32"
)

type Renderer_WebGL struct {
	name            string
	spriteProgram   js.Value           // WebGL program handle
	vertexBuffer    js.Value           // Quad vertex buffer (position + uv)
	vao             js.Value           // Vertex array object
	currentTexture  *Texture_WebGL     // Bound texture
	currentPalTex   *Texture_WebGL     // Bound palette texture
	blendEnabled    bool
	modelviewLoc    js.Value           // uniform location for modelview
	projectionLoc   js.Value           // uniform location for projection
	texLoc          js.Value           // uniform location for tex
	palTexLoc       js.Value           // uniform location for pal
	x1x2x4x3Loc     js.Value           // uniform location for x1x2x4x3
	tintLoc         js.Value           // uniform location for tint
	addLoc          js.Value           // uniform location for add
	multLoc         js.Value           // uniform location for mult
	alphaPodLoc     js.Value           // uniform location for alpha
	grayLoc         js.Value           // uniform location for gray
	hueLoc          js.Value           // uniform location for hue
	maskLoc         js.Value           // uniform location for mask
	isFlatLoc       js.Value           // uniform location for isFlat
	isRgbaLoc       js.Value           // uniform location for isRgba
	isTrapezLoc     js.Value           // uniform location for isTrapez
	negLoc          js.Value           // uniform location for neg
	shaderReady     bool               // Lazily compiled on first use
}

// Persistent TypedArray buffer pool to avoid per-call allocations.
var (
	tmpArrayBuffer  js.Value
	tmpUint8Array   js.Value
	tmpFloat32Array js.Value
)

func ensureTmpBuffer(byteLen int) {
	if !tmpArrayBuffer.Truthy() || tmpArrayBuffer.Get("byteLength").Int() < byteLen {
		cap := 16
		for cap < byteLen {
			cap *= 2
		}
		tmpArrayBuffer = js.Global().Get("ArrayBuffer").New(cap)
		tmpUint8Array = js.Global().Get("Uint8Array").New(tmpArrayBuffer)
		tmpFloat32Array = js.Global().Get("Float32Array").New(tmpArrayBuffer)
	}
}

// u8FromBytes copies a byte slice into the temp buffer and returns a Uint8Array view.
func u8FromBytes(b []byte) js.Value {
	ensureTmpBuffer(len(b))
	js.CopyBytesToJS(tmpUint8Array, b)
	return tmpUint8Array.Call("subarray", 0, len(b))
}

// f32FromFloats copies float32 values into the temp buffer as little-endian bytes
// and returns a Float32Array view.
func f32FromFloats(f []float32) js.Value {
	byteLen := len(f) * 4
	ensureTmpBuffer(byteLen)
	// Pack floats into bytes as little-endian
	buf := make([]byte, byteLen)
	for i, v := range f {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	js.CopyBytesToJS(tmpUint8Array, buf)
	return tmpFloat32Array.Call("subarray", 0, len(f))
}

type Texture_WebGL struct {
	width  int32
	height int32
	depth  int32
	filter bool
	serial uint64
	handle js.Value // WebGL texture object
}

// WebGL2 format constants for paletted, RGB, RGBA, and HDR textures.
// Mirror native Texture_GL33.MapInternalFormat: depth=8 → RED,
// 24 → RGB, 32 → RGBA, 96 → RGB32F, 128 → RGBA32F. Engine sends
// 1-byte-per-pixel paletted data for depth=8 (most sprites + fonts);
// uploading as RGBA caused texImage2D to reject the buffer.
// WebGL2 requires SIZED internalformat per OpenGL ES 3.0 spec table 8.10.
// Format + type stay unsized. Verified against
// https://registry.khronos.org/webgl/specs/latest/2.0/#3.7.6
const (
	glRED               = 0x1903
	glRGB               = 0x1907
	glRGBA              = 0x1908
	glR8                = 0x8229
	glRGB8              = 0x8051
	glRGBA8             = 0x8058
	glRGB32F            = 0x8815
	glRGBA32F           = 0x8814
	glFLOAT             = 0x1406
	glUNPACK_ALIGNMENT  = 0x0CF5
	glUNPACK_ROW_LENGTH = 0x0CF2
)

func texFormatForDepth(depth int32) (internalFormat, format, dtype int) {
	if depth < 8 {
		depth = 8
	}
	switch depth {
	case 8:
		return glR8, glRED, UNSIGNED_BYTE
	case 24:
		return glRGB8, glRGB, UNSIGNED_BYTE
	case 32:
		return glRGBA8, glRGBA, UNSIGNED_BYTE
	case 96:
		return glRGB32F, glRGB, glFLOAT
	case 128:
		return glRGBA32F, glRGBA, glFLOAT
	}
	return glRGBA8, glRGBA, UNSIGNED_BYTE
}

// Implement Texture interface
func (t *Texture_WebGL) SetData(data []byte) {
	if !t.handle.Truthy() {
		return
	}

	internal, fmt, dtype := texFormatForDepth(t.depth)

	// Pin to unit 0 so prior SetTexture("pal") active-unit state doesn't
	// route this upload into the wrong texture object.
	webglContext.Call("activeTexture", TEXTURE0)
	webglContext.Call("bindTexture", TEXTURE_2D, t.handle)
	webglContext.Call("pixelStorei", glUNPACK_ALIGNMENT, 1)
	webglContext.Call("pixelStorei", glUNPACK_ROW_LENGTH, 0)
	// Disable Y-flip on texture upload. WebGL default is flip=true (top-down),
	// but MUGEN sprites are bottom-up like OpenGL.
	const glUNPACK_FLIP_Y_WEBGL = 0x9240
	webglContext.Call("pixelStorei", glUNPACK_FLIP_Y_WEBGL, 0)

	if len(data) > 0 {
		arr := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(arr, data)
		if t.width == 256 && t.height == 1 {
			for i := 0; i < len(data); i++ {
				arr.SetIndex(i, int(data[i]))
			}
		}
		// Clear any pre-existing GL error before texImage2D
		webglContext.Call("getError")
		webglContext.Call("texImage2D",
			TEXTURE_2D, 0, internal,
			int(t.width), int(t.height), 0,
			fmt, dtype, arr,
		)
		_ = arr // gl.error logging stripped after confirming upload succeeds
	} else {
		webglContext.Call("texImage2D",
			TEXTURE_2D, 0, internal,
			int(t.width), int(t.height), 0,
			fmt, dtype, js.Null(),
		)
	}

	// For paletted (depth=8) textures, MUST use NEAREST to avoid interpolating
	// palette indices. Linear filtering on R8 produces wrong colors.
	filter := NEAREST
	if t.filter && t.depth != 8 {
		filter = LINEAR
	}
	webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_MIN_FILTER, filter)
	webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_MAG_FILTER, filter)
	webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_WRAP_S, CLAMP_TO_EDGE)
	webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_WRAP_T, CLAMP_TO_EDGE)
}

func (t *Texture_WebGL) SetSubData(data []byte, x, y, width, height, stride int32) {
	if !t.handle.Truthy() {
		return
	}
	webglContext.Call("bindTexture", TEXTURE_2D, t.handle)
	uint8Array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(uint8Array, data)
	webglContext.Call(
		"texSubImage2D",
		TEXTURE_2D, 0, int(x), int(y), int(width), int(height),
		RGBA, UNSIGNED_BYTE, uint8Array,
	)
}

func (t *Texture_WebGL) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {
	t.SetData(data)
}

func (t *Texture_WebGL) SetPixelData(data []float32) {
	logConsole("Texture_WebGL.SetPixelData called")
}

func (t *Texture_WebGL) IsValid() bool {
	return t.handle.Truthy()
}

func (t *Texture_WebGL) GetWidth() int32 {
	return t.width
}

func (t *Texture_WebGL) GetHeight() int32 {
	return t.height
}

func (t *Texture_WebGL) CopyData(src *Texture) {
	logConsole("Texture_WebGL.CopyData called")
}

// logConsole was spamming 1000+ msgs/sec from BeginFrame/RenderQuad
// hot paths, killing the chromium tab via OOM. Now gated on
// verboseRender flag; production hot paths stay silent.
var verboseRender = false

func logConsole(msg interface{}) {
	if verboseRender {
		js.Global().Get("console").Call("log", msg)
	}
}

func logConsoleAlways(msg interface{}) {
	js.Global().Get("console").Call("log", msg)
}

// stripVulkanBranch removes the Vulkan-specific code path from a shader.
// The shader uses #if __VERSION__ >= 450 to branch between Vulkan and OpenGL.
// For WebGL (which is OpenGL ES 3.0), we keep only the OpenGL path.
func stripVulkanBranch(src string) string {
	lines := strings.Split(src, "\n")
	var result []string
	inVulkanBranch := false
	depth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect #if __VERSION__ >= 450
		if strings.Contains(trimmed, "#if") && strings.Contains(trimmed, "__VERSION__") && strings.Contains(trimmed, "450") {
			inVulkanBranch = true
			depth++
			continue
		}

		// Detect #else
		if strings.HasPrefix(trimmed, "#else") && inVulkanBranch && depth > 0 {
			inVulkanBranch = false
			continue
		}

		// Detect #endif
		if strings.HasPrefix(trimmed, "#endif") && depth > 0 {
			depth--
			if depth == 0 {
				inVulkanBranch = false
			}
			continue
		}

		// Add line if not in Vulkan branch
		if !inVulkanBranch {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// Renderer_WebGL methods implementing the Renderer interface

func (r *Renderer_WebGL) GetName() string {
	return "WebGL2"
}

func (r *Renderer_WebGL) Init() {
	logConsole("Init: WebGL renderer ready, shader will compile on first use")
	// Safe defaults will be set when shader compiles
}

func (r *Renderer_WebGL) compileShaders() {
	logConsole("compileShaders called: shaderReady=" + js.ValueOf(r.shaderReady).String())
	if r.shaderReady || !webglContext.Truthy() {
		return
	}

	logConsole("compileShaders: starting compilation with embedded shaders")

	// Prefix for WebGL2 GLSL
	const wgl2Prefix = "#version 300 es\nprecision highp float;\nprecision highp int;\n"

	// Strip Vulkan-specific code from shader (lines with #if __VERSION__ >= 450)
	// This shader is designed for both Vulkan (450+) and OpenGL. For WebGL (300 es),
	// we use the OpenGL path by removing the Vulkan branch.
	cleanedVert := stripVulkanBranch(vertShader)
	cleanedFrag := stripVulkanBranch(fragShader)

	// Engine's real shaders — clean baseline.
	_ = cleanedFrag
	_ = cleanedVert

	// Compile vertex shader (embedded from src/shaders/sprite.vert.glsl)
	vertShaderSrc := wgl2Prefix + cleanedVert
	vertShaderObj := webglContext.Call("createShader", VERTEX_SHADER)
	webglContext.Call("shaderSource", vertShaderObj, vertShaderSrc)
	webglContext.Call("compileShader", vertShaderObj)

	vertStatus := webglContext.Call("getShaderParameter", vertShaderObj, COMPILE_STATUS)
	if !vertStatus.Truthy() || !vertStatus.Bool() {
		log := webglContext.Call("getShaderInfoLog", vertShaderObj).String()
		logConsole("SHADER COMPILE ERROR (vertex): " + log)
		logConsoleAlways("SHADER COMPILE ERROR (vertex): " + log)
		return
	}

	// Compile fragment shader (embedded from src/shaders/sprite.frag.glsl)
	fragShaderSrc := wgl2Prefix + cleanedFrag
	fragShaderObj := webglContext.Call("createShader", FRAGMENT_SHADER)
	webglContext.Call("shaderSource", fragShaderObj, fragShaderSrc)
	webglContext.Call("compileShader", fragShaderObj)

	fragStatus := webglContext.Call("getShaderParameter", fragShaderObj, COMPILE_STATUS)
	if !fragStatus.Truthy() || !fragStatus.Bool() {
		log := webglContext.Call("getShaderInfoLog", fragShaderObj).String()
		logConsole("SHADER COMPILE ERROR (fragment): " + log)
		logConsoleAlways("SHADER COMPILE ERROR (fragment): " + log)
		return
	}

	// Link program
	r.spriteProgram = webglContext.Call("createProgram")
	webglContext.Call("attachShader", r.spriteProgram, vertShaderObj)
	webglContext.Call("attachShader", r.spriteProgram, fragShaderObj)
	webglContext.Call("linkProgram", r.spriteProgram)

	linkStatus := webglContext.Call("getProgramParameter", r.spriteProgram, LINK_STATUS)
	if !linkStatus.Truthy() || !linkStatus.Bool() {
		log := webglContext.Call("getProgramInfoLog", r.spriteProgram).String()
		logConsole("PROGRAM LINK ERROR: " + log)
		logConsoleAlways("PROGRAM LINK ERROR: " + log)
		return
	}

	logConsole("compileShaders: shader linked successfully")

	webglContext.Call("deleteShader", vertShaderObj)
	webglContext.Call("deleteShader", fragShaderObj)

	// Get attribute locations
	posLoc := webglContext.Call("getAttribLocation", r.spriteProgram, "position").Int()
	uvLoc := webglContext.Call("getAttribLocation", r.spriteProgram, "uv").Int()

	// Get all uniform locations required by the real engine shaders
	r.modelviewLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "modelview")
	r.projectionLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "projection")
	r.texLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "tex")
	r.palTexLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "pal")
	r.x1x2x4x3Loc = webglContext.Call("getUniformLocation", r.spriteProgram, "x1x2x4x3")
	r.tintLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "tint")
	r.addLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "add")
	r.multLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "mult")
	r.alphaPodLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "alpha")
	r.grayLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "gray")
	r.hueLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "hue")
	r.maskLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "mask")
	r.isFlatLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "isFlat")
	r.isRgbaLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "isRgba")
	r.isTrapezLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "isTrapez")
	r.negLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "neg")

	// Create vertex buffer with a unit quad (4 verts: xy + uv interleaved, stride=16 bytes)
	vertData := js.Global().Get("Float32Array").New(16)
	// Vertex 0: pos=(0,0) uv=(0,0)
	vertData.SetIndex(0, 0.0)
	vertData.SetIndex(1, 0.0)
	vertData.SetIndex(2, 0.0)
	vertData.SetIndex(3, 0.0)
	// Vertex 1: pos=(1,0) uv=(1,0)
	vertData.SetIndex(4, 1.0)
	vertData.SetIndex(5, 0.0)
	vertData.SetIndex(6, 1.0)
	vertData.SetIndex(7, 0.0)
	// Vertex 2: pos=(0,1) uv=(0,1)
	vertData.SetIndex(8, 0.0)
	vertData.SetIndex(9, 1.0)
	vertData.SetIndex(10, 0.0)
	vertData.SetIndex(11, 1.0)
	// Vertex 3: pos=(1,1) uv=(1,1)
	vertData.SetIndex(12, 1.0)
	vertData.SetIndex(13, 1.0)
	vertData.SetIndex(14, 1.0)
	vertData.SetIndex(15, 1.0)

	r.vertexBuffer = webglContext.Call("createBuffer")
	webglContext.Call("bindBuffer", ARRAY_BUFFER, r.vertexBuffer)
	webglContext.Call("bufferData", ARRAY_BUFFER, vertData, STATIC_DRAW)

	// Create VAO
	r.vao = webglContext.Call("createVertexArray")
	webglContext.Call("bindVertexArray", r.vao)
	webglContext.Call("bindBuffer", ARRAY_BUFFER, r.vertexBuffer)

	// Enable and configure position attribute
	webglContext.Call("enableVertexAttribArray", posLoc)
	webglContext.Call("vertexAttribPointer", posLoc, 2, FLOAT, false, 16, 0)

	// Enable and configure uv attribute
	webglContext.Call("enableVertexAttribArray", uvLoc)
	webglContext.Call("vertexAttribPointer", uvLoc, 2, FLOAT, false, 16, 8)

	webglContext.Call("bindVertexArray", js.Null())

	// Set safe defaults for uniforms so uninitialized values don't garbage output
	webglContext.Call("useProgram", r.spriteProgram)

	// Bool uniforms default to false (0)
	if r.isFlatLoc.Truthy() {
		webglContext.Call("uniform1i", r.isFlatLoc, 0)
	}
	if r.isRgbaLoc.Truthy() {
		webglContext.Call("uniform1i", r.isRgbaLoc, 0)
	}
	if r.isTrapezLoc.Truthy() {
		webglContext.Call("uniform1i", r.isTrapezLoc, 0)
	}
	if r.negLoc.Truthy() {
		webglContext.Call("uniform1i", r.negLoc, 0)
	}
	if r.maskLoc.Truthy() {
		webglContext.Call("uniform1i", r.maskLoc, 0)
	}

	// Float uniforms
	if r.alphaPodLoc.Truthy() {
		webglContext.Call("uniform1f", r.alphaPodLoc, 1.0)
	}
	if r.grayLoc.Truthy() {
		webglContext.Call("uniform1f", r.grayLoc, 0.0)
	}
	if r.hueLoc.Truthy() {
		webglContext.Call("uniform1f", r.hueLoc, 0.0)
	}

	// Vector uniforms
	if r.tintLoc.Truthy() {
		webglContext.Call("uniform4f", r.tintLoc, 0.0, 0.0, 0.0, 0.0)
	}
	if r.addLoc.Truthy() {
		webglContext.Call("uniform3f", r.addLoc, 0.0, 0.0, 0.0)
	}
	if r.multLoc.Truthy() {
		webglContext.Call("uniform3f", r.multLoc, 1.0, 1.0, 1.0)
	}
	if r.x1x2x4x3Loc.Truthy() {
		webglContext.Call("uniform4f", r.x1x2x4x3Loc, 0.0, 0.0, 0.0, 0.0)
	}

	r.shaderReady = true
	logConsole("compileShaders: VAO, buffers, and safe defaults ready")
	// Stash refs on window for live diagnostic queries from JS.
	js.Global().Set("__ikemen_program", r.spriteProgram)
	js.Global().Set("__ikemen_vao", r.vao)
	js.Global().Set("__ikemen_vbo", r.vertexBuffer)
	logConsoleAlways(fmt.Sprintf("diag: globals set — progTruthy=%v vaoTruthy=%v vboTruthy=%v",
		r.spriteProgram.Truthy(), r.vao.Truthy(), r.vertexBuffer.Truthy()))
}

func (r *Renderer_WebGL) Close() {
	logConsole("Renderer_WebGL.Close called")
}

func (r *Renderer_WebGL) BeginFrame(clearColor bool) {
	if !webglContext.Truthy() {
		return
	}

	width := webglCanvas.Get("width").Int()
	height := webglCanvas.Get("height").Int()
	webglContext.Call("viewport", 0, 0, width, height)
	// Force disable cull + depth so geometry can't be hidden by them.
	webglContext.Call("disable", CULL_FACE)
	webglContext.Call("disable", DEPTH_TEST)
	webglContext.Call("disable", SCISSOR_TEST)

	// Honor the clearColor flag. Engine calls BeginFrame(false) after
	// motif's storyboard render to PRESERVE the drawn text/menu items
	// for the outer renderFrame loop (motif.go:2774). Unconditional
	// clear wiped menu draws between layers, causing menu items to
	// flash visible then vanish.
	if clearColor {
		webglContext.Call("clearColor", 0.0, 0.0, 0.0, 1.0)
		webglContext.Call("clear", COLOR_BUFFER_BIT)
	}
}

func (r *Renderer_WebGL) EndFrame() {
	// No-op stub for now
}

func (r *Renderer_WebGL) Await() {
	// No-op stub
}

func (r *Renderer_WebGL) IsModelEnabled() bool {
	return false
}

func (r *Renderer_WebGL) IsShadowEnabled() bool {
	return false
}

func (r *Renderer_WebGL) LoadCustomSpriteShader(shaderName string, shaderData []byte) uint32 {
	logConsole("LoadCustomSpriteShader: " + shaderName)
	return 0
}

func (r *Renderer_WebGL) UnloadCustomSpriteShader(shaderName string) {
	logConsole("UnloadCustomSpriteShader: " + shaderName)
}

func (r *Renderer_WebGL) SetSpritePipeline(shaderName string) {
	r.compileShaders()
	if !r.spriteProgram.Truthy() {
		logConsole("SetSpritePipeline: spriteProgram not truthy!")
		return
	}
	webglContext.Call("useProgram", r.spriteProgram)
	webglContext.Call("bindVertexArray", r.vao)

	// Set sampler-unit assignments once. activeTexture state doesn't matter
	// for uniform1i — it just maps the named sampler to a unit. Engine
	// calls SetTexture afterward which sets activeTexture + binds the
	// actual texture object to that unit.
	if r.texLoc.Truthy() {
		webglContext.Call("uniform1i", r.texLoc, 0)
	}
	if r.palTexLoc.Truthy() {
		webglContext.Call("uniform1i", r.palTexLoc, 1)
	}

	// Don't force blendFunc here — engine's EnableBlending sets it per quad
	// with the correct equation+factors. Just ensure blend is on.
	if !r.blendEnabled {
		webglContext.Call("enable", BLEND)
		r.blendEnabled = true
	}
}

func (r *Renderer_WebGL) SetCustomUniforms(params [16]float32) {
	// No-op for basic rendering
}

func (r *Renderer_WebGL) NeedsGrabPass() bool {
	return false
}

func (r *Renderer_WebGL) ResolveBackBuffer() Texture {
	return nil
}

func (r *Renderer_WebGL) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	if !r.blendEnabled {
		webglContext.Call("enable", BLEND)
		r.blendEnabled = true
	}
	webglContext.Call("blendEquation", r.mapBlendEquation(eq))
	srcGLFunc := r.mapBlendFunc(src)
	dstGLFunc := r.mapBlendFunc(dst)
	webglContext.Call("blendFunc", srcGLFunc, dstGLFunc)
}

func (r *Renderer_WebGL) mapBlendEquation(eq BlendEquation) int {
	switch eq {
	case BlendAdd:
		return 0x8006 // FUNC_ADD
	case BlendReverseSubtract:
		return 0x800B // FUNC_REVERSE_SUBTRACT
	default:
		return 0x8006
	}
}

func (r *Renderer_WebGL) DisableBlending() {
	if r.blendEnabled {
		webglContext.Call("disable", BLEND)
		r.blendEnabled = false
	}
}

func (r *Renderer_WebGL) mapBlendFunc(bf BlendFunc) int {
	switch bf {
	case BlendZero:
		return ZERO
	case BlendOne:
		return ONE
	case BlendSrcAlpha:
		return SRC_ALPHA
	case BlendOneMinusSrcAlpha:
		return ONE_MINUS_SRC_ALPHA
	case BlendDstColor:
		return DST_COLOR
	case BlendOneMinusDstColor:
		return ONE_MINUS_DST_COLOR
	default:
		return ONE
	}
}

func (r *Renderer_WebGL) prepareShadowMapPipeline(bufferIndex uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) ReleaseShadowPipeline() {
	// No-op stub
}

func (r *Renderer_WebGL) prepareModelPipeline(bufferIndex uint32, env *Environment) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1, useOutlineAttribute bool, numVertices, vertAttrOffset uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetMeshOutlinePipeline(invertFrontFace bool, meshOutline float32) {
	// No-op stub
}

func (r *Renderer_WebGL) ReleaseModelPipeline() {
	// No-op stub
}

func (r *Renderer_WebGL) newTexture(width, height, depth int32, filter bool) Texture {
	var handle js.Value
	if webglContext.Truthy() {
		webglContext.Call("activeTexture", TEXTURE0)
		handle = webglContext.Call("createTexture")
		webglContext.Call("bindTexture", TEXTURE_2D, handle)

		// Match initial allocation format to the texture's intended depth.
		// Previously this always used unsized RGBA which conflicted with
		// SetData re-spec'ing as RGBA8/R8 — WebGL2 disallows changing the
		// internalformat between texImage2D calls on the same texture.
		initInternal, initFmt, initType := texFormatForDepth(depth)
		webglContext.Call(
			"texImage2D",
			TEXTURE_2D, 0, initInternal,
			int(width), int(height), 0,
			initFmt, initType, js.Null(),
		)

		// Set filters
		glFilter := NEAREST
		if filter {
			glFilter = LINEAR
		}
		webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_MIN_FILTER, glFilter)
		webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_MAG_FILTER, glFilter)
		webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_WRAP_S, CLAMP_TO_EDGE)
		webglContext.Call("texParameteri", TEXTURE_2D, TEXTURE_WRAP_T, CLAMP_TO_EDGE)
	}

	textureSerialNumber++
	return &Texture_WebGL{
		width:  width,
		height: height,
		depth:  depth,
		filter: filter,
		serial: textureSerialNumber,
		handle: handle,
	}
}

func (r *Renderer_WebGL) newPaletteTexture() Texture {
	return r.newTexture(256, 1, 32, false)
}

func (r *Renderer_WebGL) newModelTexture(width, height, depth int32, filter bool) Texture {
	return r.newTexture(width, height, depth, filter)
}

func (r *Renderer_WebGL) newDataTexture(width, height int32) Texture {
	return r.newTexture(width, height, 128, false)
}

func (r *Renderer_WebGL) newHDRTexture(width, height int32) Texture {
	return r.newTexture(width, height, 96, false)
}

func (r *Renderer_WebGL) newCubeMapTexture(widthHeight int32, mipmap bool, lowestMipLevel int32) Texture {
	return r.newTexture(widthHeight, widthHeight, 24, false)
}

func (r *Renderer_WebGL) ReadPixels(data []uint8, width, height int) {
	logConsole("ReadPixels called")
}

func (r *Renderer_WebGL) EnableScissor(x, y, width, height int32) {
	// No-op stub
}

func (r *Renderer_WebGL) DisableScissor() {
	// No-op stub
}

func (r *Renderer_WebGL) SetUniformI(name string, val int) {
	if !r.spriteProgram.Truthy() {
		return
	}
	var loc js.Value
	// Cache frequently-used bool uniforms for faster lookup
	switch name {
	case "isFlat":
		loc = r.isFlatLoc
	case "isRgba":
		loc = r.isRgbaLoc
	case "isTrapez":
		loc = r.isTrapezLoc
	case "neg":
		loc = r.negLoc
	case "mask":
		loc = r.maskLoc
	default:
		loc = webglContext.Call("getUniformLocation", r.spriteProgram, name)
	}
	if loc.Truthy() {
		webglContext.Call("uniform1i", loc, val)
	}
}

func (r *Renderer_WebGL) SetUniformF(name string, values ...float32) {
	if !r.spriteProgram.Truthy() {
		return
	}
	var loc js.Value
	switch name {
	case "alpha":
		loc = r.alphaPodLoc
	case "tint":
		loc = r.tintLoc
	case "add":
		loc = r.addLoc
	case "mult":
		loc = r.multLoc
	case "gray":
		loc = r.grayLoc
	case "hue":
		loc = r.hueLoc
	case "x1x2x4x3":
		loc = r.x1x2x4x3Loc
	default:
		loc = webglContext.Call("getUniformLocation", r.spriteProgram, name)
	}
	if !loc.Truthy() {
		return
	}
	switch len(values) {
	case 1:
		webglContext.Call("uniform1f", loc, values[0])
	case 2:
		webglContext.Call("uniform2f", loc, values[0], values[1])
	case 3:
		webglContext.Call("uniform3f", loc, values[0], values[1], values[2])
	case 4:
		webglContext.Call("uniform4f", loc, values[0], values[1], values[2], values[3])
	}
}

func (r *Renderer_WebGL) SetUniformFv(name string, values []float32) {
	if !r.spriteProgram.Truthy() {
		return
	}
	var loc js.Value
	switch name {
	case "tint":
		loc = r.tintLoc
	case "add":
		loc = r.addLoc
	case "mult":
		loc = r.multLoc
	case "x1x2x4x3":
		loc = r.x1x2x4x3Loc
	default:
		loc = webglContext.Call("getUniformLocation", r.spriteProgram, name)
	}
	if !loc.Truthy() {
		return
	}
	// Use persistent buffer to avoid per-call allocation
	f32Array := f32FromFloats(values)
	switch len(values) {
	case 1:
		webglContext.Call("uniform1fv", loc, f32Array)
	case 2:
		webglContext.Call("uniform2fv", loc, f32Array)
	case 3:
		webglContext.Call("uniform3fv", loc, f32Array)
	case 4:
		webglContext.Call("uniform4fv", loc, f32Array)
	}
}

var firstProjection, firstModelview string

func (r *Renderer_WebGL) SetUniformMatrix(name string, value []float32) {
	if !r.spriteProgram.Truthy() {
		return
	}
	if len(value) >= 16 {
		if name == "projection" {
			copy(lastProjection[:], value[:16])
			if firstProjection == "" {
				firstProjection = fmt.Sprintf("%v", value[:16])
			}
		}
		if name == "modelview" {
			copy(lastModelview[:], value[:16])
			if firstModelview == "" {
				firstModelview = fmt.Sprintf("%v", value[:16])
			}
		}
	}
	var loc js.Value
	switch name {
	case "modelview":
		loc = r.modelviewLoc
	case "projection":
		loc = r.projectionLoc
	default:
		loc = webglContext.Call("getUniformLocation", r.spriteProgram, name)
	}
	if !loc.Truthy() {
		return
	}
	// Use persistent buffer for matrix data
	if len(value) >= 16 {
		f32Array := f32FromFloats(value[:16])
		webglContext.Call("uniformMatrix4fv", loc, false, f32Array)
	}
}

func (r *Renderer_WebGL) SetTexture(name string, tex Texture) {
	if tex == nil {
		return
	}
	t := tex.(*Texture_WebGL)
	if !t.handle.Truthy() {
		return
	}

	unit := 0
	if name == "palTex" || name == "pal" {
		unit = 1
		r.currentPalTex = t
	} else {
		unit = 0
		r.currentTexture = t
	}

	// Bind texture to the correct unit and set sampler uniform to point to it
	webglContext.Call("activeTexture", TEXTURE0+unit)
	webglContext.Call("bindTexture", TEXTURE_2D, t.handle)

	// Set the sampler uniform to point to this texture unit (sprite sampler = 0, palette sampler = 1)
	if name == "palTex" || name == "pal" {
		if r.palTexLoc.Truthy() {
			webglContext.Call("uniform1i", r.palTexLoc, unit)
		}
	} else {
		if r.texLoc.Truthy() {
			webglContext.Call("uniform1i", r.texLoc, unit)
		}
	}
}

func (r *Renderer_WebGL) SetModelUniformI(name string, val int) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelUniformF(name string, values ...float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelUniformFv(name string, values []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelUniformMatrix(name string, value []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelUniformMatrix3(name string, value []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelTexture(name string, t Texture) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapUniformI(name string, val int) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapUniformF(name string, values ...float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapUniformFv(name string, values []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapUniformMatrix(name string, value []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapUniformMatrix3(name string, value []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowMapTexture(name string, t Texture) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowFrameTexture(i uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetShadowFrameCubeTexture(i uint32) {
	// No-op stub
}

// Diagnostic: capture first N SetVertexData calls verbatim so we can verify
// engine actually pushes per-quad geometry rather than reusing one rect.
var vertexCallLog []string
var vertexCallCount int
var setVertexDataTotal int
var lastProjection [16]float32
var lastModelview [16]float32

func (r *Renderer_WebGL) SetVertexData(values ...float32) {
	if !webglContext.Truthy() || !r.vertexBuffer.Truthy() {
		return
	}
	setVertexDataTotal++
	// Capture glyph quad vertex payloads verbatim (first 8). inGlyphDraw
	// set by font.go drawChar. Format: x0,y0,u0,v0, x1,y1,u1,v1, ...
	if inGlyphDraw && len(glyphVertLog) < 8 {
		s := ""
		for i, v := range values {
			if i > 0 {
				s += ","
			}
			s += fmt.Sprintf("%.1f", v)
		}
		s += " P=" + fmt.Sprintf("%v", lastProjection) + " M=" + fmt.Sprintf("%v", lastModelview)
		glyphVertLog = append(glyphVertLog, s)
	}
	// Sample bbox of each quad (every 3rd call) once engine past first
	// 100 calls (skip startup logo/storyboard). Cap log at 200 entries.
	if setVertexDataTotal > 100 && setVertexDataTotal%3 == 0 && vertexCallCount < 200 {
		minX, maxX := values[0], values[0]
		minY, maxY := values[1], values[1]
		for i := 0; i < len(values); i += 4 {
			if values[i] < minX {
				minX = values[i]
			}
			if values[i] > maxX {
				maxX = values[i]
			}
			if values[i+1] < minY {
				minY = values[i+1]
			}
			if values[i+1] > maxY {
				maxY = values[i+1]
			}
		}
		vertexCallLog = append(vertexCallLog, fmt.Sprintf("#%d xy(%.0f..%.0f,%.0f..%.0f)", setVertexDataTotal, minX, maxX, minY, maxY))
		vertexCallCount++
	}
	// Use a FRESH Float32Array per call (avoid persistent-buffer aliasing).
	fresh := js.Global().Get("Float32Array").New(len(values))
	for i, v := range values {
		fresh.SetIndex(i, v)
	}
	webglContext.Call("bindBuffer", ARRAY_BUFFER, r.vertexBuffer)
	webglContext.Call("bufferData", ARRAY_BUFFER, fresh, DYNAMIC_DRAW)

	// Diagnostic: read GPU buffer back to confirm bufferData stuck.
	// Captured into bufferReadback for first 3 calls only.
	if bufferReadbackCount < 3 {
		readback := js.Global().Get("Float32Array").New(len(values))
		webglContext.Call("getBufferSubData", ARRAY_BUFFER, 0, readback)
		s := ""
		for i := 0; i < len(values); i++ {
			if i > 0 {
				s += ","
			}
			s += fmt.Sprintf("%.1f", readback.Index(i).Float())
		}
		bufferReadback = append(bufferReadback, s)
		bufferReadbackCount++
	}
}

var bufferReadback []string
var bufferReadbackCount int

func (r *Renderer_WebGL) SetModelVertexData(bufferIndex uint32, values []byte) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelIndexData(bufferIndex uint32, values ...uint32) {
	// No-op stub
}

// Diagnostic counters; exposed via window.ikemen.drawStats().
var renderQuadCount int
var renderQuadSkipped int

var drawTimeLog []string

func (r *Renderer_WebGL) RenderQuad() {
	if !webglContext.Truthy() || !r.spriteProgram.Truthy() || !r.vao.Truthy() {
		renderQuadSkipped++
		return
	}
	if renderQuadCount > 150 && renderQuadCount%5 == 0 && len(drawTimeLog) < 60 {
		drawTimeLog = append(drawTimeLog, fmt.Sprintf("#%d P0=%.4f P5=%.4f Px=%.2f Py=%.2f M13=%.1f", renderQuadCount, lastProjection[0], lastProjection[5], lastProjection[12], lastProjection[13], lastModelview[13]))
	}
	// Glyph instrumentation: count glyph RenderQuads + read back what the
	// GPU buffer actually holds at glyph draw time (first 8 glyph draws).
	if inGlyphDraw {
		glyphRenderQuadCount++
		if len(glyphQuadReadback) < 4 {
			// Read ACTUAL GPU uniform values at glyph draw (ground truth,
			// not the Go-side mirror). useProgram first so getUniform reads
			// the right program.
			// Read VAO attrib-0 (position) state: which buffer it reads,
			// enabled, size, stride. Compare to r.vertexBuffer.
			webglContext.Call("bindVertexArray", r.vao)
			attrBuf := webglContext.Call("getVertexAttrib", 0, 0x889F) // BUFFER_BINDING
			en0 := webglContext.Call("getVertexAttrib", 0, 0x8622).Bool()  // ARRAY_ENABLED
			sz0 := webglContext.Call("getVertexAttrib", 0, 0x8623).Int()   // ARRAY_SIZE
			st0 := webglContext.Call("getVertexAttrib", 0, 0x8624).Int()   // ARRAY_STRIDE
			en1 := webglContext.Call("getVertexAttrib", 1, 0x8622).Bool()
			sameBuf := attrBuf.Truthy() && attrBuf.Equal(r.vertexBuffer)
			glyphQuadReadback = append(glyphQuadReadback, fmt.Sprintf(
				"attr0bufIsVertexBuffer=%v attr0enabled=%v size=%d stride=%d attr1enabled=%v vaoTruthy=%v",
				sameBuf, en0, sz0, st0, en1, r.vao.Truthy()))
		}
	}
	webglContext.Call("useProgram", r.spriteProgram)
	webglContext.Call("bindVertexArray", r.vao)
	webglContext.Call("drawArrays", 5, 0, 4)
	renderQuadCount++
}

var glyphRenderQuadCount int
var glyphQuadReadback []string

func (r *Renderer_WebGL) RenderElements(mode PrimitiveMode, count, offset int) {
	logConsole("RenderElements called")
}

func (r *Renderer_WebGL) RenderShadowMapElements(mode PrimitiveMode, count, offset int) {
	logConsole("RenderShadowMapElements called")
}

func (r *Renderer_WebGL) RenderCubeMap(envTexture Texture, cubeTexture Texture) {
	logConsole("RenderCubeMap called")
}

func (r *Renderer_WebGL) RenderFilteredCubeMap(distribution int32, cubeTexture Texture, filteredTexture Texture, mipmapLevel, sampleCount int32, roughness float32) {
	logConsole("RenderFilteredCubeMap called")
}

func (r *Renderer_WebGL) RenderLUT(distribution int32, cubeTexture Texture, lutTexture Texture, sampleCount int32) {
	logConsole("RenderLUT called")
}

func (r *Renderer_WebGL) PerspectiveProjectionMatrix(angle, aspect, near, far float32) mgl.Mat4 {
	return mgl.Perspective(angle, aspect, near, far)
}

func (r *Renderer_WebGL) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	return mgl.Ortho(left, right, bottom, top, near, far)
}

func (r *Renderer_WebGL) SetVSync(interval int) {
	// No-op stub
}

func (r *Renderer_WebGL) NewWorkerThread() bool {
	return false
}

// FontRenderer_WebGL is a stub font renderer for wasm.
// Real glyph rendering lands in D5+. For now, all methods are no-ops.
type FontRenderer_WebGL struct{}

// Implement FontRenderer interface
func (f *FontRenderer_WebGL) Init(renderer interface{}) {
	logConsole("FontRenderer_WebGL.Init called")
}

func (f *FontRenderer_WebGL) LoadFont(file string, scale int32, windowWidth int, windowHeight int) (interface{}, error) {
	logConsole("FontRenderer_WebGL.LoadFont: " + file)
	return &Font_WebGL{}, nil
}

// Font_WebGL is a stub font for wasm.
type Font_WebGL struct{}

func (f *Font_WebGL) SetColor(red float32, green float32, blue float32, alpha float32) {
	// No-op stub
}

func (f *Font_WebGL) SetPalFX(neg bool, gray float32, add, mul [3]float32, hue float32) {
	// No-op stub
}

func (f *Font_WebGL) UpdateResolution(windowWidth int, windowHeight int) {
	// No-op stub
}

func (f *Font_WebGL) Printf(x, y float32, xscl, yscl float32, spacingXAdd float32, align int32, blend bool, window [4]int32,
	rxadd float32, rot Rotation, projectionMode int32, fLength float32, rcx, rcy float32,
	fs string, argv ...interface{}) error {
	// No-op stub
	return nil
}

func (f *Font_WebGL) Width(scale float32, spacingXAdd float32, fs string, argv ...interface{}) float32 {
	// Stub returns 0 width for all strings
	return 0
}

// Register the WebGL renderer on init
func init() {
	gfx = &Renderer_WebGL{name: "WebGL2"}
	logConsole("WebGL2 renderer registered")
}
