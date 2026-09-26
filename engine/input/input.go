// Package input tracks keyboard, mouse and gamepad state per frame. It is pure Go: the
// platform layer feeds it events, gameplay code queries it.
package input

// Key identifies a keyboard key. Values match GLFW key codes so the platform
// layer can convert with a plain cast.
type Key int

const (
	KeySpace Key = 32
	Key0     Key = 48
	Key1     Key = 49
	Key2     Key = 50
	Key3     Key = 51
	Key4     Key = 52
	Key5     Key = 53
	Key6     Key = 54
	Key7     Key = 55
	Key8     Key = 56
	Key9     Key = 57
	KeyA     Key = 65
	KeyB     Key = 66
	KeyC     Key = 67
	KeyD     Key = 68
	KeyE     Key = 69
	KeyF     Key = 70
	KeyG     Key = 71
	KeyH     Key = 72
	KeyI     Key = 73
	KeyJ     Key = 74
	KeyK     Key = 75
	KeyL     Key = 76
	KeyM     Key = 77
	KeyN     Key = 78
	KeyO     Key = 79
	KeyP     Key = 80
	KeyQ     Key = 81
	KeyR     Key = 82
	KeyS     Key = 83
	KeyT     Key = 84
	KeyU     Key = 85
	KeyV     Key = 86
	KeyW     Key = 87
	KeyX     Key = 88
	KeyY     Key = 89
	KeyZ     Key = 90

	KeyEscape    Key = 256
	KeyEnter     Key = 257
	KeyTab       Key = 258
	KeyBackspace Key = 259
	KeyRight     Key = 262
	KeyLeft      Key = 263
	KeyDown      Key = 264
	KeyUp        Key = 265
	KeyF1        Key = 290
	KeyF2        Key = 291
	KeyF3        Key = 292
	KeyF4        Key = 293
	KeyF5        Key = 294
	KeyF6        Key = 295
	KeyF7        Key = 296
	KeyF8        Key = 297
	KeyF9        Key = 298
	KeyF10       Key = 299
	KeyF11       Key = 300
	KeyF12       Key = 301

	KeyLeftShift    Key = 340
	KeyLeftControl  Key = 341
	KeyLeftAlt      Key = 342
	KeyRightShift   Key = 344
	KeyRightControl Key = 345
	KeyRightAlt     Key = 346

	keyCount = 349 // GLFW_KEY_LAST + 1

	// Mouse buttons can stand in for keys, so a control can be bound to one:
	// KeyMouse + the button (GLFW numbering: 0 left, 1 right, 2 middle, 3
	// and 4 the side buttons). Down, Pressed and PressedKey know them.
	KeyMouse       Key = 400
	KeyMouseLeft   Key = KeyMouse + Key(MouseLeft)
	KeyMouseRight  Key = KeyMouse + Key(MouseRight)
	KeyMouseMiddle Key = KeyMouse + Key(MouseMiddle)
)

// IsMouse reports whether k is a mouse button (see KeyMouse).
func (k Key) IsMouse() bool { return k >= KeyMouse && k < KeyMouse+buttonCount }

// MouseButton identifies a mouse button (GLFW numbering).
type MouseButton int

const (
	MouseLeft   MouseButton = 0
	MouseRight  MouseButton = 1
	MouseMiddle MouseButton = 2

	buttonCount = 8
)

// State is the input for the current frame. Call NewFrame before feeding the
// frame's events; queries then describe this frame.
type State struct {
	keys, prevKeys       [keyCount]bool
	buttons, prevButtons [buttonCount]bool

	mouseX, mouseY float64
	haveMouse      bool // false until the first move after start/reset
	dx, dy         float64
	scroll         float64

	pad   padState
	touch touchState
}

// NewFrame starts a new frame: edges and deltas are relative to this point.
func (s *State) NewFrame() {
	s.prevKeys = s.keys
	s.prevButtons = s.buttons
	s.pad.prevButtons = s.pad.buttons
	s.pad.prevAxes = s.pad.axes
	s.dx, s.dy, s.scroll = 0, 0, 0
	s.newTouchFrame()
}

