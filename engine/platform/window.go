// Package platform owns the OS window, input events and time, via GLFW.
package platform

import (
	"fmt"
	"runtime"

	"github.com/go-gl/glfw/v3.3/glfw"

	"vkgame/engine/input"
)

// GLFW (and the Win32 message loop) must stay on the main OS thread.
func init() {
	runtime.LockOSThread()
}

type Window struct {
	win          *glfw.Window
	input        input.State
	cursorLocked bool
}

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
	w := &Window{win: win}

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

func PollEvents() { glfw.PollEvents() }
func WaitEvents() { glfw.WaitEvents() }

// Time returns seconds since the platform was initialised.
func Time() float64 { return glfw.GetTime() }
