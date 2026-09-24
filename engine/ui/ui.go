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

func (b *Builder) End() { b.add(gfx.UIEnd, "") }

func (b *Builder) Separator() { b.add(gfx.UISeparator, "") }

// Text adds a line of text (fmt.Sprintf formatting).
func (b *Builder) Text(format string, args ...any) {
	b.add(gfx.UIText, fmt.Sprintf(format, args...))
}

// Slider edits *v within [min, max].
func (b *Builder) Slider(label string, v *float32, min, max float32) {
	c := b.add(gfx.UISlider, label)
	c.Value, c.Min, c.Max = *v, min, max
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
