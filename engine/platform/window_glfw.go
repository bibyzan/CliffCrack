//go:build !android && !ios

// Package platform owns the OS window, input events (keyboard, mouse,
// gamepad, touch) and time: via GLFW on desktop, NativeActivity on Android
// and UIKit on iOS.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"

	"CliffCrack/engine/input"
)

// GLFW (and the Win32 message loop) must stay on the main OS thread.
func init() {
	runtime.LockOSThread()
}

type Window struct {
	win          *glfw.Window
	input        input.State
	cursorLocked bool
	pad          glfw.Joystick // the gamepad being read, or -1
}

// current is the window whose input PollEvents feeds with gamepad state.
var current *Window

func NewWindow(title string, width, height int) (*Window, error) {
	if err := glfw.Init(); err != nil {
		return nil, fmt.Errorf("glfw init: %w", err)
	}
	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI) // Vulkan: no OpenGL context
	glfw.WindowHint(glfw.Resizable, glfw.True)

	win, err := glfw.CreateWindow(width, height, title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return nil, fmt.Errorf("create window: %w", err)
	}
	w := &Window{win: win, pad: -1}
	current = w

	win.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
		w.input.KeyEvent(input.Key(key), action != glfw.Release)
	})
	win.SetMouseButtonCallback(func(_ *glfw.Window, b glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
		w.input.ButtonEvent(input.MouseButton(b), action != glfw.Release)
	})
	win.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		w.input.MoveEvent(x, y)
	})
	win.SetScrollCallback(func(_ *glfw.Window, _, dy float64) {
		w.input.ScrollEvent(dy)
	})
	win.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		if !focused {
			w.input.ReleaseAll() // key-up events are lost while unfocused
		}
	})
	return w, nil
}

func (w *Window) Destroy() {
	w.win.Destroy()
	glfw.Terminate()
}

// Input is the window's input state. Call NewFrame on it before PollEvents.
func (w *Window) Input() *input.State { return &w.input }

func (w *Window) ShouldClose() bool     { return w.win.ShouldClose() }
func (w *Window) SetShouldClose(v bool) { w.win.SetShouldClose(v) }

// SetCursorLocked hides the cursor and gives unbounded (raw, if supported)
// mouse motion, for mouse-look. Unlocking restores the normal cursor.
func (w *Window) SetCursorLocked(locked bool) {
	if locked == w.cursorLocked {
		return
	}
	w.cursorLocked = locked
	if locked {
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		if glfw.RawMouseMotionSupported() {
			w.win.SetInputMode(glfw.RawMouseMotion, glfw.True)
		}
	} else {
		w.win.SetInputMode(glfw.RawMouseMotion, glfw.False)
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
	w.input.ResetMouse() // the cursor position jumps when the mode changes
}

// FramebufferSize is the drawable size in pixels (differs from window size on high-DPI).
func (w *Window) FramebufferSize() (int, int) { return w.win.GetFramebufferSize() }

// CursorLocked reports whether SetCursorLocked(true) is in effect.
func (w *Window) CursorLocked() bool { return w.cursorLocked }

// ToFramebuffer converts a cursor position (window coordinates) to framebuffer pixels.
func (w *Window) ToFramebuffer(x, y float64) (float64, float64) {
	ww, wh := w.win.GetSize()
	fw, fh := w.win.GetFramebufferSize()
	if ww == 0 || wh == 0 {
		return x, y
	}
	return x * float64(fw) / float64(ww), y * float64(fh) / float64(wh)
}

func (w *Window) OnFramebufferResize(fn func(width, height int)) {
	w.win.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		fn(width, height)
	})
}

// PollEvents processes pending window events and reads the gamepad.
func PollEvents() {
	glfw.PollEvents()
	if current != nil {
		current.pollGamepad()
	}
}

func WaitEvents() { glfw.WaitEvents() }

// pollGamepad feeds the first connected gamepad's state into the input. GLFW
// maps controllers to a standard Xbox-style layout (it ships SDL's mapping
// database), so buttons and axes convert with plain casts.
func (w *Window) pollGamepad() {
	if w.pad < 0 || !w.pad.IsGamepad() {
		w.pad = -1
		for j := glfw.Joystick1; j <= glfw.JoystickLast; j++ {
			if j.IsGamepad() {
				w.pad = j
				break
			}
		}
		if w.pad < 0 {
			return
		}
	}
	state := w.pad.GetGamepadState()
	if state == nil {
		return
	}
	for b, action := range state.Buttons {
		w.input.PadEvent(input.PadButton(b), action == glfw.Press)
	}
	for a, v := range state.Axes {
		if a := input.PadAxis(a); a == input.PadLeftTrigger || a == input.PadRightTrigger {
			v = (v + 1) / 2 // GLFW triggers rest at -1
		}
		w.input.PadAxisEvent(input.PadAxis(a), v)
	}
}

// ShaderDir is where the compiled shaders are: next to the executable.
func ShaderDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "shaders"), nil
}

// DataDir is where the game keeps its files (settings): the user's config
// directory, e.g. %AppData%\CliffCrack on Windows. It is created if needed.
func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "CliffCrack")
	return dir, os.MkdirAll(dir, 0o755)
}

// UIScale is how much to enlarge the UI for the screen (1 on desktop).
func UIScale() float32 { return 1 }

// TouchScreen reports whether to show on-screen touch controls (iOS only).
func TouchScreen() bool { return false }

// OnNativeWindow registers a function called when the OS replaces or removes
// the native window (Android only; desktop windows live as long as the game).
func (w *Window) OnNativeWindow(func(unsafe.Pointer)) {}

// Exit ends the process once the game loop has returned (Android only).
func Exit() {}

// Time returns seconds since the platform was initialised.
func Time() float64 { return glfw.GetTime() }
