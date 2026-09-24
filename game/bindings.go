package game

import "CliffCrack/engine/input"

// Shared bindings: every action works from the keyboard and from a gamepad
// (desktop pads through GLFW, handheld built-in controllers on Android).

// navY is a menu move this frame: -1 up, 1 down, 0 none. W/S, the arrows,
// the d-pad or a flick of the left stick.
func navY(in *input.State) int {
	switch {
	case in.Pressed(input.KeyW) || in.Pressed(input.KeyUp) || in.PadPressed(input.PadUp) ||
		in.PadFlicked(input.PadLeftY, -1):
		return -1
	case in.Pressed(input.KeyS) || in.Pressed(input.KeyDown) || in.PadPressed(input.PadDown) ||
		in.PadFlicked(input.PadLeftY, 1):
		return 1
	}
	return 0
}

// confirmPressed selects the highlighted menu item: Enter, Space, A or Start.
func confirmPressed(in *input.State) bool {
	return in.Pressed(input.KeyEnter) || in.Pressed(input.KeySpace) ||
		in.PadPressed(input.PadA) || in.PadPressed(input.PadStart)
}

// pausePressed leaves a mode for the main menu: Esc (also Android's back
// button) or Start.
func pausePressed(in *input.State) bool {
	return in.Pressed(input.KeyEscape) || in.PadPressed(input.PadStart)
}

// debugPressed toggles the debug window: F1 or the pad's View/Select button.
func debugPressed(in *input.State) bool {
	return in.Pressed(input.KeyF1) || in.PadPressed(input.PadBack)
}

// padDirX is the d-pad's horizontal direction (-1, 0, 1).
func padDirX(in *input.State) float32 {
	var v float32
	if in.PadDown(input.PadLeft) {
		v--
	}
	if in.PadDown(input.PadRight) {
		v++
	}
	return v
}

// padDirY is the d-pad's vertical direction, -1 up .. 1 down.
func padDirY(in *input.State) float32 {
	var v float32
	if in.PadDown(input.PadUp) {
		v--
	}
	if in.PadDown(input.PadDown) {
		v++
	}
	return v
}

// prompt picks the keyboard or gamepad version of a hint, depending on what
// the player used last.
func prompt(in *input.State, keys, pad string) string {
	if in != nil && in.UsingPad() {
		return pad
	}
	return keys
}
