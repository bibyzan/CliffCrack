package input

import "math"

// PadButton identifies a gamepad button, in the standard (Xbox-style) layout:
// A is the bottom face button, B the right one. Values follow GLFW's gamepad
// numbering so the desktop platform can convert with a plain cast.
type PadButton int

const (
	PadA PadButton = iota
	PadB
	PadX
	PadY
	PadLB // left shoulder
	PadRB
	PadBack // "View" / "Select"
	PadStart
	PadGuide
	PadLStick // stick clicks
	PadRStick
	PadUp // d-pad
	PadRight
	PadDown
	PadLeft

	padButtonCount
)

// PadAxis identifies a gamepad axis (GLFW's order). Sticks run -1..1 with +Y
// pointing down; triggers run 0..1.
type PadAxis int

const (
	PadLeftX PadAxis = iota
	PadLeftY
	PadRightX
	PadRightY
	PadLeftTrigger
	PadRightTrigger

	padAxisCount
)

const (
	stickDeadzone   = 0.2  // radial: stick drift below this reads as centred
	triggerDeadzone = 0.05 // resting triggers often read a little above 0
	flickThreshold  = 0.6  // a stick pushed this far counts as a d-pad press (menus)
)

// padState is the gamepad part of State.
type padState struct {
	buttons, prevButtons [padButtonCount]bool
	axes, prevAxes       [padAxisCount]float32
	usingPad             bool // the most recent input came from a gamepad
}

// PadEvent records a gamepad button going down or up.
func (s *State) PadEvent(b PadButton, down bool) {
	if b < 0 || b >= padButtonCount {
		return
	}
	s.pad.buttons[b] = down
	if down {
		s.pad.usingPad = true
	}
}

// PadAxisEvent records a gamepad axis value (sticks -1..1, triggers 0..1).
func (s *State) PadAxisEvent(a PadAxis, v float32) {
	if a < 0 || a >= padAxisCount {
		return
	}
	s.pad.axes[a] = v
	if abs(v) > 0.5 {
		s.pad.usingPad = true
	}
}

// PadDown reports whether a gamepad button is held.
func (s *State) PadDown(b PadButton) bool {
	return b >= 0 && b < padButtonCount && s.pad.buttons[b]
}

// PadPressed reports whether a gamepad button went down this frame.
func (s *State) PadPressed(b PadButton) bool {
	return b >= 0 && b < padButtonCount && s.pad.buttons[b] && !s.pad.prevButtons[b]
}

// PadAxis is an axis with its deadzone removed: sticks come back as 0 inside
// the deadzone and rescaled to reach 1 at full tilt, so fine control starts
// right at its edge.
func (s *State) PadAxis(a PadAxis) float32 {
	if a < 0 || a >= padAxisCount {
		return 0
	}
	return deadzoned(s.pad.axes, a)
}

// PadStick returns a stick (left when right is false) with a radial deadzone,
// so diagonals aren't clipped the way per-axis deadzones would clip them.
func (s *State) PadStick(right bool) (x, y float32) {
	ax, ay := PadLeftX, PadLeftY
	if right {
		ax, ay = PadRightX, PadRightY
	}
	x, y = s.pad.axes[ax], s.pad.axes[ay]
	l := float32(math.Hypot(float64(x), float64(y)))
	if l <= stickDeadzone {
		return 0, 0
	}
	scale := min(1, (l-stickDeadzone)/(1-stickDeadzone)) / l
	return x * scale, y * scale
}

// PadFlicked reports whether a stick axis crossed flickThreshold in the
// direction of dir (-1 or 1) this frame: a d-pad press made with the stick,
// for menus.
func (s *State) PadFlicked(a PadAxis, dir float32) bool {
	if a < 0 || a >= padAxisCount {
		return false
	}
	return s.pad.axes[a]*dir > flickThreshold && s.pad.prevAxes[a]*dir <= flickThreshold
}

// UsingPad reports whether the player last used a gamepad (rather than the
// keyboard or mouse), e.g. to pick which button prompts to show.
func (s *State) UsingPad() bool { return s.pad.usingPad }

func deadzoned(axes [padAxisCount]float32, a PadAxis) float32 {
	v := axes[a]
	if a == PadLeftTrigger || a == PadRightTrigger {
		if v <= triggerDeadzone {
			return 0
		}
		return min(1, (v-triggerDeadzone)/(1-triggerDeadzone))
	}
	if abs(v) <= stickDeadzone {
		return 0
	}
	sign := float32(1)
	if v < 0 {
		sign = -1
	}
	return sign * min(1, (abs(v)-stickDeadzone)/(1-stickDeadzone))
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
