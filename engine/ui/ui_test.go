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
