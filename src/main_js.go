//go:build js

package main

import (
	"syscall/js"
)

// wasm entry point. The engine is driven from JS via the global
// `ikemen` object exposed below. Browser calls into the wasm by reading
// ikemen.version, ikemen.tick(deltaSec), ikemen.handleInput(...), etc.
//
// main() registers the bridge then blocks on a channel that gets pinged
// from JS callbacks. Without a live channel reference, Node's wasm_exec
// hits "all goroutines asleep" because there's no DOM event loop to keep
// the runtime awake.
func main() {
	logConsole("[ikemen-wasm] main() entered, registering JS bridge")

	keepAlive := make(chan struct{}, 1)

	bridge := js.Global().Get("Object").New()
	bridge.Set("version", js.ValueOf(Version))
	bridge.Set("buildTime", js.ValueOf(BuildTime))
	bridge.Set("rendererName", js.ValueOf(func() string {
		if gfx != nil {
			return gfx.GetName()
		}
		return "none"
	}()))

	// ikemen.shutdown() — lets the page request a clean wasm exit.
	bridge.Set("shutdown", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		logConsole("[ikemen-wasm] shutdown called")
		select {
		case keepAlive <- struct{}{}:
		default:
		}
		return nil
	}))

	// ikemen.ping() — round-trip check from browser. Returns a string.
	bridge.Set("ping", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf("pong from wasm: " + Version)
	}))

	// ikemen.setCanvas(canvasElement) — JS hands the canvas to Go for WebGL setup
	bridge.Set("setCanvas", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			logConsole("[ikemen-wasm] setCanvas: no canvas argument")
			return false
		}

		canvas := args[0]
		if !canvas.Truthy() {
			logConsole("[ikemen-wasm] setCanvas: canvas is falsy")
			return false
		}

		// Try to get WebGL2 context with preserveDrawingBuffer so screenshots
		// and readPixels see the most recent clear/draw without race against
		// the compositor's buffer swap. See WebGL 2 spec §2.2.
		opts := js.Global().Get("Object").New()
		opts.Set("preserveDrawingBuffer", true)
		opts.Set("antialias", false)
		opts.Set("alpha", false)
		gl := canvas.Call("getContext", "webgl2", opts)
		if !gl.Truthy() {
			gl = canvas.Call("getContext", "experimental-webgl", opts)
		}
		if !gl.Truthy() {
			logConsole("[ikemen-wasm] setCanvas: FATAL — no WebGL support (webgl2 and experimental-webgl both failed)")
			return false
		}

		// Stash the canvas and context as package-level vars in render_webgl_state.go
		setWebGLContext(canvas, gl)
		logConsole("[ikemen-wasm] setCanvas: WebGL2 context acquired, canvas dimensions: " +
			js.ValueOf(canvas.Get("width").Int()).String() + "x" + js.ValueOf(canvas.Get("height").Int()).String())
		return true
	}))

	// ikemen.frameTick() — callback for requestAnimationFrame
	// Calls BeginFrame + EndFrame to drive the render loop from JS
	bridge.Set("frameTick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if gfx != nil {
			gfx.BeginFrame(true)
			gfx.EndFrame()
		}
		return nil
	}))

	// ikemen.injectKey(code, keyVal, down, mods) — DOM keyboard event bridge
	// Called from JS event listeners (window.addEventListener keydown/keyup).
	// Args: code (string), keyVal (int), down (bool), mods (int modifier bitmask).
	// Pushes DomKeyEvent onto the queue for consumption by DrainDomKeys() in D6.3+.
	bridge.Set("injectKey", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			logConsole("[ikemen-wasm] injectKey: insufficient args (need code, keyVal, down, mods)")
			return nil
		}

		code := args[0].String()
		keyVal := args[1].Int()
		down := args[2].Bool()
		mods := sdlKeymod(args[3].Int())

		injectKeyFromDOM(code, keyVal, down, mods)
		return nil
	}))

	// ikemen.lastKeys() — returns last N key events as a JS array for status display.
	// Each entry is an object: { code: string, down: bool, t: timestamp (ms) }.
	// Used by the page to show recent keypresses in a status div.
	bridge.Set("lastKeys", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		maxKeys := 5

		domKeyMutex.Lock()
		defer domKeyMutex.Unlock()

		resultLen := len(domKeyQueue)
		if resultLen > maxKeys {
			resultLen = maxKeys
		}

		// Return the last N keys from the queue
		result := js.Global().Get("Array").New()
		start := len(domKeyQueue) - resultLen
		if start < 0 {
			start = 0
		}

		idx := 0
		for _, evt := range domKeyQueue[start:] {
			entry := js.Global().Get("Object").New()
			entry.Set("code", evt.Code)
			entry.Set("down", evt.Down)
			entry.Set("t", js.Global().Get("performance").Call("now").Float())
			result.SetIndex(idx, entry)
			idx++
		}

		return result
	}))

	js.Global().Set("ikemen", bridge)
	logConsole("[ikemen-wasm] JS bridge ready as window.ikemen")

	<-keepAlive
	logConsole("[ikemen-wasm] main() returning")
}

// logConsole is a thin wrapper around console.log via syscall/js.
// Declared in render_webgl.go on the js side, but provided here as a
// fallback if that file's logConsole is unexported. Safe to re-declare
// because render_webgl.go's is package-private and not exported.
