package input

import (
	"math"
	"testing"
)

func TestPadButtonsAndEdges(t *testing.T) {
	var s State
	s.NewFrame()
	s.PadEvent(PadA, true)
	if !s.PadDown(PadA) || !s.PadPressed(PadA) {
		t.Fatal("A should be down and pressed on its first frame")
	}
	if !s.UsingPad() {
		t.Error("a pad press should switch prompts to the pad")
	}
	s.NewFrame()
	if !s.PadDown(PadA) || s.PadPressed(PadA) {
		t.Error("A held: down but not pressed again")
	}
	s.KeyEvent(KeySpace, true)
	if s.UsingPad() {
		t.Error("a key press should switch prompts back to the keyboard")
	}
	s.ReleaseAll()
	if s.PadDown(PadA) {
		t.Error("ReleaseAll should release pad buttons")
	}
}

func TestDeadzones(t *testing.T) {
	var s State
	s.PadAxisEvent(PadLeftX, 0.15)
	s.PadAxisEvent(PadLeftTrigger, 0.03)
	if v := s.PadAxis(PadLeftX); v != 0 {
		t.Errorf("stick drift reads %v, want 0", v)
	}
	if v := s.PadAxis(PadLeftTrigger); v != 0 {
		t.Errorf("resting trigger reads %v, want 0", v)
	}
	s.PadAxisEvent(PadLeftX, -1)
	s.PadAxisEvent(PadRightTrigger, 1)
	if v := s.PadAxis(PadLeftX); v != -1 {
		t.Errorf("full tilt reads %v, want -1", v)
	}
	if v := s.PadAxis(PadRightTrigger); v != 1 {
		t.Errorf("full trigger reads %v, want 1", v)
	}
	// Radial: a diagonal at full tilt keeps length ~1 and its direction.
	s.PadAxisEvent(PadLeftX, 0.7071)
	s.PadAxisEvent(PadLeftY, 0.7071)
	x, y := s.PadStick(false)
	if l := math.Hypot(float64(x), float64(y)); math.Abs(l-1) > 0.01 || math.Abs(float64(x-y)) > 1e-5 {
		t.Errorf("diagonal = (%v, %v)", x, y)
	}
}

func TestStickFlickIsAnEdge(t *testing.T) {
	var s State
	s.NewFrame()
	s.PadAxisEvent(PadLeftY, 0.9)
	if !s.PadFlicked(PadLeftY, 1) || s.PadFlicked(PadLeftY, -1) {
		t.Fatal("pushing the stick down should flick down only")
	}
	s.NewFrame()
	if s.PadFlicked(PadLeftY, 1) {
		t.Error("holding the stick should not repeat the flick")
	}
}
