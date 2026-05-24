//go:build js

package main

import (
	"fmt"
	"syscall/js"
)

// bootEngine is the wasm equivalent of native realMain() up to the point where
// the engine starts rendering menus. It runs the minimum initialization sequence:
// ensure config, run initLUTs, loadConfig, then enter the title screen state.
// Audio, file writes, and SDL window mgmt are stubbed out via existing platform stubs.
func bootEngine() error {
	defer func() {
		if r := recover(); r != nil {
			logConsole(fmt.Sprintf("[ikemen-wasm] bootEngine PANIC: %v", r))
		}
	}()

	logConsole("[ikemen-wasm] bootEngine() entered")

	// Ensure cmdFlags map exists
	if sys.cmdFlags == nil {
		sys.cmdFlags = make(map[string]string)
	}
	sys.cmdFlags["-stats"] = "save/stats.json"

	// Skip initLUTs — it's in input_sdl.go which is tagged out.
	// Platform stubs provide empty StringToKeyLUT/StringToButtonLUT.

	cfg, cfgErr := loadConfig("save/config.ini")
	if cfgErr != nil {
		logConsole("[ikemen-wasm] loadConfig failed: " + cfgErr.Error())
		return cfgErr
	}
	sys.cfg = *cfg
	logConsole(fmt.Sprintf("[ikemen-wasm] config loaded: motif=%s", sys.cfg.Config.Motif))

	return nil
}

// exposBootEngineBridge registers the JS bridge for bootEngine.
// This runs via init() goroutine to ensure main_js.go has created the ikemen object first.
func init() {
	// Poll for the ikemen bridge to be registered by main_js.go.
	go func() {
		for {
			bridge := js.Global().Get("ikemen")
			if !bridge.Truthy() {
				// Bridge not ready yet; retry in 10ms
			} else {
				// Bridge is ready; expose bootEngine
				bridge.Set("bootEngine", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					if err := bootEngine(); err != nil {
						return js.ValueOf(map[string]interface{}{"ok": false, "err": err.Error()})
					}
					return js.ValueOf(map[string]interface{}{"ok": true})
				}))
				logConsole("[ikemen-wasm] bootEngine bridge registered")
				return
			}
		}
	}()
}
