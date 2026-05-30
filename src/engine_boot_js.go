//go:build js

package main

import (
	"fmt"
	"strings"
	"syscall/js"

	lua "github.com/yuin/gopher-lua"
)

var (
	engineStarted   bool
	engineLastError string
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

// installVFSIoOpen overrides Lua's `io.open` with a VFS-backed reader.
// The native `io.open` calls os.Open under the hood, which can't see
// the in-memory VFS on wasm. Override returns a Lua table that mimics
// Lua's file handle (read, lines, close) backed by an engineReadFile
// byte slice. Writes are silently dropped — wasm has no fs.
//
// This is what unblocks main.lua's `f_fileRead` (line 43) and any other
// Lua code that does `io.open(path)` to load motif files, char lists,
// stage rosters, options.lua state, etc.
func installVFSIoOpen(L *lua.LState) {
	ioTbl, ok := L.GetField(L.Get(lua.EnvironIndex), "io").(*lua.LTable)
	if !ok {
		logConsole("[ikemen-wasm] LUA: io table not found, skipping io.open override")
		return
	}
	L.SetField(ioTbl, "open", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		// mode := L.OptString(2, "r") — VFS is read-only, ignore mode
		data, err := engineReadFile(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		// Build a file-handle-like table with read + close methods.
		handle := L.NewTable()
		offset := 0
		L.SetField(handle, "read", L.NewFunction(func(L *lua.LState) int {
			// arg 1 is self (the handle); arg 2 is the format
			format := L.OptString(2, "*l")
			if format == "*a" || format == "*all" {
				out := string(data[offset:])
				offset = len(data)
				L.Push(lua.LString(out))
				return 1
			}
			if format == "*l" || format == "*L" {
				// Read one line.
				start := offset
				for offset < len(data) && data[offset] != '\n' {
					offset++
				}
				line := string(data[start:offset])
				if offset < len(data) {
					offset++ // consume \n
				}
				L.Push(lua.LString(line))
				return 1
			}
			if n, ok := L.Get(2).(lua.LNumber); ok {
				wanted := int(n)
				if offset+wanted > len(data) {
					wanted = len(data) - offset
				}
				if wanted <= 0 {
					L.Push(lua.LNil)
					return 1
				}
				out := string(data[offset : offset+wanted])
				offset += wanted
				L.Push(lua.LString(out))
				return 1
			}
			L.Push(lua.LNil)
			return 1
		}))
		L.SetField(handle, "close", L.NewFunction(func(L *lua.LState) int {
			return 0
		}))
		L.SetField(handle, "lines", L.NewFunction(func(L *lua.LState) int {
			L.Push(L.NewFunction(func(L *lua.LState) int {
				if offset >= len(data) {
					L.Push(lua.LNil)
					return 1
				}
				start := offset
				for offset < len(data) && data[offset] != '\n' {
					offset++
				}
				line := string(data[start:offset])
				if offset < len(data) {
					offset++
				}
				L.Push(lua.LString(line))
				return 1
			}))
			return 1
		}))
		L.SetField(handle, "seek", L.NewFunction(func(L *lua.LState) int {
			whence := L.OptString(2, "cur")
			off := int(L.OptInt(3, 0))
			switch whence {
			case "set":
				offset = off
			case "cur":
				offset += off
			case "end":
				offset = len(data) + off
			}
			if offset < 0 {
				offset = 0
			}
			if offset > len(data) {
				offset = len(data)
			}
			L.Push(lua.LNumber(offset))
			return 1
		}))
		L.Push(handle)
		return 1
	}))
	logConsole("[ikemen-wasm] LUA: io.open overridden to route through VFS")
}

