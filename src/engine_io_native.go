//go:build !js

// engineOpen / engineReadFile / engineStat: platform-neutral file
// reads for the engine. Native side just forwards to the os package
// so behavior is unchanged. JS side (engine_io_js.go) routes to a
// virtual filesystem populated from a fetched zip bundle.
//
// Sed maps engine call sites from os.X to engineX so neither build
// loses access to assets. Only read paths are routed; writes
// (save/replays, save/logs, stats.json) still hit os.* directly via
// engineWriteX wrappers below — on js those are no-ops since wasm has
// no real filesystem.
package main

import (
	"io"
	"os"
)

func engineOpen(path string) (io.ReadCloser, error) { return os.Open(path) }
func engineReadFile(path string) ([]byte, error)    { return os.ReadFile(path) }
func engineStat(path string) (os.FileInfo, error)   { return os.Stat(path) }
