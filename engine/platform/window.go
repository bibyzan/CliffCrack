// Package platform owns the OS window, input and time, via GLFW.
package platform

import (
	"fmt"
	"runtime"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// GLFW (and the Win32 message loop) must stay on the main OS thread.
func init() {
	runtime.LockOSThread()
}

type Key = glfw.Key

const (
	KeyEscape = glfw.KeyEscape
	KeySpace  = glfw.KeySpace
)

type Window struct {
	win *glfw.Window
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
	return &Window{win: win}, nil
}

func (w *Window) Destroy() {
	w.win.Destroy()
	glfw.Terminate()
}

func (w *Window) ShouldClose() bool     { return w.win.ShouldClose() }
func (w *Window) SetShouldClose(v bool) { w.win.SetShouldClose(v) }
func (w *Window) KeyDown(k Key) bool    { return w.win.GetKey(k) == glfw.Press }

// FramebufferSize is the drawable size in pixels (differs from window size on high-DPI).
func (w *Window) FramebufferSize() (int, int) { return w.win.GetFramebufferSize() }

func (w *Window) OnFramebufferResize(fn func(width, height int)) {
	w.win.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		fn(width, height)
	})
}

func PollEvents() { glfw.PollEvents() }
func WaitEvents() { glfw.WaitEvents() }

// Time returns seconds since the platform was initialised.
func Time() float64 { return glfw.GetTime() }
