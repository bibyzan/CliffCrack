package input

import "testing"

func TestKeyEdges(t *testing.T) {
	var s State

	s.NewFrame()
	s.KeyEvent(KeyW, true)
	if !s.Down(KeyW) || !s.Pressed(KeyW) || s.Released(KeyW) {
		t.Fatal("frame 1: W should be down and pressed")
	}

	s.NewFrame() // held, no new events
	if !s.Down(KeyW) || s.Pressed(KeyW) {
		t.Fatal("frame 2: W should be down but not newly pressed")
	}

	s.NewFrame()
	s.KeyEvent(KeyW, false)
	if s.Down(KeyW) || !s.Released(KeyW) {
		t.Fatal("frame 3: W should be released")
	}

	s.NewFrame()
	if s.Released(KeyW) {
		t.Fatal("frame 4: release edge should last one frame")
	}
}

func TestTapWithinOneFrame(t *testing.T) {
	var s State
	s.NewFrame()
	s.KeyEvent(KeySpace, true)
	s.KeyEvent(KeySpace, false)
	// A press and release inside one frame ends up "up"; it was never seen down.
	if s.Down(KeySpace) || s.Pressed(KeySpace) {
		t.Fatal("tap inside one frame should not register as down")
	}
}

func TestMouseDeltaAndReset(t *testing.T) {
	var s State
	s.NewFrame()
	s.MoveEvent(100, 100) // first position: no delta
	s.MoveEvent(110, 95)
	s.MoveEvent(115, 90)
	if dx, dy := s.MouseDelta(); dx != 15 || dy != -10 {
		t.Fatalf("delta = (%v, %v), want (15, -10)", dx, dy)
	}

	s.NewFrame()
	if dx, dy := s.MouseDelta(); dx != 0 || dy != 0 {
		t.Fatal("delta should reset each frame")
	}

	s.ResetMouse()
	s.MoveEvent(5000, 5000) // cursor mode change: big jump must be ignored
	if dx, dy := s.MouseDelta(); dx != 0 || dy != 0 {
		t.Fatalf("delta after reset = (%v, %v), want 0", dx, dy)
	}
}

func TestAxisScrollAndBounds(t *testing.T) {
	var s State
	s.NewFrame()
	s.KeyEvent(KeyA, true)
	if s.Axis(KeyA, KeyD) != -1 {
		t.Error("A alone should give -1")
	}
	s.KeyEvent(KeyD, true)
	if s.Axis(KeyA, KeyD) != 0 {
		t.Error("A and D together should cancel")
	}

	s.ScrollEvent(1)
	s.ScrollEvent(0.5)
	if s.Scroll() != 1.5 {
		t.Errorf("scroll = %v, want 1.5", s.Scroll())
	}

	// Out-of-range codes (GLFW_KEY_UNKNOWN is -1) must be ignored, not panic.
	s.KeyEvent(-1, true)
	s.KeyEvent(10000, true)
	if s.Down(-1) || s.Down(10000) {
		t.Error("invalid keys should never be down")
	}

	s.ButtonEvent(MouseRight, true)
	s.ReleaseAll()
	if s.MouseDown(MouseRight) || s.Down(KeyA) {
		t.Error("ReleaseAll should clear keys and buttons")
	}
}
