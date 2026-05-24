//go:build js

package main

import (
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
	texLoc          js.Value           // uniform location for tex
	palTexLoc       js.Value           // uniform location for palTex
	shaderReady     bool               // Lazily compiled on first use
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

	webglContext.Call("bindTexture", TEXTURE_2D, t.handle)
	webglContext.Call("pixelStorei", glUNPACK_ALIGNMENT, 1)
	webglContext.Call("pixelStorei", glUNPACK_ROW_LENGTH, 0)

	if len(data) > 0 {
		var arr js.Value
		if dtype == glFLOAT {
			// Floats arrive as []byte (4 bytes per float). View as
			// Float32Array of len(data)/4 elements.
			arr = js.Global().Get("Uint8Array").New(len(data))
			js.CopyBytesToJS(arr, data)
		} else {
			arr = js.Global().Get("Uint8Array").New(len(data))
			js.CopyBytesToJS(arr, data)
		}
		webglContext.Call("texImage2D",
			TEXTURE_2D, 0, internal,
			int(t.width), int(t.height), 0,
			fmt, dtype, arr,
		)
	} else {
		webglContext.Call("texImage2D",
			TEXTURE_2D, 0, internal,
			int(t.width), int(t.height), 0,
			fmt, dtype, js.Null(),
		)
	}

	filter := NEAREST
	if t.filter {
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

// Renderer_WebGL methods implementing the Renderer interface

func (r *Renderer_WebGL) GetName() string {
	return "WebGL2"
}

func (r *Renderer_WebGL) Init() {
	logConsole("Init: WebGL renderer ready, shader will compile on first use")
}

func (r *Renderer_WebGL) compileShaders() {
	logConsole("compileShaders called: shaderReady=" + js.ValueOf(r.shaderReady).String())
	if r.shaderReady || !webglContext.Truthy() {
		return
	}

	logConsole("compileShaders: starting compilation")

	// Compile vertex shader
	vertShader := webglContext.Call("createShader", VERTEX_SHADER)
	webglContext.Call("shaderSource", vertShader, `
		#version 300 es
		precision highp float;
		in vec2 position;
		in vec2 uv;
		uniform mat4 modelview;
		out vec2 vUV;
		void main() {
			gl_Position = modelview * vec4(position, 0.0, 1.0);
			vUV = uv;
		}
	`)
	webglContext.Call("compileShader", vertShader)

	if !webglContext.Call("getShaderParameter", vertShader, COMPILE_STATUS).Bool() {
		log := webglContext.Call("getShaderInfoLog", vertShader).String()
		logConsole("Vertex shader compile error: " + log)
		return
	}

	// Compile fragment shader
	fragShader := webglContext.Call("createShader", FRAGMENT_SHADER)
	webglContext.Call("shaderSource", fragShader, `
		#version 300 es
		precision highp float;
		in vec2 vUV;
		uniform sampler2D tex;
		uniform sampler2D palTex;
		out vec4 outColor;
		void main() {
			vec4 color = texture(tex, vUV);
			outColor = color;
		}
	`)
	webglContext.Call("compileShader", fragShader)

	if !webglContext.Call("getShaderParameter", fragShader, COMPILE_STATUS).Bool() {
		log := webglContext.Call("getShaderInfoLog", fragShader).String()
		logConsole("Fragment shader compile error: " + log)
		return
	}

	// Link program
	r.spriteProgram = webglContext.Call("createProgram")
	webglContext.Call("attachShader", r.spriteProgram, vertShader)
	webglContext.Call("attachShader", r.spriteProgram, fragShader)
	webglContext.Call("linkProgram", r.spriteProgram)

	if !webglContext.Call("getProgramParameter", r.spriteProgram, LINK_STATUS).Bool() {
		log := webglContext.Call("getProgramInfoLog", r.spriteProgram).String()
		logConsole("Program link error: " + log)
		return
	}

	logConsole("compileShaders: shader linked successfully")

	webglContext.Call("deleteShader", vertShader)
	webglContext.Call("deleteShader", fragShader)

	// Get attribute and uniform locations
	posLoc := webglContext.Call("getAttribLocation", r.spriteProgram, "position").Int()
	uvLoc := webglContext.Call("getAttribLocation", r.spriteProgram, "uv").Int()
	r.modelviewLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "modelview")
	r.texLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "tex")
	r.palTexLoc = webglContext.Call("getUniformLocation", r.spriteProgram, "palTex")

	// Create vertex buffer with a unit quad (4 verts: xy + uv interleaved)
	vertData := js.Global().Get("Float32Array").New(16)
	// Vertex 0: (0,0) uv (0,1)
	vertData.SetIndex(0, 0.0)
	vertData.SetIndex(1, 0.0)
	vertData.SetIndex(2, 0.0)
	vertData.SetIndex(3, 1.0)
	// Vertex 1: (1,0) uv (1,1)
	vertData.SetIndex(4, 1.0)
	vertData.SetIndex(5, 0.0)
	vertData.SetIndex(6, 1.0)
	vertData.SetIndex(7, 1.0)
	// Vertex 2: (0,1) uv (0,0)
	vertData.SetIndex(8, 0.0)
	vertData.SetIndex(9, 1.0)
	vertData.SetIndex(10, 0.0)
	vertData.SetIndex(11, 0.0)
	// Vertex 3: (1,1) uv (1,0)
	vertData.SetIndex(12, 1.0)
	vertData.SetIndex(13, 1.0)
	vertData.SetIndex(14, 1.0)
	vertData.SetIndex(15, 0.0)

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

	r.shaderReady = true
	logConsole("compileShaders: VAO and buffers ready")
}

