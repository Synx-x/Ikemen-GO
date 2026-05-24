//go:build js

// JS stubs for symbols normally provided by SDL-dependent and cgo files
// (input_sdl.go, system_sdl.go, video_ffmpeg.go, main.go), which are
// excluded from the wasm build via //go:build !js. Stubs return zero
// values and no-op; real impls land in D5+ (DOM input bridge, Web
// Audio, HTML video).
package main

import (
	"fmt"
	"image"
)

// ---------------------------------------------------------------------
// Input layer — normally in input_sdl.go.
// ---------------------------------------------------------------------

type Key int32

// ModifierKey is a type alias of sdlKeymod (defined in input_sdl_keymod_js.go)
// so bitwise ops between them type-check without conversion, matching the
// native-side `type ModifierKey = sdl.Keymod` alias semantics.
type ModifierKey = sdlKeymod

// USB HID / SDL2 keycode constants. Values mirror SDLK_* so any persisted
// keymaps from the native side load identically in wasm builds.
const (
	KeyUnknown    Key = 0
	KeyEscape     Key = 0x4000001b // SDLK_ESCAPE
	KeyEnter      Key = 0x0000000d // SDLK_RETURN
	KeyInsert     Key = 0x40000049 // SDLK_INSERT
	KeyF5         Key = 0x40000040 // SDLK_F5
	KeyF12        Key = 0x40000045 // SDLK_F12
	KeyPause      Key = 0x40000048 // SDLK_PAUSE
	KeyScrollLock Key = 0x40000047 // SDLK_SCROLLLOCK
)

// ControllerState mirrors the native struct field types so input.go's
// indexed reads type-check unchanged. Axes is int8 to match the native
// SDL controller axis representation.
type ControllerState struct {
	Axes      [6]int8
	Buttons   map[int]byte
	HasRumble bool
}

type Input struct {
	controllerstate [MaxPlayerNo]*ControllerState
}

// buttonOrder mirrors input_sdl.go's []sdl.GameControllerButton list,
// stubbed as a small int slice so gamepad iteration loops compile.
var buttonOrder = []int{}

// CheckAxisForDpad and CheckAxisForTrigger return the axis name a gamepad
// hint maps to. Stub returns empty string ("no input").
func CheckAxisForDpad(axes *[6]float32, base int) string { return "" }
func CheckAxisForTrigger(axes *[6]float32) string        { return "" }

// Version + BuildTime live in main.go (now tagged !js). Mirror the values
// here. Real impl could read them from a build-injected js global.
var (
	Version   = "development-wasm"
	BuildTime = ""
)

func (i *Input) UpdateGamepadMappings(path string)            {}
func (i *Input) GetMaxJoystickCount() int                     { return 0 }
func (i *Input) IsJoystickPresent(joy int) bool               { return false }
func (i *Input) GetJoystickName(joy int) string               { return "" }
func (i *Input) GetJoystickAxes(joy int) [6]float32           { return [6]float32{} }
func (i *Input) GetJoystickButtons(joy int) []byte            { return nil }
func (i *Input) GetJoystickPath(joy int) string               { return "" }
func (i *Input) GetJoystickGUID(joy int) string               { return "" }
func (i *Input) RumbleController(joy int, lo, hi uint16, ticks uint32) {}

var input Input

var (
	StringToKeyLUT    = map[string]Key{}
	StringToButtonLUT = map[string]int{}
)

func StringToKey(s string) Key { return KeyUnknown }

// ---------------------------------------------------------------------
// System layer — normally in system_sdl.go.
// ---------------------------------------------------------------------

type Window struct {
	fullscreen bool
}

// ---------------------------------------------------------------------
// Video layer — normally in video_ffmpeg.go.
// ---------------------------------------------------------------------

type bgVideo struct {
	texture Texture
}

// MixerCleared is called by stage.go when mixer is cleared. No-op for wasm.
func (b *bgVideo) MixerCleared() {}

func (bgv *bgVideo) Open(filename string, volume int, sm BgVideoScaleMode, sf BgVideoScaleFilter, loop bool) error {
	return nil
}
func (bgv *bgVideo) Tick() error      { return nil }
func (bgv *bgVideo) Close()           {}
func (bgv *bgVideo) Reset()           {}
func (bgv *bgVideo) SetVisible(on bool) {}
func (bgv *bgVideo) SetPlaying(on bool) {}

// ---------------------------------------------------------------------
// Misc — normally in main.go.
// ---------------------------------------------------------------------

// handlePanic mirrors main.go's deferred panic recoverer signature.
// On native, it dumps a crash log and shows a dialog. On js, console.log
// the panic and bail. Real impl in D5+ — for now, no-op.
func handlePanic(r interface{}) {}

// chk mirrors main.go's panic-on-error helper.
func chk(err error) {
	if err != nil {
		panic(err)
	}
}

// NewModifierKey mirrors input_sdl.go's bit-or constructor.
func NewModifierKey(ctrl, alt, shift bool) (mod sdlKeymod) {
	if ctrl {
		mod |= sdlKMOD_CTRL
	}
	if alt {
		mod |= sdlKMOD_ALT
	}
	if shift {
		mod |= sdlKMOD_SHIFT
	}
	return mod
}

