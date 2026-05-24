# render_webgl.go research notes. KEY findings

## Use embedded shaders, not custom GLSL

`src/render.go` (NO build tag, available on js) declares:

```go
//go:embed shaders/sprite.vert.glsl
var vertShader string

//go:embed shaders/sprite.frag.glsl
var fragShader string
```

These are the EXACT shaders the engine expects. The fragment shader supports:

- `vec2 position` + `vec2 uv` attributes
- `mat4 modelview`, `mat4 projection` uniforms
- `sampler2D tex`, `sampler2D pal` samplers
- `vec4 x1x2x4x3`, `vec4 tint`
- `vec3 add`, `vec3 mult`
- `float alpha`, `float gray`, `float hue`
- `int mask`
- `bool isFlat`, `bool isRgba`, `bool isTrapez`, `bool neg`

For paletted sprites, fragment shader does:

```glsl
c = COMPAT_TEXTURE(tex, uv);   // sample R8 paletted
c = COMPAT_TEXTURE(pal, vec2(c.r * 0.9966, 0.5));  // lookup palette
```

## Compiling the embedded shaders for WebGL2

Native render_gl33 prepends `"#version 330 core\n"`. For WebGL2, prepend `"#version 300 es\n"` instead. The shader uses `#if __VERSION__ >= 450` to gate Vulkan, otherwise falls into `#ifdef GL_ES` precision blocks.

```go
const wgl2Prefix = "#version 300 es\nprecision highp float;\nprecision highp int;\n"
fullVertSrc := wgl2Prefix + vertShader
fullFragSrc := wgl2Prefix + fragShader
```

## Common pitfalls

1. **Texture filter on R8 MUST be NEAREST**. LINEAR interpolates palette indices → wrong colors. Hardcode NEAREST for depth=8 textures regardless of t.filter.

2. **Bind both tex AND pal samplers** every SetSpritePipeline. `glUniform1i(texLoc, 0)` and `glUniform1i(palLoc, 1)`. slot 0 + slot 1.

3. **isRgba flag**. pass it as int uniform per RenderQuad. For R8 textures `isRgba=0`, for RGBA `isRgba=1`. Engine sets via SetUniformI("isRgba", ...) before RenderQuad.

4. **BlendFuncSeparate per pipeline**. SetSpritePipeline receives the BlendFunc + BlendEquation. Native calls `gl.BlendFuncSeparate(srcRGB, dstRGB, srcA, dstA)` + `gl.BlendEquation(eq)`. Don't just Enable(BLEND).

5. **UNPACK_FLIP_Y_WEBGL = 0x9240**. set via `pixelStorei(UNPACK_FLIP_Y_WEBGL, 0)`. MUGEN sprites are bottom-up like OpenGL; WebGL default is top-down. Most likely set to false (0).

6. **Vertex attribute layout**. engine's SetVertexData uploads `[x, y, u, v, x, y, u, v, ...]`. Stride = 16 bytes (4 floats × 4 bytes). Position attrib offset=0, UV attrib offset=8 bytes. Must `enableVertexAttribArray` for BOTH.

7. **gl.useProgram + gl.bindVertexArray BEFORE setting uniforms**. uniforms are per-program. Order matters.

8. **DrawArrays mode = TRIANGLE_FAN (6) not TRIANGLE_STRIP (5)**. check what native uses. Different vertex order requires different mode.

## Native draw order per quad (from render_gl33.go RenderQuad)

```
1. gl.UseProgram(currentProgram)
2. gl.BindVertexArray(vao)
3. gl.BindBuffer(ARRAY_BUFFER, vbo) + gl.BufferData(...) [in SetVertexData]
4. Per-uniform: gl.Uniform4fv("x1x2x4x3", ...), Uniform1f("alpha", ...), etc.
5. gl.ActiveTexture(TEXTURE0) + gl.BindTexture(2D, spriteTex) + gl.Uniform1i("tex", 0)
6. gl.ActiveTexture(TEXTURE1) + gl.BindTexture(2D, paletteTex) + gl.Uniform1i("pal", 1)
7. gl.Enable(BLEND) + gl.BlendFuncSeparate(...) + gl.BlendEquation(...)
8. gl.DrawArrays(mode, 0, 4)
```

If any of 4-7 is missing, the quad draws transparent or wrong-colored or invisible.

## Why brown screenshot today

Agent's RenderQuad calls drawArrays but the engine never:
- Wired up uniforms via SetUniformF (probably a no-op stub)
- Bound the palette texture via SetTexture (no-op)
- Set blend func per pipeline (probably default OFF or wrong)
- Uploaded vertex data via SetVertexData (no-op → vao bound is empty → drawArrays renders nothing)

Result: drawArrays issued but vertex buffer is empty → no geometry → no pixels.

The fix is implementing each of those methods, mirroring render_gl33 line by line.
