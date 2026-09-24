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
	"image"
	"unsafe"

	"vkgame/engine/geom"
	"vkgame/engine/mathx"
)

// Mesh is a handle to vertex/index buffers in GPU memory. The zero value is "no mesh".
type Mesh uint32

// Texture is a handle to a sampled image. The zero value is a built-in white
// texture, so an untextured draw just uses its colour.
type Texture uint32

// DrawCmd draws one mesh. It must match RDrawCmd in renderer.h byte for byte.
type DrawCmd struct {
	Model   mathx.Mat4 // object-to-world transform
	Color   [4]float32 // linear RGBA, multiplied with the texture
	Texture Texture
	Mesh    Mesh
}

// FrameParams is the per-frame scene state: camera and lighting.
type FrameParams struct {
	ViewProj     mathx.Mat4
	CameraPos    mathx.Vec3
	SunDirection mathx.Vec3 // towards the light; normalised by the shader
	SunColor     mathx.Vec3 // linear RGB * intensity
	Ambient      mathx.Vec3 // linear RGB
	Clear        [4]float32 // linear RGBA
}

// frameParams mirrors RFrameParams in renderer.h byte for byte.
type frameParams struct {
	viewProj     mathx.Mat4
	cameraPos    [4]float32
	sunDirection [4]float32
	sunColor     [4]float32
	ambient      [4]float32
	clear        [4]float32
}

func init() {
	checkSize("render.DrawCmd", unsafe.Sizeof(DrawCmd{}), uintptr(C.sizeof_RDrawCmd))
	checkSize("render.frameParams", unsafe.Sizeof(frameParams{}), uintptr(C.sizeof_RFrameParams))
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

// CreateTexture uploads an image and builds its mip chain (blocking). Set srgb
// for colour data (base colour maps, UI); leave it off for data such as normal
// maps. The pixels are copied.
func CreateTexture(img *image.NRGBA, srgb bool) (Texture, error) {
	b := img.Bounds()
	if b.Empty() {
		return 0, errors.New("create texture: empty image")
	}
	if b.Min != (image.Point{}) || img.Stride != 4*b.Dx() {
		return 0, errors.New("create texture: image must be tightly packed and start at (0,0)")
	}
	var flags C.uint32_t
	if srgb {
		flags |= C.R_TEXTURE_SRGB
	}
	h := C.r_create_texture((*C.uint8_t)(unsafe.Pointer(&img.Pix[0])), C.uint32_t(b.Dx()), C.uint32_t(b.Dy()), flags)
	if h == 0 {
		return 0, fmt.Errorf("create texture: %w", lastError())
	}
	return Texture(h), nil
}

// DestroyTexture frees a texture. It stalls the GPU, so don't call it every frame.
func DestroyTexture(t Texture) {
	C.r_destroy_texture(C.RTexture(t))
}

// BeginFrame returns false when the frame should be skipped; Draw and EndFrame
// must only be called after it returns true.
func BeginFrame(p FrameParams) bool {
	c := frameParams{
		viewProj:     p.ViewProj,
		cameraPos:    vec4(p.CameraPos),
		sunDirection: vec4(p.SunDirection),
		sunColor:     vec4(p.SunColor),
		ambient:      vec4(p.Ambient),
		clear:        p.Clear,
	}
	return C.r_begin_frame((*C.RFrameParams)(unsafe.Pointer(&c))) != 0
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

// CaptureNextFrame asks for the next completed frame to be read back; collect
// it with ReadCapture right after that frame's EndFrame.
func CaptureNextFrame() {
	C.r_capture_next_frame()
}

// ReadCapture waits for the captured frame and returns it as sRGB RGBA pixels.
func ReadCapture() (*image.NRGBA, error) {
	var w, h C.uint32_t
	if C.r_read_capture(nil, 0, &w, &h) == 0 {
		return nil, fmt.Errorf("read capture: %w", lastError())
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	if C.r_read_capture((*C.uint8_t)(unsafe.Pointer(&img.Pix[0])), C.uint32_t(len(img.Pix)), &w, &h) == 0 {
		return nil, fmt.Errorf("read capture: %w", lastError())
	}
	return img, nil
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

func vec4(v mathx.Vec3) [4]float32 {
	return [4]float32{v[0], v[1], v[2], 0}
}
