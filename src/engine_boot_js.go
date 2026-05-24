//go:build js

package main

import (
	"fmt"
	"strings"
	"syscall/js"

	lua "github.com/yuin/gopher-lua"
)

// installVFSLuaLoader prepends a custom searcher to package.loaders so
// `require("external.script.debug")` resolves against the VFS instead of
// hitting native os.Open. gopher-lua follows Lua 5.1 semantics where
// package.loaders is a table of search functions tried in order.
//
// The custom loader converts module names like "external.script.debug"
// to a file path "external/script/debug.lua" and reads via engineReadFile.
// On success it returns a compiled function via L.LoadString. On failure
// it returns a string error message so Lua keeps trying other loaders.
func installVFSLuaLoader(L *lua.LState) {
	pkg := L.GetField(L.Get(lua.EnvironIndex), "package").(*lua.LTable)
	loaders := L.GetField(pkg, "loaders")
	if loaders == lua.LNil {
		// Lua 5.2+ name; fall back. gopher-lua should expose loaders.
		loaders = L.GetField(pkg, "searchers")
	}
	loadersTbl, ok := loaders.(*lua.LTable)
	if !ok {
		logConsole("[ikemen-wasm] LUA: package.loaders not a table, skipping VFS loader")
		return
	}

	vfsLoader := L.NewFunction(func(L *lua.LState) int {
		modName := L.CheckString(1)
		// Convert dots to slashes, append .lua.
		fpath := strings.ReplaceAll(modName, ".", "/") + ".lua"
		data, err := engineReadFile(fpath)
		if err != nil {
			// Push an error message string and return 1 so Lua moves to next loader.
			L.Push(lua.LString("\n\tno VFS entry " + fpath))
			return 1
		}
		fn, lerr := L.LoadString(string(data))
		if lerr != nil {
			L.Push(lua.LString("\n\tVFS compile error in " + fpath + ": " + lerr.Error()))
			return 1
		}
		L.Push(fn)
		return 1
	})

	// Prepend by shifting existing loaders down one slot.
	existing := []lua.LValue{}
	for i := 1; ; i++ {
		v := loadersTbl.RawGetInt(i)
		if v == lua.LNil {
			break
		}
		existing = append(existing, v)
	}
	loadersTbl.RawSetInt(1, vfsLoader)
	for i, v := range existing {
		loadersTbl.RawSetInt(i+2, v)
	}
	logConsole("[ikemen-wasm] LUA: VFS loader installed at package.loaders[1]")
}

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

	// Stage 1: Open and verify the Lua system script file
	logConsole("[ikemen-wasm] stage=file_open: opening " + sys.cfg.Config.System)
	ftemp, err := engineOpen(sys.cfg.Config.System)
	if err != nil {
		logConsole("[ikemen-wasm] stage=file_open: FAILED: " + err.Error())
		return err
	}
	ftemp.Close()
	logConsole("[ikemen-wasm] stage=file_open: OK")

	// Stage 2: Initialize renderer and system state
	logConsole("[ikemen-wasm] stage=sys_init: calling sys.init(" +
		fmt.Sprintf("%d, %d)", sys.gameWidth, sys.gameHeight))

	var luaErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				luaErr = fmt.Errorf("panic during sys.init: %v", r)
			}
		}()
		sys.luaLState = sys.init(int32(sys.gameWidth), int32(sys.gameHeight))
	}()

	if luaErr != nil {
		logConsole("[ikemen-wasm] stage=sys_init: FAILED: " + luaErr.Error())
		return luaErr
	}
	logConsole("[ikemen-wasm] stage=sys_init: OK")

	// Stage 3: Execute the Lua system script
	logConsole("[ikemen-wasm] stage=lua_dofile: calling sys.luaLState.DoFile(" + sys.cfg.Config.System + ")")

	func() {
		defer func() {
			if r := recover(); r != nil {
				luaErr = fmt.Errorf("panic during DoFile: %v", r)
			}
		}()
		// gopher-lua's DoFile uses native os.Open, which can't see the VFS
		// on wasm. Read the script bytes via engineReadFile then DoString.
		// Lua's `require` calls inside the script will hit the same wall —
		// next step is to install a custom package.loaders entry that
		// routes through engineReadFile, but try this first to see how
		// far we get.
		installVFSLuaLoader(sys.luaLState)
		scriptBytes, readErr := engineReadFile(sys.cfg.Config.System)
		if readErr != nil {
			luaErr = readErr
		} else {
			luaErr = sys.luaLState.DoString(string(scriptBytes))
		}
	}()

	if luaErr != nil {
		errMsg := luaErr.Error()
		logConsole("[ikemen-wasm] stage=lua_dofile: " + errMsg)
		// Check if this is a normal game-end signal
		if strings.Contains(errMsg, "<game end>") {
			logConsole("[ikemen-wasm] stage=lua_dofile: game end signal received (normal)")
			return nil
		}
		return luaErr
	}
	logConsole("[ikemen-wasm] stage=lua_dofile: OK (game running)")

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
