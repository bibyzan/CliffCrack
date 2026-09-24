package ui

import (
	"testing"

	"CliffCrack/engine/gfx"
)

func label(b *Builder, c gfx.UICmd) string {
	return string(b.Labels[c.LabelOffset : c.LabelOffset+c.LabelLength])
}

func TestBuildAndApply(t *testing.T) {
	var b Builder
	speed := float32(2)
	enabled := false

	build := func() bool {
		b.Reset()
		b.Window("Settings", 10, 20)
		b.Text("fps %d", 60)
		b.Slider("Speed", &speed, 0, 5)
		b.Checkbox("Enabled", &enabled)
		clicked := b.Button("Go")
		b.End()
		return clicked
	}

	if build() {
		t.Fatal("button can't be clicked before the first frame is drawn")
	}
	if len(b.Cmds) != 6 {
		t.Fatalf("got %d commands, want 6", len(b.Cmds))
	}
	want := []struct {
		kind  gfx.UICmdKind
		label string
	}{
		{gfx.UIWindow, "Settings"}, {gfx.UIText, "fps 60"}, {gfx.UISlider, "Speed"},
		{gfx.UICheckbox, "Enabled"}, {gfx.UIButton, "Go"}, {gfx.UIEnd, ""},
	}
	for i, w := range want {
		if b.Cmds[i].Kind != w.kind || label(&b, b.Cmds[i]) != w.label {
			t.Errorf("cmd %d = %v %q, want %v %q", i, b.Cmds[i].Kind, label(&b, b.Cmds[i]), w.kind, w.label)
		}
	}
	if c := b.Cmds[0]; c.X != 10 || c.Y != 20 {
		t.Errorf("window position = (%v, %v)", c.X, c.Y)
	}
	if c := b.Cmds[2]; c.Value != 2 || c.Min != 0 || c.Max != 5 {
		t.Errorf("slider = %+v", c)
	}

	// Pretend the renderer drew it: slider moved, checkbox ticked, button clicked.
	b.Cmds[2].Value, b.Cmds[2].Result = 3.5, 1
	b.Cmds[3].Value, b.Cmds[3].Result = 1, 1
	b.Cmds[4].Result = 1
	b.Apply()
	if speed != 3.5 || !enabled {
		t.Errorf("after Apply: speed=%v enabled=%v, want 3.5 true", speed, enabled)
	}

	if !build() {
		t.Error("button click should be reported on the next frame")
	}
	b.Apply() // no click this time
	if build() {
		t.Error("click should only be reported once")
	}
}

func TestUnchangedWidgetsKeepValues(t *testing.T) {
	var b Builder
	v := float32(1)
	b.Slider("v", &v, 0, 2)
	b.Cmds[0].Value = 1.5 // renderer echoed a value but reported no change
	b.Apply()
	if v != 1 {
		t.Errorf("v = %v, want unchanged 1", v)
	}
}

func TestPanelAndMenuButton(t *testing.T) {
	var b Builder
	build := func() bool {
		b.Reset()
		b.Panel("##menu", 0.5, 0.25, gfx.UIOverlay|gfx.UIAnchored, 2.5)
		clicked := b.MenuButton("Run", 240, 48, true)
		b.MenuButton("Quit", 0, 0, false)
		b.End()
		return clicked
	}
	build()
	if c := b.Cmds[0]; c.Kind != gfx.UIWindow || c.X != 0.5 || c.Y != 0.25 ||
		gfx.UIWindowFlags(c.Value) != gfx.UIOverlay|gfx.UIAnchored || c.Max != 2.5 {
		t.Errorf("panel = %+v", c)
	}
	if c := b.Cmds[1]; c.Min != 240 || c.Max != 48 || c.Value != 1 {
		t.Errorf("highlighted button = %+v", c)
	}
	if c := b.Cmds[2]; c.Value != 0 {
		t.Errorf("plain button should not be highlighted: %+v", c)
	}
	b.Cmds[1].Result = 1
	b.Apply()
	if !build() {
		t.Error("menu button click should be reported on the next frame")
	}
}

func TestColorTextProgressAndSameLine(t *testing.T) {
	var b Builder
	b.ColorText([4]float32{0.1, 0.2, 0.3, 0.5}, "hot %d", 1)
	b.SameLine(12)
	b.Progress("", 0.75, 200, 6)
	if c := b.Cmds[0]; c.Kind != gfx.UIText || label(&b, c) != "hot 1" ||
		c.X != 0.1 || c.Y != 0.2 || c.Min != 0.3 || c.Max != 0.5 {
		t.Errorf("colour text = %+v", c)
	}
	if c := b.Cmds[1]; c.Kind != gfx.UISameLine || c.Value != 12 {
		t.Errorf("same line = %+v", c)
	}
	if c := b.Cmds[2]; c.Kind != gfx.UIProgress || c.Value != 0.75 || c.Min != 200 || c.Max != 6 {
		t.Errorf("progress = %+v", c)
	}
	var plain Builder
	plain.Text("x")
	if plain.Cmds[0].Max != 0 {
		t.Error("plain text must not carry a colour")
	}
}
