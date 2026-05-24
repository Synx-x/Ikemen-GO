//go:build !js

// Package mempool wraps Go's experimental arena package for non-js targets,
// and provides a heap-backed fallback for GOOS=js GOARCH=wasm builds where
// arena is unavailable. See pool_js.go for the WASM fallback.
//
// Surface mirrors stdlib arena exactly so call sites read identically:
//   a := mempool.NewArena()
//   p := mempool.New[Foo](a)
//   s := mempool.MakeSlice[Bar](a, n, n)
//   a.Free()
//
// Build requires GOEXPERIMENT=arenas on native targets (Go 1.20+).
package mempool

import "arena"

type Arena = arena.Arena

func NewArena() *Arena { return arena.NewArena() }

func New[T any](a *Arena) *T { return arena.New[T](a) }

func MakeSlice[T any](a *Arena, length, capacity int) []T {
	return arena.MakeSlice[T](a, length, capacity)
}