// KeyEvent records a key going down (including auto-repeat) or up.
func (s *State) KeyEvent(k Key, down bool) {
	if k >= 0 && k < keyCount {
		s.keys[k] = down
		s.pad.usingPad = false
		s.touch.usingTouch = false
	}
}

// ButtonEvent records a mouse button going down or up.
func (s *State) ButtonEvent(b MouseButton, down bool) {
	if b >= 0 && b < buttonCount {
		s.buttons[b] = down
		s.pad.usingPad = false
	}
}

// MoveEvent records the cursor position (in window pixels, or unbounded
// virtual units while the cursor is locked).
func (s *State) MoveEvent(x, y float64) {
	if s.haveMouse {
		s.dx += x - s.mouseX
		s.dy += y - s.mouseY
	}
	s.mouseX, s.mouseY, s.haveMouse = x, y, true
}

// ScrollEvent records vertical scrolling (positive = away from the user).
func (s *State) ScrollEvent(dy float64) {
	s.scroll += dy
}

// ResetMouse forgets the last cursor position so the next move produces no
// delta. Use it when the cursor mode changes and positions jump.
func (s *State) ResetMouse() {
	s.haveMouse = false
}

// ReleaseAll marks every key and button as up, centres the gamepad and lifts
// every finger (e.g. when the window loses focus).
func (s *State) ReleaseAll() {
	s.keys = [keyCount]bool{}
	s.buttons = [buttonCount]bool{}
	s.pad.buttons = [padButtonCount]bool{}
	s.pad.axes = [padAxisCount]float32{}
	for i := range s.touch.touches {
		s.touch.touches[i].Ended = true
	}
}

// PressedKey is a key or mouse button (as KeyMouse + the button) that went
// down this frame, if any (the lowest-numbered if several did): for
// rebinding controls.
func (s *State) PressedKey() (Key, bool) {
	for k := range Key(keyCount) {
		if s.keys[k] && !s.prevKeys[k] {
			return k, true
		}
	}
	for b := range MouseButton(buttonCount) {
		if s.MousePressed(b) {
			return KeyMouse + Key(b), true
		}
	}
	return 0, false
}

// Down, Pressed and Released take a key or a mouse button (KeyMouse + the button).
func (s *State) Down(k Key) bool {
	if k.IsMouse() {
		return s.MouseDown(MouseButton(k - KeyMouse))
	}
	return valid(k) && s.keys[k]
}

func (s *State) Pressed(k Key) bool {
	if k.IsMouse() {
		return s.MousePressed(MouseButton(k - KeyMouse))
	}
	return valid(k) && s.keys[k] && !s.prevKeys[k]
}

func (s *State) Released(k Key) bool {
	if k.IsMouse() {
		b := k - KeyMouse
		return !s.buttons[b] && s.prevButtons[b]
	}
	return valid(k) && !s.keys[k] && s.prevKeys[k]
}

// AnyMouseDown reports whether any mouse button is held.
func (s *State) AnyMouseDown() bool { return s.buttons != [buttonCount]bool{} }

func (s *State) MouseDown(b MouseButton) bool {
	return b >= 0 && b < buttonCount && s.buttons[b]
}

func (s *State) MousePressed(b MouseButton) bool {
	return b >= 0 && b < buttonCount && s.buttons[b] && !s.prevButtons[b]
}

// MouseDelta is how far the cursor moved this frame.
func (s *State) MouseDelta() (dx, dy float64) { return s.dx, s.dy }

// MousePos is the last known cursor position.
func (s *State) MousePos() (x, y float64) { return s.mouseX, s.mouseY }

// Scroll is this frame's vertical scroll amount.
func (s *State) Scroll() float64 { return s.scroll }

// Axis returns -1, 0 or 1 from a pair of opposing keys (both held cancels out).
func (s *State) Axis(negative, positive Key) float32 {
	var v float32
	if s.Down(negative) {
		v--
	}
	if s.Down(positive) {
		v++
	}
	return v
}

func valid(k Key) bool { return k >= 0 && k < keyCount }
