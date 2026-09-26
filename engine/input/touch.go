package input

// Touch is one finger on a touch screen, in framebuffer pixels.
type Touch struct {
	ID             uint64
	X, Y           float64
	StartX, StartY float64 // where it came down
	PrevX, PrevY   float64 // where it was at the start of this frame
	Began          bool    // it came down this frame
	Ended          bool    // it lifted this frame (it's gone on the next)
}

// Delta is how far the finger moved this frame.
func (t Touch) Delta() (dx, dy float64) { return t.X - t.PrevX, t.Y - t.PrevY }

// TouchPhase is what happened to a finger.
type TouchPhase int

const (
	TouchBegan TouchPhase = iota
	TouchMoved
	TouchEnded // lifted, or cancelled by the OS
)

type touchState struct {
	touches    []Touch
	usingTouch bool // the most recent input came from the touch screen
}

// TouchEvent records a finger coming down, moving or lifting. Platforms that
// have a touch screen also report the first finger as the mouse, so menus
// work unchanged; TouchEvent is for controls that need every finger.
func (s *State) TouchEvent(id uint64, x, y float64, phase TouchPhase) {
	s.touch.usingTouch = true
	s.pad.usingPad = false
	i := s.touchIndex(id)
	switch {
	case phase == TouchBegan && i < 0:
		s.touch.touches = append(s.touch.touches, Touch{ID: id, X: x, Y: y, StartX: x, StartY: y,
			PrevX: x, PrevY: y, Began: true})
	case i < 0:
		// A move or lift for a finger we never saw come down (e.g. it began
		// while the app was switching back in): ignore it.
	case phase == TouchEnded:
		s.touch.touches[i].X, s.touch.touches[i].Y = x, y
		s.touch.touches[i].Ended = true
	default:
		s.touch.touches[i].X, s.touch.touches[i].Y = x, y
	}
}

func (s *State) touchIndex(id uint64) int {
	for i, t := range s.touch.touches {
		if t.ID == id && !t.Ended {
			return i
		}
	}
	return -1
}

// newTouchFrame drops the fingers that lifted last frame and makes this
// frame's movement relative to where the others are now.
func (s *State) newTouchFrame() {
	kept := s.touch.touches[:0]
	for _, t := range s.touch.touches {
		if t.Ended {
			continue
		}
		t.PrevX, t.PrevY, t.Began = t.X, t.Y, false
		kept = append(kept, t)
	}
	s.touch.touches = kept
}

// Touches are the fingers on the screen this frame, including those that
// lifted during it (Ended). The slice is only valid until the next NewFrame.
func (s *State) Touches() []Touch { return s.touch.touches }

// PreferTouch makes touch the input in use until something else is used:
// for devices where it's the main one, so hints are right from the start.
func (s *State) PreferTouch() { s.touch.usingTouch = true }

// UsingTouch reports whether the player last used the touch screen (rather
// than the keyboard or a gamepad), e.g. to show on-screen controls.
func (s *State) UsingTouch() bool { return s.touch.usingTouch }
