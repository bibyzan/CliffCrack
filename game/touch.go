package game

import (
	"math"

	"CliffCrack/engine/input"
	"CliffCrack/engine/ui"
)

// On-screen controls for a touch screen (a phone): an analog stick in the
// bottom-left corner (or, with the FloatingStick setting, wherever the left
// thumb lands), round buttons, and dragging anywhere else to look around. Run and Arena each lay out
// their own buttons (see runTouch and arenaTouch).
//
// Sizes and positions are in screen heights, so the controls are the same
// physical size on every phone held sideways.
const (
	touchMargin   = 0.09 // gap to the screen's edges (clears the rounded corners)
	touchStickR   = 0.13 // how far the stick's knob travels
	touchKnobR    = 0.055
	touchStickRim = 2.0  // a fixed stick is taken by a thumb landing this many radii from its centre
	touchHitSlop  = 1.35 // buttons take touches this much further out than they're drawn
	touchLookSens = 3.0  // radians of camera turn per screen height of drag
)

// Touch control colours (sRGB, for ui.Builder.Circle and Image).
var (
	touchBack   = [4]float32{0.07, 0.09, 0.16, 0.4}
	touchRing   = [4]float32{1, 1, 1, 0.4}
	touchKnob   = [4]float32{1, 1, 1, 0.8}
	touchIcon   = [4]float32{1, 1, 1, 0.92}
	touchOrange = [4]float32{0.96, 0.52, 0.16, 0.6}
	touchRed    = [4]float32{0.9, 0.22, 0.18, 0.55}
)

// touchIcons are the controls' own icons (the Arena's also show weapons and
// grenades, from its HUD icons). Loaded once, the first time they're needed.
type touchIcons struct {
	jump, aim, reload, pause, crouch icon
}

var loadedTouchIcons *touchIcons

// getTouchIcons loads the icons, or returns them loaded. A missing icon is
// left zero and its button shows its label instead.
func getTouchIcons() *touchIcons {
	if loadedTouchIcons != nil {
		return loadedTouchIcons
	}
	t := &touchIcons{}
	for _, x := range []struct {
		dst  *icon
		name string
	}{{&t.jump, "jump"}, {&t.aim, "aim"}, {&t.reload, "reload"}, {&t.pause, "pause"}, {&t.crouch, "crouch"}} {
		ic, err := loadIcon(x.name, 128)
		if err != nil {
			logf("touch controls: %v", err)
			continue
		}
		*x.dst = ic
	}
	loadedTouchIcons = t
	return t
}

// touchButton is a round on-screen button, showing an icon (or, without
// one, its label).
type touchButton struct {
	label string
	icon  icon
	x, y  float32 // centre, in screen heights from the left and top edges; negative: from the right and bottom
	r     float32 // radius, in screen heights
	color [4]float32
	look  bool // a finger on it also turns the camera as it drags (fire, so you can aim while shooting)

	hidden bool // set before update: not drawn and takes no touches
	lit    bool // drawn highlighted (a toggle that's on)

	down    bool // a finger is on it
	pressed bool // a finger landed on it this frame
	id      uint64
}

// touchControls reads the fingers each frame and draws the controls.
type touchControls struct {
	w, h     float32 // the screen, in pixels
	buttons  []*touchButton
	floating bool // the stick appears where the thumb lands (set before update, from the settings)

	stickOn      bool
	stickID      uint64
	baseX, baseY float32 // the stick's centre: where the thumb landed
	x, y         float32 // its deflection, -1..1, +y down

	lookIDs []uint64 // fingers turning the camera (besides look buttons)
}

// touchFrame is what the fingers did this frame.
type touchFrame struct {
	stickX, stickY float32 // -1..1, +y down (towards the player)
	yaw, elev      float32 // radians of look: right, and down
}

// centre is where button b sits on the screen, in pixels.
func (t *touchControls) centre(b *touchButton) (float32, float32) {
	x, y := b.x*t.h, b.y*t.h
	if b.x < 0 {
		x += t.w
	}
	if b.y < 0 {
		y += t.h
	}
	return x, y
}

