// Package gfx holds the plain data types exchanged with the renderer: GPU
// resource handles, draw commands and per-frame parameters. It is pure Go, so
// scene code can build draw lists and be tested without the renderer DLL;
// package render re-exports these types and moves them across the C boundary.
package gfx

import "vkgame/engine/mathx"

// Mesh is a handle to vertex/index buffers in GPU memory. The zero value is "no mesh".
type Mesh uint32

// Texture is a handle to a sampled image. The zero value is a built-in white
// texture, so an untextured draw just uses its colour.
type Texture uint32

// DrawCmd draws one mesh. Its layout must match RDrawCmd in renderer.h
// (checked at startup by package render).
type DrawCmd struct {
	Model   mathx.Mat4 // object-to-world transform
	Color   [4]float32 // linear RGBA, multiplied with the texture
	Texture Texture
	Mesh    Mesh
}

// UICmdKind selects a debug-UI widget (values match R_UI_* in renderer.h).
type UICmdKind uint32

const (
	UIWindow UICmdKind = iota + 1
	UIEnd
	UIText
	UISlider
	UICheckbox
	UIButton
	UISeparator
)

// UICmd is one debug-UI command. Its layout must match RUICmd in renderer.h.
// Labels are byte ranges in a shared text buffer; Result and Value are written
// back by the renderer.
type UICmd struct {
	Kind        UICmdKind
	LabelOffset uint32
	LabelLength uint32
	Result      uint32 // 1 if changed/clicked this frame
	Value       float32
	Min, Max    float32
	X, Y        float32 // window position hint
}

// UIInput is the mouse state the debug UI sees. Layout must match RUIInput.
type UIInput struct {
	MouseX, MouseY float32 // framebuffer pixels; negative = no mouse
	Wheel          float32
	MouseButtons   uint32 // bit 0 left, 1 right, 2 middle
	DeltaTime      float32
}

// UIOutput reports what the debug UI is using. Layout must match RUIOutput.
type UIOutput struct {
	WantMouse    int32
	WantKeyboard int32
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
