package game

import (
	"fmt"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/ui"
)

// pauseAction is a pause-menu item.
type pauseAction int

const (
	pauseResume pauseAction = iota
	pauseRestart
	pauseSettings
	pauseMainMenu
)

var pauseLabels = map[pauseAction]string{
	pauseResume:   "Resume",
	pauseRestart:  "Restart run",
	pauseSettings: "Settings",
	pauseMainMenu: "Main menu",
}

// pauseMenu is the overlay shown while the game is frozen.
type pauseMenu struct {
	choice  int
	clicked *pauseAction // a mouse click, reported by the UI one frame late
}

// open resets the menu to Resume.
func (m *pauseMenu) open() {
	m.choice, m.clicked = 0, nil
}

// update handles navigation; Esc, Start or B resume straight away.
func (m *pauseMenu) update(in *input.State, items []pauseAction) (pauseAction, bool) {
	if m.clicked != nil {
		a := *m.clicked
		m.clicked = nil
		return a, true
	}
	if pausePressed(in) || in.PadPressed(input.PadB) {
		return pauseResume, true
	}
	if d := navY(in); d != 0 {
		m.choice = (m.choice + d + len(items)) % len(items)
	}
	if confirmPressed(in) {
		return items[m.choice], true
	}
	return 0, false
}

func (m *pauseMenu) ui(b *ui.Builder, items []pauseAction, in *input.State) {
	b.Panel("##paused", 0.5, 0.26, hudText, 4)
	b.Text("PAUSED")
	b.End()
	b.Panel("##pause", 0.5, 0.6, card, 1.5)
	for i, a := range items {
		if b.MenuButton(pauseLabels[a], 320, 50, i == m.choice) {
			a := a
			m.clicked = &a
		}
	}
	b.End()
	b.Panel("##pausekeys", 0.5, 0.975, hudText, 1.05)
	b.ColorText(uiMuted, "%s", prompt(in,
		"W / S  choose      Enter  select      Esc  resume",
		"D-pad  choose      A  select      B  resume"))
	b.End()
}

// ---- settings ---------------------------------------------------------------

// settingRow is one line of the settings screen.
type settingRow int

const (
	rowFOV settingRow = iota
	rowSensitivity
	rowInvert
	rowStick // touch screens only: a fixed or floating move stick
	rowVolume
	rowHUD
	rowView
	rowBack
	rowCount
)

// Repeat timing for holding left/right on a slider.
const (
	repeatDelay = 0.35 // seconds before a held direction starts repeating
	repeatEvery = 0.05
)

// settingsScreen edits Settings with the mouse (drag the sliders), the
// keyboard or a gamepad: up/down picks a row, left/right adjusts it (held,
// it repeats; the left stick adjusts smoothly), A toggles, B goes back.
type settingsScreen struct {
	touch   bool // the device has a touch screen: show its settings
	choice  settingRow
	held    int     // direction held on the last frame (-1, 0, 1)
	heldFor float32 // seconds it has been held
	back    bool    // "Back" was clicked
}

// open resets the selection to the first row.
func (m *settingsScreen) open() { *m = settingsScreen{touch: m.touch} }