// Window method stubs mirror system_sdl.go's *Window method set. Most
// no-op or return canvas defaults until the real DOM canvas bridge lands
// in D5+. GetSize returns 1280x720 as a sensible default.
func (w *Window) SwapBuffers() {
	// The engine's per-frame scheduler in system.go renderFrame()
	// already calls time.Sleep(diff) before each frame to hit the
	// configured fps. Adding another sleep here pushes the scheduler
	// into permanent frameSkip mode, which causes the title-menu
	// state to drop all queued draws (logo renders, then nothing).
	// Per-frame yield is not needed on wasm: Go's runtime yields to
	// the JS event loop on syscall/js calls (every gl.* invocation),
	// which RenderQuad makes thousands of times per frame.
}
func (w *Window) SetIcon(icons []image.Image)                                    {}
func (w *Window) SetSwapInterval(interval int)                                   {}
func (w *Window) GetSize() (int, int)                                            { return 1280, 720 }
func (w *Window) GetScaledViewportSize() (int32, int32, int32, int32)            { return 0, 0, 1280, 720 }
func (w *Window) GetClipboardString() string                                     { return "" }
func (w *Window) toggleFullscreen()                                              {}
func (w *Window) UpdateDebugFPS()                                                {}
func (w *Window) pollEvents()                                                    {}
func (w *Window) GLCreateContext() (interface{}, error)                          { return nil, nil }
func (w *Window) GLMakeCurrent(ctx interface{})                                  {}
func (w *Window) Close()                                                         {}
func (w *Window) shouldClose() bool                                              { return false }

// KeyToString and getJoystickKey mirror input_sdl.go.
func KeyToString(k Key) string                          { return "" }
func getJoystickKey(controllerIdx int) (string, int)    { return "", 0 }

// osPreferredLanguage matches util_{linux,windows,darwin}.go signature.
// On js, returns navigator.language via syscall/js. For now stub to "en".
func osPreferredLanguage() string { return "en" }

// -----------------------------------------------------------------------
// Audio subsystem: speaker package + xmpDecode stub.
// -----------------------------------------------------------------------

// AudioSink interface mirrors audio_sdl.go's interface for native builds.
type AudioSink interface {
	Init(sr SampleRate, bufferSize int) error
	Play(s Streamer)
	Lock()
	Unlock()
	Close() error
	FillAudio()
}

// Speaker type wraps the beep speaker API for wasm.
type Speaker struct{}

// SDLSpeaker is the wasm implementation of the audio speaker.
type SDLSpeaker struct{}

// Speaker instance matching beep's github.com/faiface/beep/speaker module.
// sound.go calls speaker.Lock(), speaker.Unlock(), speaker.Play(), speaker.Close().
var speaker AudioSink

// Lock synchronizes access to speaker state. Callers wrap state mutations in Lock/Unlock.
func (Speaker) Lock()   {}

// Unlock releases the speaker lock.
func (Speaker) Unlock() {}

// Play queues a streamer for playback. No-op for wasm.
func (Speaker) Play(s Streamer) {}

// Close stops the speaker. No-op for wasm.
func (Speaker) Close() error { return nil }

// Init initializes the speaker with sample rate and buffer size. No-op for wasm.
func (Speaker) Init(sampleRate SampleRate, bufferSize int) error { return nil }

// SDLSpeaker methods for audio_sdl.go compatibility.
func (s *SDLSpeaker) Init(sampleRate SampleRate, bufferSize int) error { return nil }
func (s *SDLSpeaker) FillAudio()                                        {}
func (s *SDLSpeaker) Play(st Streamer)                                  {}
func (s *SDLSpeaker) Lock()                                             {}
func (s *SDLSpeaker) Unlock()                                           {}
func (s *SDLSpeaker) Close() error                                      { return nil }

// xmpDecode stubs the XM decoder from sound_xm.go (//go:build !js).
// Returns a minimal StreamSeeker stub instead of an error to satisfy the type.
type stubXMStreamer struct{}

func (s *stubXMStreamer) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubXMStreamer) Err() error                              { return nil }
func (s *stubXMStreamer) Position() int                           { return 0 }
func (s *stubXMStreamer) Len() int                                { return 0 }
func (s *stubXMStreamer) Seek(p int) error                        { return nil }

func xmpDecode(f interface{}) (StreamSeeker, Format, error) {
	return &stubXMStreamer{}, Format{}, nil
}

// -----------------------------------------------------------------------
// System initialization stubs from system.go + util files.
// -----------------------------------------------------------------------

// Logcat writes a string to stdout (stub for system.go:375).
// util_desktop.go and util_android.go do fmt.Println; wasm also just prints.
func Logcat(s string) {
	fmt.Println(s)
}

// selectRenderer returns the WebGL renderer for wasm builds.
// The Renderer_WebGL and FontRenderer_WebGL are defined in render_webgl.go.
func selectRenderer(cfgVal string) (Renderer, FontRenderer) {
	return &Renderer_WebGL{name: "WebGL2"}, &FontRenderer_WebGL{}
}

// System.newWindow creates a window. Stub for system_sdl.go:22.
// The stub Window is created in the Input + Window stubs above.
func (s *System) newWindow(w, h int) (*Window, error) {
	return &Window{}, nil
}