// installWriteNoops overrides Lua bindings that try to write to disk.
// wasm has no fs so saves/options/replays/screenshots can't persist
// without a localStorage bridge. For v1, no-op them so the engine
// progresses past config save calls without panic. Later we can
// route to localStorage via a JS bridge.
//
// Bindings re-registered: saveGameOption, saveIni, takeScreenshot,
// f_fileWrite (Lua-side helper that also uses io.open in write mode —
// already handled by installVFSIoOpen returning nil from io.open for
// write modes? No — current io.open override always reads. Need to
// keep write attempts from panicking by short-circuiting saves.)
func installWriteNoops(L *lua.LState) {
	noop := L.NewFunction(func(L *lua.LState) int {
		// Silently consume args; return success.
		return 0
	})
	L.SetGlobal("saveGameOption", noop)
	L.SetGlobal("saveIni", noop)
	L.SetGlobal("takeScreenshot", noop)
	logConsole("[ikemen-wasm] LUA: write paths (saveGameOption/saveIni/takeScreenshot) no-op'd")
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

	// Disable the wall-clock frame-skip path. On wasm the rAF-driven loop +
	// time.Sleep semantics pin frameSkip=true, which makes refresh() discard
	// the Lua draw queue (luaDiscardDrawQueue) and drop all menu textImgDraw
	// /rectDraw calls — menu items flash once then vanish.
	forceNoFrameSkip = true

	// Ensure cmdFlags map exists
	if sys.cmdFlags == nil {
		sys.cmdFlags = make(map[string]string)
	}
	sys.cmdFlags["-stats"] = "save/stats.json"

	// Seed bootstrap files into VFS that native main() would have
	// created via os.MkdirAll + write. On wasm there's no real fs,
	// so we put placeholder bytes into the VFS so the engine + Lua
	// io.open calls succeed.
	vfsSeed("save/stats.json", []byte("{}"))

	// Skip initLUTs — it's in input_sdl.go which is tagged out.
	// Platform stubs provide empty StringToKeyLUT/StringToButtonLUT.

	cfg, cfgErr := loadConfig("save/config.ini")
	if cfgErr != nil {
		logConsole("[ikemen-wasm] loadConfig failed: " + cfgErr.Error())
		return cfgErr
	}
	sys.cfg = *cfg
	// On wasm, config saves are no-op'd (no filesystem), so Config.FirstRun
	// never persists as false. Left true, the title menu enters the first-run
	// infobox (f_warning), which loops waiting for a dismiss key that never
	// arrives headless — the menu loop wedges and the menu never appears.
	// Force it false so the title falls straight through to the live menu.
	sys.cfg.Config.FirstRun = false
	// The seeded wasm config.ini has no [Keys_P1] bindings, so every keyConfig
	// loads with dU/dD/...=0 — GetKeyboardState then reads keyState[Key(0)] and
	// the menu never sees keyboard input. Seed P1 keyboard defaults that match
	// the DOM bridge's domCodeToKey mapping (arrows + Enter/Esc + letters) so
	// the menu is navigable. Buttons use SDLK letter codes (KeyA=0x61, etc).
	if len(sys.keyConfig) > 0 {
		sys.keyConfig[0] = KeyConfig{
			Joy: -1,
			dU:  int(KeyUp), dD: int(KeyDown), dL: int(KeyLeft), dR: int(KeyRight),
			bA: 0x7a /*z*/, bB: 0x78 /*x*/, bC: 0x63 /*c*/,
			bX: 0x61 /*a*/, bY: 0x73 /*s*/, bZ: 0x64 /*d*/,
			bS: int(KeyEnter), bD: int(KeyEnter), bW: int(KeyBackspace), bM: int(KeyEscape),
		}
	}
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
		installVFSIoOpen(sys.luaLState)
		installWriteNoops(sys.luaLState)
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
				bridge.Set("bootEngineAsync", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					// Lua main loop never returns. Run engine in goroutine,
					// return immediately so JS event loop survives.
					if engineStarted {
						return js.ValueOf(map[string]interface{}{"ok": true, "alreadyStarted": true})
					}
					engineStarted = true
					go func() {
						defer func() {
							if r := recover(); r != nil {
								logConsole(fmt.Sprintf("[ikemen-wasm] engine goroutine panic: %v", r))
								engineLastError = fmt.Sprintf("%v", r)
							}
						}()
						if err := bootEngine(); err != nil {
							engineLastError = err.Error()
							logConsole("[ikemen-wasm] bootEngine returned err: " + err.Error())
						} else {
							logConsole("[ikemen-wasm] bootEngine returned cleanly")
						}
					}()
					return js.ValueOf(map[string]interface{}{"ok": true, "running": true})
				}))
				bridge.Set("getProgram", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					rwg, _ := gfx.(*Renderer_WebGL)
					if rwg != nil && rwg.spriteProgram.Truthy() {
						return rwg.spriteProgram
					}
					return js.Null()
				}))
				bridge.Set("getVAO", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					rwg, _ := gfx.(*Renderer_WebGL)
					if rwg != nil && rwg.vao.Truthy() {
						return rwg.vao
					}
					return js.Null()
				}))
				bridge.Set("getVBO", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					rwg, _ := gfx.(*Renderer_WebGL)
					if rwg != nil && rwg.vertexBuffer.Truthy() {
						return rwg.vertexBuffer
					}
					return js.Null()
				}))
				bridge.Set("engineStatus", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					rwg, _ := gfx.(*Renderer_WebGL)
					shaderReady := false
					vaoReady := false
					if rwg != nil {
						shaderReady = rwg.spriteProgram.Truthy()
						vaoReady = rwg.vao.Truthy()
					}
					vlog := make([]interface{}, len(vertexCallLog))
					for i, s := range vertexCallLog {
						vlog[i] = s
					}
					rlog := make([]interface{}, len(bufferReadback))
					for i, s := range bufferReadback {
						rlog[i] = s
					}
					dlog := make([]interface{}, len(drawTimeLog))
					for i, s := range drawTimeLog {
						dlog[i] = s
					}
					kcDump := ""
					for i, kc := range sys.keyConfig {
						kcDump += fmt.Sprintf("[%d joy=%d dD=%d dU=%d] ", i, kc.Joy, kc.dD, kc.dU)
					}
					return js.ValueOf(map[string]interface{}{
						"started":            engineStarted,
						"err":                engineLastError,
						"keyStateDown":       sys.keyState[KeyDown],
						"keyStateUp":         sys.keyState[KeyUp],
						"nKeyConfig":         len(sys.keyConfig),
						"nCmdLists":          len(sys.commandLists),
						"kcDump":             kcDump,
						"frameCounter":       int(sys.frameCounter),
						"storyboardActive":   sys.storyboard.active,
						"renderQuad":         renderQuadCount,
						"renderQuadSkip":     renderQuadSkipped,
						"shaderReady":        shaderReady,
						"vaoReady":           vaoReady,
						"vertexCallCount":    vertexCallCount,
						"vertexCallLog":      vlog,
						"setVertexDataTotal": setVertexDataTotal,
						"firstProjection":    firstProjection,
						"firstModelview":     firstModelview,
						"bufferReadback":     rlog,
						"drawTimeLog":        dlog,
						"glyphDrawAttempts":  glyphDrawAttempts,
						"glyphSprNil":        glyphSprNil,
						"glyphTexNil":        glyphTexNil,
						"glyphRendered":      glyphRendered,
						"textImgDrawTotal":    textImgDrawTotal,
						"textImgDrawLastFrame": textImgDrawLastFrame,
						"flushLayerLog":       func() []interface{} { o := make([]interface{}, len(flushLayerLog)); for i, s := range flushLayerLog { o[i] = s }; return o }(),
						"flushOpsHist":        func() []interface{} { o := make([]interface{}, len(flushOpsHist)); for i, v := range flushOpsHist { o[i] = v }; return o }(),
						"awaitPixelLog":       func() []interface{} { o := make([]interface{}, len(awaitPixelLog)); for i, s := range awaitPixelLog { o[i] = s }; return o }(),
						"uiTrigLog":           func() []interface{} { o := make([]interface{}, len(uiTrigLog)); for i, s := range uiTrigLog { o[i] = s }; return o }(),
						"dbgTickLog":          func() []interface{} { o := make([]interface{}, len(dbgTickLog)); for i, s := range dbgTickLog { o[i] = s }; return o }(),
						"luaFlushCount":       luaFlushCount,
						"luaFlushNonEmpty":    luaFlushNonEmptyCount,
						"luaFlushOpsTotal":    luaFlushOpsTotal,
						"luaDiscardCount":     luaDiscardCount,
						"frameSkipNow":        sys.frameSkip,
						"forceNoFrameSkip":    forceNoFrameSkip,
						"ttfLog":              func() []interface{} { o := make([]interface{}, len(ttfLog)); for i, s := range ttfLog { o[i] = s }; return o }(),
						"tsDrawDiag":          func() []interface{} { o := make([]interface{}, len(tsDrawDiag)); for i, s := range tsDrawDiag { o[i] = s }; return o }(),
						"textImgDrawSamples":  func() []interface{} { o := make([]interface{}, len(textImgDrawSamples)); for i, s := range textImgDrawSamples { o[i] = s }; return o }(),
						"glyphVertLog":       func() []interface{} { o := make([]interface{}, len(glyphVertLog)); for i, s := range glyphVertLog { o[i] = s }; return o }(),
						"glyphDiag":           func() []interface{} { o := make([]interface{}, len(glyphDiag)); for i, s := range glyphDiag { o[i] = s }; return o }(),
						"glyphRenderQuad":     glyphRenderQuadCount,
						"glyphQuadReadback":   func() []interface{} { o := make([]interface{}, len(glyphQuadReadback)); for i, s := range glyphQuadReadback { o[i] = s }; return o }(),
					})
				}))
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
