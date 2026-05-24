//go:build js

package main

// wasm entry point. Real initialization happens via syscall/js callbacks.
func main() {
	// WASM main is a no-op. The engine is driven from JS via exported functions.
	// Pending: DOM bridge, Web Audio initialization, and event loop setup in D5+.
	select {}
}