// restBase is where the stick sits until a thumb takes it.
func (t *touchControls) restBase() (float32, float32) {
	off := (touchMargin + touchStickR*1.15) * t.h
	return off, t.h - off
}

// release lets go of everything (e.g. when a menu takes over), so no finger
// keeps steering or firing.
func (t *touchControls) release() {
	t.stickOn, t.x, t.y = false, 0, 0
	t.lookIDs = t.lookIDs[:0]
	for _, b := range t.buttons {
		b.down, b.pressed = false, false
	}
}

// update reads this frame's fingers on a w x h screen.
func (t *touchControls) update(in *input.State, w, h float32) touchFrame {
	t.w, t.h = w, h
	var f touchFrame
	for _, b := range t.buttons {
		b.pressed = false
		if b.hidden {
			b.down = false
		}
	}
	if w <= 0 || h <= 0 {
		return f
	}
	for _, tc := range in.Touches() {
		x, y := float32(tc.X), float32(tc.Y)
		if tc.Began {
			t.claim(tc.ID, x, y)
		}
		dx, dy := tc.Delta()
		look := false
		switch {
		case t.stickOn && tc.ID == t.stickID:
			t.moveStick(x, y)
			if tc.Ended {
				t.stickOn, t.x, t.y = false, 0, 0
			}
		case t.buttonOf(tc.ID) != nil:
			b := t.buttonOf(tc.ID)
			look = b.look
			if tc.Ended {
				b.down = false
			}
		default:
			for i, id := range t.lookIDs {
				if id == tc.ID {
					look = true
					if tc.Ended {
						t.lookIDs = append(t.lookIDs[:i], t.lookIDs[i+1:]...)
					}
					break
				}
			}
		}
		if look {
			f.yaw += float32(dx) / h * touchLookSens
			f.elev += float32(dy) / h * touchLookSens
		}
	}
	f.stickX, f.stickY = t.x, t.y
	return f
}

// buttonOf is the button finger id is holding, if any.
func (t *touchControls) buttonOf(id uint64) *touchButton {
	for _, b := range t.buttons {
		if b.down && b.id == id {
			return b
		}
	}
	return nil
}

// claim decides what a new finger does from where it landed: the nearest
// button in reach, else the stick on the left half, else looking.
func (t *touchControls) claim(id uint64, x, y float32) {
	var best *touchButton
	bestDist := float32(math.MaxFloat32)
	for _, b := range t.buttons {
		if b.hidden {
			continue
		}
		cx, cy := t.centre(b)
		d := float32(math.Hypot(float64(x-cx), float64(y-cy)))
		if d <= b.r*touchHitSlop*t.h && d/b.r < bestDist {
			best, bestDist = b, d/b.r
		}
	}
	switch {
	case best != nil:
		best.down, best.pressed, best.id = true, true, id
	case t.floating && x < t.w*0.5 && !t.stickOn:
		// The stick centres on the thumb, kept far enough from the edges that
		// it can be pushed all the way in every direction.
		r := touchStickR * t.h
		t.stickOn, t.stickID = true, id
		t.baseX = min(max(x, r), t.w*0.5)
		t.baseY = min(max(y, r), t.h-r)
		t.x, t.y = 0, 0
		t.moveStick(x, y)
	case !t.floating && !t.stickOn && t.nearStick(x, y):
		// The stick stays put; a thumb landing on it (or near: thumbs are
		// imprecise) pushes it straight away from its centre.
		t.stickOn, t.stickID = true, id
		t.baseX, t.baseY = t.restBase()
		t.moveStick(x, y)
	default:
		t.lookIDs = append(t.lookIDs, id)
	}
}

// nearStick reports whether x, y is on the fixed stick or close to it.
func (t *touchControls) nearStick(x, y float32) bool {
	bx, by := t.restBase()
	return math.Hypot(float64(x-bx), float64(y-by)) <= float64(touchStickRim*touchStickR*t.h)
}

