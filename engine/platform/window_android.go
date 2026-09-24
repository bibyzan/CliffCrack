//go:build android

// Package platform owns the OS window, input events (keyboard, mouse,
// gamepad) and time: via GLFW on desktop, via NativeActivity on Android.
package platform

/*
#cgo LDFLAGS: -landroid -llog
#include "android.h"
*/
import "C"

import (
	"errors"
	"os"
	"time"
	"unsafe"

	"CliffCrack/engine/input"
)

// Window is the activity's native window. Android owns it: it can go away
// (app switched out) and come back, which OnNativeWindow reports.
type Window struct {
	input       input.State
	closing     bool
	onNative    func(unsafe.Pointer)
	onResize    func(width, height int)
	touching    bool
	width       int
	height      int
	hatX, hatY  float32
	digitalTrig [2]bool // controllers that report triggers as buttons only
}

var (
	current *Window
	start   = time.Now()
	mainFn  func()
)

// SetMain registers the game's entry point. On Android the process starts in
// NativeActivity, not in Go's main; it calls this function on the activity's
// game thread.
func SetMain(fn func()) { mainFn = fn }

//export ccMain
func ccMain() {
	if mainFn != nil {
		mainFn()
	}
}

//export ccNativeWindow
func ccNativeWindow(win *C.ANativeWindow) {
	if current != nil && current.onNative != nil {
		current.onNative(unsafe.Pointer(win))
	}
}

// NewWindow waits for the activity's native window. The title and size are
// ignored: the game is fullscreen.
func NewWindow(title string, width, height int) (*Window, error) {
	w := &Window{}
	current = w
	for C.cc_app.window == nil {
		if C.cc_poll(100) != 0 {
			return nil, errors.New("activity destroyed before its window appeared")
		}
	}
	w.updateSize()
	return w, nil
}

func (w *Window) updateSize() {
	if win := C.cc_app.window; win != nil {
		w.width = int(C.ANativeWindow_getWidth(win))
		w.height = int(C.ANativeWindow_getHeight(win))
	}
}

func (w *Window) Destroy() {}

// Input is the window's input state. Call NewFrame on it before PollEvents.
func (w *Window) Input() *input.State { return &w.input }

func (w *Window) ShouldClose() bool     { return w.closing || C.cc_app.destroyRequested != 0 }
func (w *Window) SetShouldClose(v bool) { w.closing = v }

// NativeHandle is the ANativeWindow the renderer creates its surface from.
func (w *Window) NativeHandle() unsafe.Pointer { return unsafe.Pointer(C.cc_app.window) }

// OnNativeWindow registers fn, called when Android replaces or removes the
// native window (nil). It runs before the old window is released, so the
// renderer can drop its surface in time.
func (w *Window) OnNativeWindow(fn func(unsafe.Pointer)) { w.onNative = fn }

// FramebufferSize is the window's size in pixels (0, 0 while it's gone).
func (w *Window) FramebufferSize() (int, int) {
	if C.cc_app.window == nil {
		return 0, 0
	}
	return w.width, w.height
}

func (w *Window) OnFramebufferResize(fn func(width, height int)) { w.onResize = fn }

// There is no cursor to lock on Android.
func (w *Window) SetCursorLocked(bool)                          {}
func (w *Window) CursorLocked() bool                            { return false }
func (w *Window) ToFramebuffer(x, y float64) (float64, float64) { return x, y }

// PollEvents processes pending activity events and input.
func PollEvents() { poll(0) }

// WaitEvents blocks briefly for events (e.g. while the app is in the background).
func WaitEvents() { poll(100) }

func poll(timeoutMS int) {
	C.cc_poll(C.int(timeoutMS))
	w := current
	if w == nil {
		return
	}
	var events [C.CC_MAX_EVENTS]C.CCEvent
	n := int(C.cc_take_events(&events[0], C.CC_MAX_EVENTS))
	for _, e := range events[:n] {
		w.handle(e)
	}
}