func (r *Renderer_WebGL) Close() {
	logConsole("Renderer_WebGL.Close called")
}

func (r *Renderer_WebGL) BeginFrame(clearColor bool) {
	// Guard: if canvas not yet set via setCanvas(), no-op safely.
	if !webglContext.Truthy() {
		logConsole("BeginFrame: no webglContext yet, skipping")
		return
	}

	// Get canvas dimensions
	width := webglCanvas.Get("width").Int()
	height := webglCanvas.Get("height").Int()

	logConsole("BeginFrame: viewport " + js.ValueOf(width).String() + "x" + js.ValueOf(height).String())

	// Set viewport to match canvas
	webglContext.Call("viewport", 0, 0, width, height)

	// Set clear color to dark warm brown #2a1e14 (0.165, 0.118, 0.078, 1.0)
	webglContext.Call("clearColor", 0.165, 0.118, 0.078, 1.0)
	logConsole("BeginFrame: clearColor set")

	// Clear the color buffer
	webglContext.Call("clear", COLOR_BUFFER_BIT)
	logConsole("BeginFrame: clear called")

	// Check for GL errors
	err := webglContext.Call("getError").Int()
	if err != 0 {
		logConsole("BeginFrame: GL error " + js.ValueOf(err).String())
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
	logConsole("SetSpritePipeline called, about to compile")
	r.compileShaders()
	logConsole("SetSpritePipeline: shaderReady now " + js.ValueOf(r.shaderReady).String())
	if r.spriteProgram.Truthy() {
		logConsole("SetSpritePipeline: using program")
		webglContext.Call("useProgram", r.spriteProgram)
		webglContext.Call("bindVertexArray", r.vao)
	} else {
		logConsole("SetSpritePipeline: spriteProgram not truthy!")
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
	// Map blend funcs
	srcGLFunc := r.mapBlendFunc(src)
	dstGLFunc := r.mapBlendFunc(dst)
	webglContext.Call("blendFunc", srcGLFunc, dstGLFunc)
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
		handle = webglContext.Call("createTexture")
		webglContext.Call("bindTexture", TEXTURE_2D, handle)

		// Initialize with empty data
		webglContext.Call(
			"texImage2D",
			TEXTURE_2D, 0, RGBA,
			int(width), int(height), 0,
			RGBA, UNSIGNED_BYTE, js.Null(),
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
	if r.spriteProgram.Truthy() {
		loc := webglContext.Call("getUniformLocation", r.spriteProgram, name)
		if loc.Truthy() {
			webglContext.Call("uniform1i", loc, val)
		}
	}
}

func (r *Renderer_WebGL) SetUniformF(name string, values ...float32) {
	if r.spriteProgram.Truthy() {
		loc := webglContext.Call("getUniformLocation", r.spriteProgram, name)
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
}

func (r *Renderer_WebGL) SetUniformFv(name string, values []float32) {
	if r.spriteProgram.Truthy() {
		loc := webglContext.Call("getUniformLocation", r.spriteProgram, name)
		if !loc.Truthy() {
			return
		}
		f32Array := js.Global().Get("Float32Array").New(len(values))
		for i, v := range values {
			f32Array.SetIndex(i, v)
		}
		webglContext.Call("uniform1fv", loc, f32Array)
	}
}

func (r *Renderer_WebGL) SetUniformMatrix(name string, value []float32) {
	if r.spriteProgram.Truthy() {
		loc := webglContext.Call("getUniformLocation", r.spriteProgram, name)
		if !loc.Truthy() {
			return
		}
		f32Array := js.Global().Get("Float32Array").New(16)
		for i := 0; i < 16 && i < len(value); i++ {
			f32Array.SetIndex(i, value[i])
		}
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
	if name == "palTex" {
		unit = 1
		r.currentPalTex = t
	} else {
		unit = 0
		r.currentTexture = t
	}

	webglContext.Call("activeTexture", TEXTURE0+unit)
	webglContext.Call("bindTexture", TEXTURE_2D, t.handle)

	if name == "palTex" {
		webglContext.Call("uniform1i", r.palTexLoc, unit)
	} else {
		webglContext.Call("uniform1i", r.texLoc, unit)
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

func (r *Renderer_WebGL) SetVertexData(values ...float32) {
	if !webglContext.Truthy() || !r.vertexBuffer.Truthy() {
		return
	}

	// Convert float32 slice to JS Float32Array
	f32Array := js.Global().Get("Float32Array").New(len(values))
	for i, v := range values {
		f32Array.SetIndex(i, v)
	}

	webglContext.Call("bindBuffer", ARRAY_BUFFER, r.vertexBuffer)
	webglContext.Call("bufferData", ARRAY_BUFFER, f32Array, DYNAMIC_DRAW)
}

func (r *Renderer_WebGL) SetModelVertexData(bufferIndex uint32, values []byte) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelIndexData(bufferIndex uint32, values ...uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) RenderQuad() {
	if !webglContext.Truthy() || !r.spriteProgram.Truthy() || !r.vao.Truthy() {
		logConsole("RenderQuad: skipping, not ready")
		return
	}

	webglContext.Call("useProgram", r.spriteProgram)
	webglContext.Call("bindVertexArray", r.vao)
	err := webglContext.Call("getError").Int()
	if err != 0 {
		logConsole("RenderQuad: pre-draw GL error " + js.ValueOf(err).String())
	}
	webglContext.Call("drawArrays", TRIANGLE_STRIP, 0, 4)
	err = webglContext.Call("getError").Int()
	if err != 0 {
		logConsole("RenderQuad: post-draw GL error " + js.ValueOf(err).String())
	}
}

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
