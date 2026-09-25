package game

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"CliffCrack/engine/input"
)

func TestSettingsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil || s != DefaultSettings() {
		t.Fatalf("no file should give the defaults, got %+v, %v", s, err)
	}
	s.FOV, s.LookSensitivity, s.InvertLook, s.Volume = 72, 1.5, true, 40
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSettings(dir)
	if err != nil || back != s {
		t.Errorf("round trip gave %+v, %v; want %+v", back, err, s)
	}
}

func TestSettingsAreClampedAndDefaulted(t *testing.T) {
	dir := t.TempDir()
	// Out of range, and missing the sensitivity entirely.
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"fov": 500, "volume": -3}`), 0o644)
	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.FOV != maxFOV || s.Volume != 0 || s.LookSensitivity != DefaultSettings().LookSensitivity {
		t.Errorf("got %+v", s)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`not json`), 0o644)
	if s, err := LoadSettings(dir); err == nil || s != DefaultSettings() {
		t.Errorf("a broken file should report an error and fall back to the defaults, got %+v, %v", s, err)
	}
}

// frame starts an input frame with the given setup applied.
func frame(in *input.State, feed func(*input.State)) *input.State {
	in.NewFrame()
	if feed != nil {
		feed(in)
	}
	return in
}

func TestPauseMenu(t *testing.T) {
	items := []pauseAction{pauseResume, pauseRestart, pauseSettings, pauseMainMenu}
	var m pauseMenu
	var in input.State
	m.open()

	// Esc resumes straight away (it's how you paused, too).
	if a, ok := m.update(frame(&in, func(s *input.State) { s.KeyEvent(input.KeyEscape, true) }), items); !ok || a != pauseResume {
		t.Errorf("Esc: %v %v, want resume", a, ok)
	}
	in.ReleaseAll()
	// Down twice and A picks Settings.
	m.update(frame(&in, func(s *input.State) { s.PadEvent(input.PadDown, true) }), items)
	m.update(frame(&in, func(s *input.State) { s.PadEvent(input.PadDown, false) }), items)
	m.update(frame(&in, func(s *input.State) { s.PadEvent(input.PadDown, true) }), items)
	if a, ok := m.update(frame(&in, func(s *input.State) { s.PadEvent(input.PadA, true) }), items); !ok || a != pauseSettings {
		t.Errorf("down, down, A: %v %v, want settings", a, ok)
	}
	// A mouse click (reported by the UI) wins on the next update.
	in.ReleaseAll()
	quit := pauseMainMenu
	m.clicked = &quit
	if a, ok := m.update(frame(&in, nil), items); !ok || a != pauseMainMenu {
		t.Errorf("click: %v %v, want main menu", a, ok)
	}
}

func TestSettingsScreenAdjustsWithKeysAndPad(t *testing.T) {
	var m settingsScreen
	s := DefaultSettings()
	var in input.State
	const dt = 1.0 / 60

	// FOV row: one press of right is one degree.
	m.update(frame(&in, func(st *input.State) { st.KeyEvent(input.KeyRight, true) }), &s, dt)
	if s.FOV != DefaultSettings().FOV+1 {
		t.Fatalf("one press: FOV %v", s.FOV)
	}
	// Held: nothing more until the repeat delay, then it keeps going.
	for i := 0; i < int(repeatDelay*60)-2; i++ {
		m.update(frame(&in, nil), &s, dt)
	}
	if s.FOV != DefaultSettings().FOV+1 {
		t.Fatalf("held briefly: FOV %v, want no repeat yet", s.FOV)
	}
	for i := 0; i < 60; i++ {
		m.update(frame(&in, nil), &s, dt)
	}
	if s.FOV < DefaultSettings().FOV+10 {
		t.Errorf("held for a second: FOV %v, want it to keep climbing", s.FOV)
	}
	in.ReleaseAll()

	// The stick adjusts smoothly, and never past the limits.
	for i := 0; i < 5*60; i++ {
		m.update(frame(&in, func(st *input.State) { st.PadAxisEvent(input.PadLeftX, -1) }), &s, dt)
	}
	if s.FOV != minFOV {
		t.Errorf("stick held left: FOV %v, want the minimum %v", s.FOV, minFOV)
	}
	in.ReleaseAll()

	// Down twice to the invert toggle: A flips it once, holding doesn't flicker it.
	m.update(frame(&in, func(st *input.State) { st.KeyEvent(input.KeyDown, true) }), &s, dt)
	m.update(frame(&in, func(st *input.State) { st.KeyEvent(input.KeyDown, false) }), &s, dt)
	m.update(frame(&in, func(st *input.State) { st.KeyEvent(input.KeyDown, true) }), &s, dt)
	in.ReleaseAll()
	m.update(frame(&in, func(st *input.State) { st.PadEvent(input.PadRight, true) }), &s, dt)
	for i := 0; i < 60; i++ {
		m.update(frame(&in, nil), &s, dt)
	}
	if !s.InvertLook {
		t.Error("right on the invert row should turn it on, once")
	}
	in.ReleaseAll()

	// B leaves.
	if !m.update(frame(&in, func(st *input.State) { st.PadEvent(input.PadB, true) }), &s, dt) {
		t.Error("B should leave the settings")
	}
}

func TestLookSettingsScaleAndInvert(t *testing.T) {
	s := DefaultSettings()
	s.LookSensitivity, s.InvertLook = 2, true
	r := &Run{ride: newTestRide(3), settings: &s}
	var in input.State
	in.NewFrame()
	in.MoveEvent(100, 100)
	in.NewFrame()
	in.MoveEvent(110, 90) // right and up

	l := r.look(&in, true, 1.0/60)
	if want := float32(10 * camMouseSens * 2); math.Abs(float64(l.yaw-want)) > 1e-6 {
		t.Errorf("yaw %v, want %v (doubled)", l.yaw, want)
	}
	if l.elev <= 0 {
		t.Errorf("inverted: moving the mouse up should look down (elev > 0), got %v", l.elev)
	}
}

func TestHUDWidthSetting(t *testing.T) {
	dir := t.TempDir()
	// An older settings file, from before the HUD width: it's full width.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"fov": 60}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(dir)
	if err != nil || s.HUDWidth != 100 {
		t.Fatalf("loaded %+v (%v): want the HUD at full width", s, err)
	}
	s.HUDWidth = 5
	s.clamp()
	if s.HUDWidth != minHUDWidth {
		t.Errorf("HUD width clamped to %v, want %v", s.HUDWidth, minHUDWidth)
	}
	s.HUDWidth = 50
	m := &Arena{settings: &s}
	if got := m.hudX(0); abs32(got-0.25) > 1e-6 {
		t.Errorf("the HUD box's left edge at %v of the screen, want 0.25", got)
	}
	if got := m.hudX(1); abs32(got-0.75) > 1e-6 {
		t.Errorf("its right edge at %v, want 0.75", got)
	}
}
