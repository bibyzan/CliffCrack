//go:build ios

// Package platform owns the OS window, input events (keyboard, mouse,
// gamepad, touch) and time: via GLFW on desktop, NativeActivity on Android
// and UIKit on iOS.
package platform

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework UIKit -framework QuartzCore -framework CoreGraphics
#include "ios.h"
*/
import "C"

import (
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"CliffCrack/engine/input"
)

// Window is the app's one fullscreen view. UIKit runs on the main thread;
// the game loop runs on its own and reads UIKit's events from a queue.
type Window struct {
	input    input.State
	closing  bool
	onResize func(width, height int)
	width    int
	height   int
	primary  uint64 // the finger acting as the mouse (0 = none)
	// A finger lands as the mouse moving there, and presses a frame later
	// (the UI takes a press only on what the pointer was already over);
	// lifting it releases a frame after that at the earliest, since a quick
	// tap can land and lift between two frames.
	pressNext, releaseNext bool
}

var (
	current *Window
	start   = time.Now()
	mainFn  func()
)

// SetMain registers the game's entry point. The Go code is a static library
// in the app: its main never runs. UIKit owns the process's main thread (the
// app's main calls cc_run), and once the app has launched it calls this
// function on a thread of its own.
func SetMain(fn func()) { mainFn = fn }

//export ccMain
func ccMain() {
	if mainFn != nil {
		mainFn()
	}
}

// NewWindow waits for the view to have a size. The title and size are
// ignored: the game is fullscreen.
func NewWindow(title string, width, height int) (*Window, error) {
	w := &Window{}
	w.input.PreferTouch()
	current = w
	for w.updateSize(); w.width == 0 || w.height == 0; w.updateSize() {
		C.cc_wait(100)
	}
	return w, nil
}

func (w *Window) updateSize() {
	var width, height C.int
	C.cc_size(&width, &height)
	w.width, w.height = int(width), int(height)
}

func (w *Window) Destroy() {}

// Input is the window's input state. Call NewFrame on it before PollEvents.
func (w *Window) Input() *input.State { return &w.input }

func (w *Window) ShouldClose() bool     { return w.closing }
func (w *Window) SetShouldClose(v bool) { w.closing = v }

// NativeHandle is the view's CAMetalLayer, which the renderer makes its
// surface from.
func (w *Window) NativeHandle() unsafe.Pointer { return C.cc_layer() }

// The layer lives as long as the app, so there's nothing to replace.
func (w *Window) OnNativeWindow(func(unsafe.Pointer)) {}

// FramebufferSize is the view's size in pixels, or 0, 0 while the app is in
// the background (iOS doesn't let it draw there).
func (w *Window) FramebufferSize() (int, int) {
	if C.cc_active() == 0 {
		return 0, 0
	}
	return w.width, w.height
}

func (w *Window) OnFramebufferResize(fn func(width, height int)) { w.onResize = fn }

// There is no cursor to lock on a phone.
func (w *Window) SetCursorLocked(bool)                          {}
func (w *Window) CursorLocked() bool                            { return false }
func (w *Window) ToFramebuffer(x, y float64) (float64, float64) { return x, y }

// PollEvents processes the touches and changes UIKit has queued.
func PollEvents() { poll() }

// WaitEvents blocks briefly for events (e.g. while the app is in the background).
func WaitEvents() {
	C.cc_wait(100)
	poll()
}

func poll() {
	w := current
	if w == nil {
		return
	}
	switch {
	case w.pressNext:
		w.input.ButtonEvent(input.MouseLeft, true)
		w.pressNext = false
	case w.releaseNext:
		w.input.ButtonEvent(input.MouseLeft, false)
		w.releaseNext = false
	}
	var events [C.CC_MAX_EVENTS]C.CCEvent
	n := int(C.cc_take_events(&events[0], C.CC_MAX_EVENTS))
	for _, e := range events[:n] {
		w.handle(e)
	}
}

func (w *Window) handle(e C.CCEvent) {
	switch e._type {
	case C.CC_EVENT_TOUCH:
		id, x, y := uint64(e.id), float64(e.value), float64(e.value2)
		phase := input.TouchPhase(e.code)
		w.input.TouchEvent(id, x, y, phase)
		// The first finger down is also the mouse, so menus work by tapping.
		if phase == input.TouchBegan && w.primary == 0 {
			w.primary = id
			w.input.MoveEvent(x, y)
			w.pressNext = true
		} else if id == w.primary {
			w.input.MoveEvent(x, y)
			if phase == input.TouchEnded {
				if w.pressNext || w.input.MousePressed(input.MouseLeft) {
					w.releaseNext = true // not pressed yet, or only just
				} else {
					w.input.ButtonEvent(input.MouseLeft, false)
				}
				w.primary = 0
			}
		}
	case C.CC_EVENT_RESIZE:
		w.updateSize()
		if w.onResize != nil {
			w.onResize(w.width, w.height)
		}
	case C.CC_EVENT_FOCUS:
		if e.code == 0 {
			w.input.ReleaseAll()
			w.primary = 0
		}
	}
}

// Time returns seconds since the platform started.
func Time() float64 { return time.Since(start).Seconds() }

// ShaderDir is where the compiled shaders are: in the app bundle, next to
// the executable.
func ShaderDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "shaders"), nil
}

// DataDir is where the game keeps its files (settings): Application Support
// in the app's sandbox. It is created if needed.
func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "CliffCrack")
	return dir, os.MkdirAll(dir, 0o755)
}

// UIScale enlarges the UI for the phone's dense screen, held close.
func UIScale() float32 { return float32(C.cc_scale()) * 0.62 }

// TouchScreen reports whether the device has a touch screen for on-screen
// controls.
func TouchScreen() bool { return true }

// Exit ends the process once the game loop has returned (quitting from the
// menu), so the next launch starts clean.
func Exit() { os.Exit(0) }
