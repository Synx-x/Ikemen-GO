//go:build js

package main

import (
	"syscall/js"

	mgl "github.com/go-gl/mathgl/mgl32"
)

type Renderer_WebGL struct {
	// Minimal state tracking
	name string
}

type Texture_WebGL struct {
	width  int32
	height int32
	depth  int32
	filter bool
	serial uint64
}

// Implement Texture interface
func (t *Texture_WebGL) SetData(data []byte) {
	logConsole("Texture_WebGL.SetData called")
}

func (t *Texture_WebGL) SetSubData(data []byte, x, y, width, height, stride int32) {
	logConsole("Texture_WebGL.SetSubData called")
}

func (t *Texture_WebGL) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {
	logConsole("Texture_WebGL.SetDataG called")
}

func (t *Texture_WebGL) SetPixelData(data []float32) {
	logConsole("Texture_WebGL.SetPixelData called")
}

func (t *Texture_WebGL) IsValid() bool {
	return true
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

// logConsole logs a message to browser console
func logConsole(msg interface{}) {
	js.Global().Get("console").Call("log", msg)
}

// Renderer_WebGL methods implementing the Renderer interface

func (r *Renderer_WebGL) GetName() string {
	return "WebGL2"
}

func (r *Renderer_WebGL) Init() {
	logConsole("Renderer_WebGL.Init called")
}

func (r *Renderer_WebGL) Close() {
	logConsole("Renderer_WebGL.Close called")
}

func (r *Renderer_WebGL) BeginFrame(clearColor bool) {
	logConsole("Renderer_WebGL.BeginFrame called")
}

func (r *Renderer_WebGL) EndFrame() {
	logConsole("Renderer_WebGL.EndFrame called")
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
	logConsole("SetSpritePipeline: " + shaderName)
}

func (r *Renderer_WebGL) SetCustomUniforms(params [16]float32) {
	// No-op stub
}

func (r *Renderer_WebGL) NeedsGrabPass() bool {
	return false
}

func (r *Renderer_WebGL) ResolveBackBuffer() Texture {
	return nil
}

func (r *Renderer_WebGL) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	// No-op stub
}

func (r *Renderer_WebGL) DisableBlending() {
	// No-op stub
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
	return &Texture_WebGL{
		width:  width,
		height: height,
		depth:  depth,
		filter: filter,
		serial: textureSerialNumber,
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
	// No-op stub
}

func (r *Renderer_WebGL) SetUniformF(name string, values ...float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetUniformFv(name string, values []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetUniformMatrix(name string, value []float32) {
	// No-op stub
}

func (r *Renderer_WebGL) SetTexture(name string, tex Texture) {
	// No-op stub
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
	// No-op stub
}

func (r *Renderer_WebGL) SetModelVertexData(bufferIndex uint32, values []byte) {
	// No-op stub
}

func (r *Renderer_WebGL) SetModelIndexData(bufferIndex uint32, values ...uint32) {
	// No-op stub
}

func (r *Renderer_WebGL) RenderQuad() {
	logConsole("Renderer_WebGL.RenderQuad called")
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

// Register the WebGL renderer on init
func init() {
	gfx = &Renderer_WebGL{name: "WebGL2"}
	logConsole("WebGL2 renderer registered")
}