// moveStick sets the deflection from the thumb at x, y. Pushed past the
// stick's reach, a floating stick's base follows the thumb, so turning back
// the other way responds at once; a fixed one just stays at full tilt.
func (t *touchControls) moveStick(x, y float32) {
	r := touchStickR * t.h
	dx, dy := x-t.baseX, y-t.baseY
	if l := float32(math.Hypot(float64(dx), float64(dy))); l > r {
		if t.floating {
			t.baseX += dx * (1 - r/l)
			t.baseY += dy * (1 - r/l)
		}
		dx, dy = dx*r/l, dy*r/l
	}
	t.x, t.y = dx/r, dy/r
}

// stickAxis removes a deadzone from one axis of the stick and rescales the
// rest to reach 1 at full deflection.
func stickAxis(v, dead float32) float32 {
	a := float32(math.Abs(float64(v)))
	if a <= dead {
		return 0
	}
	return float32(math.Copysign(float64(min(1, (a-dead)/(1-dead))), float64(v)))
}

// ui draws the stick (at rest in its corner until a thumb takes it) and the
// buttons.
func (t *touchControls) ui(b *ui.Builder) {
	if t.w <= 0 || t.h <= 0 {
		return
	}
	h := t.h
	bx, by := t.restBase()
	back, ring := touchBack, touchRing
	if t.stickOn {
		bx, by = t.baseX, t.baseY
	} else {
		back[3] *= 0.6
		ring[3] *= 0.6
	}
	r := touchStickR * h
	b.Circle("", bx, by, r, 0, back, 0)
	b.Circle("", bx, by, r, 0.006*h, ring, 0)
	b.Circle("", bx+t.x*r, by+t.y*r, touchKnobR*h, 0, touchKnob, 0)

	for _, btn := range t.buttons {
		if btn.hidden {
			continue
		}
		x, y := t.centre(btn)
		r := btn.r * h
		c, ring := btn.color, touchRing
		switch {
		case btn.lit:
			c, ring = touchOrange, [4]float32{1, 0.75, 0.5, 0.9}
		case btn.down:
			c[3] = min(1, c[3]+0.3)
		}
		if btn.down {
			r *= 0.94 // pressed in
		}
		if btn.icon.tex == 0 {
			b.Circle(btn.label, x, y, r, 0, c, min(0.045, btn.r*0.42)*h)
		} else {
			b.Circle("", x, y, r, 0, c, 0)
			// Square icons fill half the button's height; wide ones (the
			// guns) are fitted to its width instead.
			ih := r * 0.95
			iw := ih * btn.icon.aspect
			if iw > r*1.4 {
				iw = r * 1.4
				ih = iw / btn.icon.aspect
			}
			b.Image(btn.icon.tex, x, y, iw, ih, touchIcon)
		}
		b.Circle("", x, y, r, 0.004*h, ring, 0)
	}
}

// ---- Run ------------------------------------------------------------------------

// runTouch is Run's layout: the stick steers (left/right) and tucks or
// brakes (up/down), JUMP bottom right, pause top left.
type runTouch struct {
	touchControls
	jump, pause touchButton
}

const (
	stickSteerDead = 0.08 // stick deflection ignored for steering...
	stickThrotDead = 0.4  // ...and for tuck/brake, so steering hard doesn't brake too
)

func newRunTouch() *runTouch {
	icons := getTouchIcons()
	t := &runTouch{
		jump:  touchButton{label: "JUMP", icon: icons.jump, x: -0.216, y: -0.216, r: 0.105, color: touchOrange},
		pause: touchButton{label: "II", icon: icons.pause, x: 0.14, y: 0.14, r: 0.05, color: touchBack},
	}
	t.buttons = []*touchButton{&t.jump, &t.pause}
	return t
}

// read turns this frame's fingers into ride and look input, and reports a
// tap on pause.
func (t *runTouch) read(in *input.State, w, h float32) (ride rideInput, look lookInput, pause bool) {
	f := t.update(in, w, h)
	ride.steer = stickAxis(f.stickX, stickSteerDead)
	ride.throttle = -stickAxis(f.stickY, stickThrotDead)
	ride.jump = t.jump.pressed
	return ride, lookInput{yaw: f.yaw, elev: f.elev}, t.pause.pressed
}
