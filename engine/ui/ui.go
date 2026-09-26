// Package ui builds the debug UI command list (pure Go). Widgets are
// immediate-mode: describe them every frame, bind them to your variables, and
// Apply writes the user's edits back after the renderer has drawn them.
//
//	b.Reset()
//	b.Window("Settings", 10, 10)
//	b.Slider("Sun", &sun, 0, 3)
//	if b.Button("Reset") { ... } // true on the frame after the click
//	b.End()
//	... render.UI(input, b.Cmds, b.Labels) ...
//	b.Apply()
package ui

import (
	"fmt"

	"CliffCrack/engine/gfx"
)

type binding struct {
	cmd int
	f   *float32
	b   *bool
}

// Builder accumulates one frame of UI. The zero value is ready to use.
type Builder struct {
	Cmds   []gfx.UICmd
	Labels []byte // all label text, referenced by offset/length from Cmds

	bindings []binding
	buttons  []int           // indices of button commands this frame
	clicked  map[string]bool // button labels clicked last frame
}

// Reset starts a new frame, keeping allocated memory.
func (b *Builder) Reset() {
	b.Cmds = b.Cmds[:0]
	b.Labels = b.Labels[:0]
	b.bindings = b.bindings[:0]
	b.buttons = b.buttons[:0]
}

func (b *Builder) add(kind gfx.UICmdKind, label string) *gfx.UICmd {
	b.Cmds = append(b.Cmds, gfx.UICmd{
		Kind:        kind,
		LabelOffset: uint32(len(b.Labels)),
		LabelLength: uint32(len(label)),
	})
	b.Labels = append(b.Labels, label...)
	return &b.Cmds[len(b.Cmds)-1]
}

// Window starts a window. x, y position it the first time it appears (0, 0 = automatic).
// Labels are also widget IDs; use "Name##id" to show the same text twice.
func (b *Builder) Window(title string, x, y float32) {
	c := b.add(gfx.UIWindow, title)
	c.X, c.Y = x, y
}

// Panel starts a window with options, e.g. a centred title card:
//
//	b.Panel("##title", 0.5, 0.3, gfx.UIOverlay|gfx.UIAnchored, 3)
//
// With gfx.UIAnchored, x and y are fractions of the screen (see gfx.UIAnchored);
// otherwise they are an initial position in pixels. scale enlarges the text
// (0 or 1 = normal); the font is rasterised at that size, so it stays sharp.
func (b *Builder) Panel(title string, x, y float32, flags gfx.UIWindowFlags, scale float32) {
	c := b.add(gfx.UIWindow, title)
	c.X, c.Y = x, y
	c.Value = float32(flags)
	c.Max = scale
}

func (b *Builder) End() { b.add(gfx.UIEnd, "") }

func (b *Builder) Separator() { b.add(gfx.UISeparator, "") }

// Text adds a line of text (fmt.Sprintf formatting).
func (b *Builder) Text(format string, args ...any) {
	b.add(gfx.UIText, fmt.Sprintf(format, args...))
}

// ColorText adds a line of text in a colour (linear RGBA, e.g. mathx.Hex(...)).
func (b *Builder) ColorText(color [4]float32, format string, args ...any) {
	c := b.add(gfx.UIText, fmt.Sprintf(format, args...))
	c.X, c.Y, c.Min, c.Max = color[0], color[1], color[2], max(color[3], 1e-3)
}

// Progress adds a bar filled to fraction (0..1), width x height pixels
// (0 = default), with label written over it.
func (b *Builder) Progress(label string, fraction, width, height float32) {
	c := b.add(gfx.UIProgress, label)
	c.Value, c.Min, c.Max = fraction, width, height
}

// GaugeStyle describes a dial: its width (the half circle's diameter) in
// pixels (0 = 200) and where its red zone starts, as a fraction of the range
// (0 = none).
type GaugeStyle struct {
	Size    float32
	RedFrom float32
}

// Gauge draws a speedometer: a half circle standing on its base, filled up
// to value within [min, max], with the value in the middle and unit under it.
func (b *Builder) Gauge(unit string, value, min, max float32, style GaugeStyle) {
	c := b.add(gfx.UIGauge, unit)
	c.Value, c.Min, c.Max = value, min, max
	c.X, c.Y = style.Size, style.RedFrom
}

// Circle draws a disc (width 0) or a ring width pixels wide behind every
// window, centred at x, y with radius r in pixels, in an sRGB colour, with
// label written in the middle at textSize pixels (0 = the normal size). It
// is for on-screen touch controls, which are laid out in screen pixels.
func (b *Builder) Circle(label string, x, y, r, width float32, srgb [4]float32, textSize float32) {
	c := b.add(gfx.UICircle, label)
	c.X, c.Y, c.Value, c.Min, c.Max = x, y, r, width, textSize
	c.Result = packSRGB(srgb)
}

