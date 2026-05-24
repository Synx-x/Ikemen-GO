//go:build js

// DOM keyboard input bridge. Runs in the browser context via syscall/js
// callbacks from window.addEventListener('keydown'/'keyup'). Feeds key events
// into a thread-safe queue for consumption by the engine.
package main

import (
	"sync"
)

// DomKeyEvent represents a single keyboard event from the DOM.
type DomKeyEvent struct {
	Code  string    // W3C code (e.g., "KeyA", "Enter", "Space")
	KeyVal int      // numeric key code (legacy, for reference)
	Down  bool      // true = keydown, false = keyup
	Mods  sdlKeymod // modifier bitmask (ctrl/alt/shift/gui)
}

// domKeyQueue is the queue of pending DOM keyboard events.
// Protected by domKeyMutex.
var (
	domKeyQueue []DomKeyEvent
	domKeyMutex sync.Mutex
)

// domKeyState tracks the current pressed/released state of each key code.
// Protected by domKeyMutex. This allows the engine to query whether a key
// is currently held down.
var domKeyState = map[string]bool{}

// DrainDomKeys pops all pending key events from the queue and returns them
// as a slice. Call this from the engine's input loop (D6.3+). The slice is
// safe to mutate after return. Under D6.1 this is for forward compatibility;
// D6.3 will call it from main input handler.
func DrainDomKeys() []DomKeyEvent {
	domKeyMutex.Lock()
	defer domKeyMutex.Unlock()

	if len(domKeyQueue) == 0 {
		return nil
	}

	result := make([]DomKeyEvent, len(domKeyQueue))
	copy(result, domKeyQueue)
	domKeyQueue = domKeyQueue[:0]

	return result
}

// IsDomKeyDown queries the current pressed state for a given key code.
// Returns true if the key has been pressed and not yet released.
func IsDomKeyDown(code string) bool {
	domKeyMutex.Lock()
	defer domKeyMutex.Unlock()
	return domKeyState[code]
}

// injectKeyFromDOM is the internal handler called by the JS bridge.
// Must be called only from syscall/js contexts (no need for goroutine safety,
// but domKeyMutex guards the queue and state).
func injectKeyFromDOM(code string, keyVal int, down bool, mods sdlKeymod) {
	domKeyMutex.Lock()
	defer domKeyMutex.Unlock()

	domKeyQueue = append(domKeyQueue, DomKeyEvent{
		Code:  code,
		KeyVal: keyVal,
		Down:  down,
		Mods:  mods,
	})

	// Update state map
	domKeyState[code] = down

	// Log to console for debugging and test verification
	action := "down"
	if !down {
		action = "up"
	}
	logConsole("[ikemen-wasm] key: " + code + " " + action)
}
