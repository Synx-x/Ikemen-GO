//go:build js

package main

import (
	"fmt"
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

	// ikemen.setViewport(w, h) — resize the render to match the canvas/window.
	// Calls sys.setGameSize, which sets scrrect and recomputes gameWidth/
	// gameHeight by the new aspect (wider window -> wider FOV, taller ->
	// taller; no stretch). The GL viewport itself is read from canvas.width/
	// height each BeginFrame, so JS sets those before calling this. Enqueued
	// onto mainThreadTask so it lands between frames, not mid-draw.
	bridge.Set("setViewport", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return false
		}
		w := int32(args[0].Int())
		h := int32(args[1].Int())
		if w < 16 || h < 16 {
			return false
		}
		if sys.mainThreadTask != nil {
			sys.mainThreadTask <- func() { sys.setGameSize(w, h) }
		} else {
			sys.setGameSize(w, h)
		}
		return true
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

	// ikemen.fetchAsset(path) — returns a Promise that resolves to a Uint8Array
	// containing the asset bytes. Used by the engine (or test harness) to read
	// chars, stages, data files served by Vite under /assets/.
	bridge.Set("fetchAsset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.Undefined()
		}
		path := args[0].String()
		promiseCtor := js.Global().Get("Promise")
		executor := js.FuncOf(func(this js.Value, pa []js.Value) interface{} {
			resolve := pa[0]
			reject := pa[1]
			go func() {
				data, err := FetchAsset(path)
				if err != nil {
					reject.Invoke(err.Error())
					return
				}
				dst := js.Global().Get("Uint8Array").New(len(data))
				js.CopyBytesToJS(dst, data)
				resolve.Invoke(dst)
			}()
			return nil
		})
		return promiseCtor.New(executor)
	}))

	// ikemen.loadAssetBundle(uint8Array) — loads a zip-backed asset bundle into the VFS.
	// Called from JS after fetching the asset bundle. Returns {ok: true, entries: count}
	// on success or {ok: false, err: "message"} on failure.
	bridge.Set("loadAssetBundle", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(map[string]interface{}{"ok": false, "err": "no args"})
		}
		u8 := args[0]
		n := u8.Get("length").Int()
		buf := make([]byte, n)
		js.CopyBytesToGo(buf, u8)
		loaded, err := LoadAssetZip(buf)
		if err != nil {
			logConsole("[ikemen-wasm] LoadAssetZip failed: " + err.Error())
			return js.ValueOf(map[string]interface{}{"ok": false, "err": err.Error()})
		}
		logConsole(fmt.Sprintf("[ikemen-wasm] VFS loaded: %d entries", loaded))
		return js.ValueOf(map[string]interface{}{"ok": true, "entries": loaded})
	}))

	// ikemen.vfsHas(path) — check if a path exists in the loaded VFS. Returns bool.
	bridge.Set("vfsHas", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return false
		}
		return VFSHas(args[0].String())
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