// packSRGB packs a colour as 0xAABBGGRR, for the renderer's circle and image commands.
func packSRGB(srgb [4]float32) uint32 {
	var p uint32
	for i, v := range srgb {
		p |= uint32(max(0, min(1, v))*255+0.5) << (8 * i)
	}
	return p
}

// Image draws texture tex (e.g. an icon) behind every window, w x h pixels
// centred at x, y, tinted with an sRGB colour: over the circles drawn before
// it, for icons on touch buttons.
func (b *Builder) Image(tex gfx.Texture, x, y, w, h float32, srgb [4]float32) {
	c := b.add(gfx.UIImage, "")
	c.X, c.Y, c.Min, c.Value, c.Max = x, y, w, h, float32(tex)
	c.Result = packSRGB(srgb)
}

// Icon draws texture tex inline in the current window, like a word of text:
// w x h pixels (the UI scale applies), tinted with an sRGB colour, centred
// on the line. Put it between words with SameLine.
func (b *Builder) Icon(tex gfx.Texture, w, h float32, srgb [4]float32) {
	c := b.add(gfx.UIIcon, "")
	c.Min, c.Value, c.Max = w, h, float32(tex)
	c.Result = packSRGB(srgb)
}

// SameLine keeps the next widget on the current line, spacing pixels after
// this one (0 = default).
func (b *Builder) SameLine(spacing float32) {
	b.add(gfx.UISameLine, "").Value = spacing
}

// Slider edits *v within [min, max].
func (b *Builder) Slider(label string, v *float32, min, max float32) {
	c := b.add(gfx.UISlider, label)
	c.Value, c.Min, c.Max = *v, min, max
	b.bindings = append(b.bindings, binding{cmd: len(b.Cmds) - 1, f: v})
}

// SliderStyle customises a slider: Format is the printf format its value is
// shown with (e.g. "%.0f°"), Width its length in pixels (0 = default), and
// Highlight marks it as the keyboard/gamepad selection.
type SliderStyle struct {
	Format    string
	Width     float32
	Highlight bool
}

// StyledSlider edits *v within [min, max], drawn as style says.
func (b *Builder) StyledSlider(label string, v *float32, min, max float32, style SliderStyle) {
	if style.Format != "" {
		label += "\x1f" + style.Format
	}
	c := b.add(gfx.UISlider, label)
	c.Value, c.Min, c.Max = *v, min, max
	c.Y = style.Width
	if style.Highlight {
		c.X = 1
	}
	b.bindings = append(b.bindings, binding{cmd: len(b.Cmds) - 1, f: v})
}

// Checkbox toggles *v.
func (b *Builder) Checkbox(label string, v *bool) {
	c := b.add(gfx.UICheckbox, label)
	if *v {
		c.Value = 1
	}
	b.bindings = append(b.bindings, binding{cmd: len(b.Cmds) - 1, b: v})
}

// Button reports whether this button was clicked on the previous frame (the
// click is only known after the frame is drawn).
func (b *Builder) Button(label string) bool {
	b.add(gfx.UIButton, label)
	b.buttons = append(b.buttons, len(b.Cmds)-1)
	return b.clicked[label]
}

// MenuButton is a Button with a size in pixels (0 = fit the label) that can
// be highlighted, e.g. to show the keyboard selection.
func (b *Builder) MenuButton(label string, width, height float32, highlight bool) bool {
	c := b.add(gfx.UIButton, label)
	c.Min, c.Max = width, height
	if highlight {
		c.Value = 1
	}
	b.buttons = append(b.buttons, len(b.Cmds)-1)
	return b.clicked[label]
}

// Apply writes edited values back to their variables and records button
// clicks for the next frame. Call it after the renderer has processed Cmds.
func (b *Builder) Apply() {
	for _, bd := range b.bindings {
		c := b.Cmds[bd.cmd]
		if c.Result == 0 {
			continue
		}
		if bd.f != nil {
			*bd.f = c.Value
		}
		if bd.b != nil {
			*bd.b = c.Value != 0
		}
	}
	clear(b.clicked)
	for _, i := range b.buttons {
		if b.Cmds[i].Result != 0 {
			if b.clicked == nil {
				b.clicked = map[string]bool{}
			}
			c := b.Cmds[i]
			b.clicked[string(b.Labels[c.LabelOffset:c.LabelOffset+c.LabelLength])] = true
		}
	}
}
