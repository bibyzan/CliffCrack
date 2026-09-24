// Package gfx holds the plain data types exchanged with the renderer: GPU
// resource handles, draw commands and per-frame parameters. It is pure Go, so
// scene code can build draw lists and be tested without the renderer DLL;
// package render re-exports these types and moves them across the C boundary.
package gfx

import "CliffCrack/engine/mathx"

// Mesh is a handle to vertex/index buffers in GPU memory. The zero value is "no mesh".
type Mesh uint32

// Texture is a handle to a sampled image. The zero value is a built-in white
// texture, so an untextured draw just uses its colour.
type Texture uint32

// DrawFlags select per-draw shading (values match R_DRAW_* in renderer.h).
type DrawFlags uint32

const (
	// DrawFlat shades each triangle with its own normal (a faceted, low-poly look).
	DrawFlat DrawFlags = 1 << iota
	// DrawSnow keeps the colour on faces that point up and turns steep faces to dark rock.
	DrawSnow
	// DrawUnlit uses the colour as-is (still fogged).
	DrawUnlit
	// DrawSky draws a procedural sky: the fog colour at the horizon blending to
	// the draw colour overhead, with a sun disc. Use it on an inside-out dome.
	DrawSky
)

// DrawCmd draws one mesh. Its layout must match RDrawCmd in renderer.h
// (checked at startup by package render).
type DrawCmd struct {
	Model   mathx.Mat4 // object-to-world transform
	Color   [4]float32 // linear RGBA, multiplied with the texture
	Texture Texture
	Flags   DrawFlags
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
	UIProgress
	UISameLine
)

// UIWindowFlags are window options (values match R_UI_WINDOW_* in renderer.h).
type UIWindowFlags uint32

const (
	// UIOverlay drops the title bar and stops the window moving, resizing or collapsing.
	UIOverlay UIWindowFlags = 1 << iota
	// UIAnchored places the window every frame at X, Y given as fractions of the
	// screen; the same fraction of the window sits on that point (0.5, 0.5 centres it).
	UIAnchored
	// UINoBackground makes the window transparent; its text gets a drop shadow.
	UINoBackground
	// UICentered centres each line of text and each button in the window.
	UICentered
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

// FrameParams is the per-frame scene state: camera, lighting and fog.
type FrameParams struct {
	ViewProj     mathx.Mat4
	CameraPos    mathx.Vec3
	SunDirection mathx.Vec3 // towards the light; normalised by the shader
	SunColor     mathx.Vec3 // linear RGB * intensity
	Ambient      mathx.Vec3 // linear RGB
	FogColor     mathx.Vec3 // linear RGB haze, also the sky's horizon colour
	FogDensity   float32    // per world unit; 0 disables fog
	Clear        [4]float32 // linear RGBA
}
