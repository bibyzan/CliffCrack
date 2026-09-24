// Package render is the Go side of the renderer boundary. It is a thin wrapper
// over renderer/include/renderer.h; all Vulkan work happens in renderer.dll.
package render

/*
#cgo CFLAGS: -I${SRCDIR}/../../renderer/include
#cgo LDFLAGS: -L${SRCDIR}/../../build/bin -lrenderer
#include <stdlib.h>
#include "renderer.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// DrawCmd draws one instance of the built-in test triangle.
// It must match RDrawCmd in renderer.h byte for byte.
type DrawCmd struct {
	MVP   [16]float32 // column-major, Vulkan clip space
	Color [4]float32
}

func init() {
	if got, want := unsafe.Sizeof(DrawCmd{}), uintptr(C.sizeof_RDrawCmd); got != want {
		panic(fmt.Sprintf("render.DrawCmd is %d bytes, RDrawCmd is %d", got, want))
	}
}

type Config struct {
	Window        unsafe.Pointer // native window handle (HWND on Windows)
	Width, Height int            // framebuffer size in pixels
	ShaderDir     string         // directory holding the compiled *.spv files
	Validation    bool           // enable Vulkan validation layers if installed
	VSync         bool
}

func Init(cfg Config) error {
	shaderDir := C.CString(cfg.ShaderDir)
	defer C.free(unsafe.Pointer(shaderDir))

	desc := C.RInitDesc{
		native_window:     cfg.Window,
		width:             C.uint32_t(cfg.Width),
		height:            C.uint32_t(cfg.Height),
		shader_dir:        shaderDir,
		enable_validation: cBool(cfg.Validation),
		vsync:             cBool(cfg.VSync),
	}
	if C.r_init(&desc) == 0 {
		return fmt.Errorf("renderer init: %w", lastError())
	}
	return nil
}

func Resize(width, height int) {
	C.r_resize(C.uint32_t(width), C.uint32_t(height))
}

// BeginFrame returns false when the frame should be skipped; Draw and EndFrame
// must only be called after it returns true.
func BeginFrame(clear [4]float32) bool {
	return C.r_begin_frame((*C.float)(&clear[0])) != 0
}

// Draw submits the whole draw list in a single cgo call.
func Draw(cmds []DrawCmd) {
	if len(cmds) == 0 {
		return
	}
	C.r_draw((*C.RDrawCmd)(unsafe.Pointer(&cmds[0])), C.uint32_t(len(cmds)))
}

func EndFrame() {
	C.r_end_frame()
}

func Shutdown() {
	C.r_shutdown()
}

func lastError() error {
	return errors.New(C.GoString(C.r_last_error()))
}

func cBool(b bool) C.int32_t {
	if b {
		return 1
	}
	return 0
}
