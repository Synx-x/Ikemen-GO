//go:build js

// WASM fallback: stdlib arena is not available under GOOS=js GOARCH=wasm
// (build constraints exclude all files in /usr/lib/go/src/arena for js).
// This impl satisfies the same surface using regular heap allocation.
// GC pressure is higher than arena-backed builds, accepted for browser.
package mempool

type Arena struct{}

func NewArena() *Arena { return &Arena{} }

func (a *Arena) Free() {}

func New[T any](_ *Arena) *T { return new(T) }

func MakeSlice[T any](_ *Arena, length, capacity int) []T {
	return make([]T, length, capacity)
}
