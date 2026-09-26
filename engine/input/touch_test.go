package input

import "testing"

func TestTouchLifecycle(t *testing.T) {
	var s State
	s.NewFrame()
	s.TouchEvent(7, 100, 200, TouchBegan)
	if ts := s.Touches(); len(ts) != 1 || !ts[0].Began || ts[0].StartX != 100 {
		t.Fatalf("frame 1: want one new touch at 100, got %+v", ts)
	}
	if !s.UsingTouch() {
		t.Fatal("a touch should switch to touch input")
	}

	s.NewFrame()
	s.TouchEvent(7, 130, 190, TouchMoved)
	ts := s.Touches()
	if len(ts) != 1 || ts[0].Began {
		t.Fatalf("frame 2: want the touch held, got %+v", ts)
	}
	if dx, dy := ts[0].Delta(); dx != 30 || dy != -10 {
		t.Fatalf("frame 2: delta %v, %v; want 30, -10", dx, dy)
	}

	s.NewFrame()
	if dx, dy := s.Touches()[0].Delta(); dx != 0 || dy != 0 {
		t.Fatalf("frame 3: a still finger should not move, got %v, %v", dx, dy)
	}
	s.TouchEvent(7, 130, 190, TouchEnded)
	if !s.Touches()[0].Ended {
		t.Fatal("frame 3: the lift should be visible this frame")
	}

	s.NewFrame()
	if len(s.Touches()) != 0 {
		t.Fatal("frame 4: the lifted finger should be gone")
	}
}

func TestTapInsideOneFrame(t *testing.T) {
	var s State
	s.NewFrame()
	s.TouchEvent(1, 10, 10, TouchBegan)
	s.TouchEvent(1, 10, 10, TouchEnded)
	// Unlike a key, a finger that lands and lifts in one frame is still seen.
	if ts := s.Touches(); len(ts) != 1 || !ts[0].Began || !ts[0].Ended {
		t.Fatalf("want one touch that began and ended, got %+v", ts)
	}
	s.NewFrame()
	if len(s.Touches()) != 0 {
		t.Fatal("the tap should be gone on the next frame")
	}
}

func TestKeyboardLeavesTouch(t *testing.T) {
	var s State
	s.TouchEvent(1, 0, 0, TouchBegan)
	s.KeyEvent(KeyW, true)
	if s.UsingTouch() {
		t.Fatal("a key press should switch away from touch")
	}
}
