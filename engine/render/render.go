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

	"vkgame/engine/geom"
	"vkgame/engine/mathx"
)

// Mesh is a handle to vertex/index buffers in GPU memory. The zero value is "no mesh".
type Mesh uint32

// DrawCmd draws one mesh. It must match RDrawCmd in renderer.h byte for byte.
type DrawCmd struct {
	MVP          mathx.Mat4  // column-major, Vulkan clip space
	NormalMatrix [12]float32 // see mathx.NormalMatrix
	Color        [4]float32
	Mesh         Mesh
}

func init() {
	checkSize("render.DrawCmd", unsafe.Sizeof(DrawCmd{}), uintptr(C.sizeof_RDrawCmd))
	checkSize("geom.Vertex", unsafe.Sizeof(geom.Vertex{}), uintptr(C.sizeof_RVertex))
}

func checkSize(name string, got, want uintptr) {
	if got != want {
		panic(fmt.Sprintf("%s is %d bytes but its C twin is %d", name, got, want))
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

// CreateMesh uploads mesh data to the GPU (blocking). The data is copied, so
// the caller may reuse or drop it afterwards.
func CreateMesh(m geom.MeshData) (Mesh, error) {
	if len(m.Vertices) == 0 || len(m.Indices) == 0 {
		return 0, errors.New("create mesh: empty mesh data")
	}
	h := C.r_create_mesh(
		(*C.RVertex)(unsafe.Pointer(&m.Vertices[0])), C.uint32_t(len(m.Vertices)),
		(*C.uint32_t)(unsafe.Pointer(&m.Indices[0])), C.uint32_t(len(m.Indices)))
	if h == 0 {
		return 0, fmt.Errorf("create mesh: %w", lastError())
	}
	return Mesh(h), nil
}

// DestroyMesh frees a mesh. It stalls the GPU, so don't call it every frame.
func DestroyMesh(m Mesh) {
	C.r_destroy_mesh(C.RMesh(m))
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