func (w *Window) handle(e C.CCEvent) {
	switch e._type {
	case C.CC_EVENT_KEY:
		down := e.value != 0
		if b, ok := padButtons[int(e.code)]; ok {
			w.input.PadEvent(b, down)
			return
		}
		switch e.code {
		case 104, 105: // L2, R2 as buttons
			i := int(e.code) - 104
			w.digitalTrig[i] = down
			v := float32(0)
			if down {
				v = 1
			}
			w.input.PadAxisEvent(input.PadLeftTrigger+input.PadAxis(i), v)
			return
		}
		if k, ok := keyFor(int(e.code)); ok {
			w.input.KeyEvent(k, down)
		}
	case C.CC_EVENT_AXIS:
		a := input.PadAxis(e.code)
		if (a == input.PadLeftTrigger || a == input.PadRightTrigger) && w.digitalTrig[a-input.PadLeftTrigger] {
			return // the button already holds it at 1
		}
		w.input.PadAxisEvent(a, float32(e.value))
	case C.CC_EVENT_HAT:
		x, y := float32(e.value), float32(e.value2)
		if x != w.hatX || y != w.hatY {
			w.hatX, w.hatY = x, y
			w.input.PadEvent(input.PadLeft, x < -0.5)
			w.input.PadEvent(input.PadRight, x > 0.5)
			w.input.PadEvent(input.PadUp, y < -0.5)
			w.input.PadEvent(input.PadDown, y > 0.5)
		}
	case C.CC_EVENT_TOUCH:
		touching := e.code != 0
		w.input.MoveEvent(float64(e.value), float64(e.value2))
		if touching != w.touching {
			w.touching = touching
			w.input.ButtonEvent(input.MouseLeft, touching)
		}
	case C.CC_EVENT_RESIZE:
		w.updateSize()
		if w.onResize != nil {
			w.onResize(w.width, w.height)
		}
	case C.CC_EVENT_FOCUS:
		if e.code == 0 {
			w.input.ReleaseAll()
		}
	}
}

// padButtons maps Android gamepad key codes to pad buttons.
var padButtons = map[int]input.PadButton{
	96:  input.PadA, // BUTTON_A
	97:  input.PadB,
	99:  input.PadX,
	100: input.PadY,
	102: input.PadLB, // L1
	103: input.PadRB,
	106: input.PadLStick, // THUMBL
	107: input.PadRStick,
	108: input.PadStart,
	109: input.PadBack,  // SELECT
	110: input.PadGuide, // MODE
}

// keyFor maps Android key codes to keys (GLFW numbering). The back button is
// Escape; the d-pad keys are the arrows.
func keyFor(code int) (input.Key, bool) {
	switch {
	case code >= 29 && code <= 54: // A..Z
		return input.KeyA + input.Key(code-29), true
	case code >= 7 && code <= 16: // 0..9
		return input.Key0 + input.Key(code-7), true
	}
	k, ok := map[int]input.Key{
		4:   input.KeyEscape, // BACK
		111: input.KeyEscape,
		62:  input.KeySpace,
		66:  input.KeyEnter,
		160: input.KeyEnter, // NUMPAD_ENTER
		23:  input.KeyEnter, // DPAD_CENTER
		61:  input.KeyTab,
		67:  input.KeyBackspace,
		19:  input.KeyUp,
		20:  input.KeyDown,
		21:  input.KeyLeft,
		22:  input.KeyRight,
		59:  input.KeyLeftShift,
		60:  input.KeyRightShift,
		131: input.KeyF1,
		142: input.KeyF12,
	}[code]
	return k, ok
}

// Time returns seconds since the platform started.
func Time() float64 { return time.Since(start).Seconds() }

// ShaderDir is where the shaders were extracted from the APK's assets.
func ShaderDir() (string, error) { return C.GoString(&C.cc_shader_dir[0]), nil }

// UIScale enlarges the UI for the dense handheld screen: a 1080p panel a few
// inches across, held closer than a monitor.
func UIScale() float32 { return float32(C.cc_density()) * 0.6 }

// Exit finishes the activity and ends the process once the game loop has
// returned, so the next launch starts clean.
func Exit() {
	C.cc_finish()
	os.Exit(0)
}
