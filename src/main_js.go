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

	js.Global().Set("ikemen", bridge)
	logConsole("[ikemen-wasm] JS bridge ready as window.ikemen")

	<-keepAlive
	logConsole("[ikemen-wasm] main() returning")
}

// logConsole is a thin wrapper around console.log via syscall/js.
// Declared in render_webgl.go on the js side, but provided here as a
// fallback if that file's logConsole is unexported. Safe to re-declare
// because render_webgl.go's is package-private and not exported.