// update applies this frame's input to s and reports when the player leaves.
func (m *settingsScreen) update(in *input.State, s *Settings, dt float32) (done bool) {
	if m.back {
		m.back = false
		return true
	}
	if pausePressed(in) || in.PadPressed(input.PadB) {
		return true
	}
	if d := navY(in); d != 0 {
		m.choice = (m.choice + settingRow(d) + rowCount) % rowCount
		if m.choice == rowStick && !m.touch {
			m.choice = (m.choice + settingRow(d) + rowCount) % rowCount
		}
	}

	// Left/right: a press steps once, holding repeats.
	dir := 0
	if in.Down(input.KeyLeft) || in.Down(input.KeyA) || in.PadDown(input.PadLeft) {
		dir--
	}
	if in.Down(input.KeyRight) || in.Down(input.KeyD) || in.PadDown(input.PadRight) {
		dir++
	}
	steps := 0
	switch {
	case dir == 0:
		m.heldFor = 0
	case dir != m.held:
		steps, m.heldFor = dir, 0
	default:
		before := m.heldFor
		m.heldFor += dt
		if m.heldFor > repeatDelay {
			steps = dir * (int((m.heldFor-repeatDelay)/repeatEvery) - int(max(0, before-repeatDelay)/repeatEvery))
		}
	}
	m.held = dir
	stick := in.PadAxis(input.PadLeftX) // smooth adjustment on the stick

	slide := func(v *float32, lo, hi, step float32) {
		*v = clampf(*v+float32(steps)*step+stick*(hi-lo)*0.5*dt, lo, hi)
	}
	switch m.choice {
	case rowFOV:
		slide(&s.FOV, minFOV, maxFOV, 1)
	case rowSensitivity:
		slide(&s.LookSensitivity, minSensitivity, maxSensitivity, 0.05)
	case rowVolume:
		slide(&s.Volume, 0, 100, 5)
	case rowHUD:
		slide(&s.HUDWidth, minHUDWidth, maxHUDWidth, 5)
	case rowInvert:
		if (steps != 0 && m.heldFor == 0) || confirmPressed(in) { // once per press, no repeat
			s.InvertLook = !s.InvertLook
		}
	case rowStick:
		if (steps != 0 && m.heldFor == 0) || confirmPressed(in) {
			s.FloatingStick = !s.FloatingStick
		}
	case rowView:
		n := len(viewProfiles)
		switch {
		case steps != 0 && m.heldFor == 0: // once per press, no repeat
			s.ViewDistance = (s.ViewDistance + steps%n + n) % n
		case confirmPressed(in):
			s.ViewDistance = (s.ViewDistance + 1) % n
		}
	case rowBack:
		if confirmPressed(in) {
			return true
		}
	}
	return false
}

func (m *settingsScreen) ui(b *ui.Builder, s *Settings, in *input.State) {
	b.Panel("##settingstitle", 0.5, 0.18, hudText, 4)
	b.Text("SETTINGS")
	b.End()

	// A left-aligned column: sliders line up, their labels to the right.
	b.Panel("##settings", 0.5, 0.56, card&^gfx.UICentered, 1.4)
	const width = 360
	style := func(row settingRow, format string) ui.SliderStyle {
		return ui.SliderStyle{Format: format, Width: width, Highlight: m.choice == row}
	}
	b.StyledSlider("Field of view", &s.FOV, minFOV, maxFOV, style(rowFOV, "%.0f°"))
	b.StyledSlider("Look sensitivity", &s.LookSensitivity, minSensitivity, maxSensitivity, style(rowSensitivity, "%.2fx"))
	invert := "Off"
	if s.InvertLook {
		invert = "On"
	}
	if b.MenuButton(fmt.Sprintf("Invert look up/down:  %s##invert", invert), width, 0, m.choice == rowInvert) {
		s.InvertLook = !s.InvertLook
	}
	if m.touch {
		stick := "Fixed"
		if s.FloatingStick {
			stick = "Floating"
		}
		if b.MenuButton(fmt.Sprintf("Move stick:  %s##stick", stick), width, 0, m.choice == rowStick) {
			s.FloatingStick = !s.FloatingStick
		}
	}
	b.StyledSlider("Volume", &s.Volume, 0, 100, style(rowVolume, "%.0f%%"))
	b.StyledSlider("HUD width (ultrawide)", &s.HUDWidth, minHUDWidth, maxHUDWidth, style(rowHUD, "%.0f%%"))
	if b.MenuButton(fmt.Sprintf("View distance:  %s##view", s.view().name), width, 0, m.choice == rowView) {
		s.ViewDistance = (s.ViewDistance + 1) % len(viewProfiles)
	}
	b.Separator()
	if b.MenuButton("Back", width, 46, m.choice == rowBack) {
		m.back = true
	}
	b.End()

	b.Panel("##settingskeys", 0.5, 0.975, hudText, 1.05)
	b.ColorText(uiMuted, "%s", prompt(in,
		"W / S  choose      A / D  adjust      Esc  back",
		"D-pad  choose      D-pad / L-stick  adjust      B  back"))
	b.End()
}
