package game

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
)

// Settings are the player's preferences, shown on the settings screen and
// saved between sessions.
type Settings struct {
	FOV             float32 `json:"fov"`              // degrees, vertical; Run widens it a little with speed
	LookSensitivity float32 `json:"look_sensitivity"` // multiplier for mouse and right-stick looking
	InvertLook      bool    `json:"invert_look"`      // push up / move the mouse up to look down
	Volume          float32 `json:"volume"`           // percent
}

// Setting ranges.
const (
	minFOV, maxFOV                 = 35, 90
	minSensitivity, maxSensitivity = 0.25, 3
)

func DefaultSettings() Settings {
	return Settings{FOV: 50, LookSensitivity: 1, Volume: 80}
}

// clamp brings every value into range (e.g. after loading a hand-edited file).
func (s *Settings) clamp() {
	d := DefaultSettings()
	fix := func(v *float32, lo, hi, def float32) {
		if math.IsNaN(float64(*v)) {
			*v = def
		}
		*v = clampf(*v, lo, hi)
	}
	fix(&s.FOV, minFOV, maxFOV, d.FOV)
	fix(&s.LookSensitivity, minSensitivity, maxSensitivity, d.LookSensitivity)
	fix(&s.Volume, 0, 100, d.Volume)
}

// fovRadians is the base vertical field of view.
func (s *Settings) fovRadians() float32 { return s.FOV * math.Pi / 180 }

// lookSign is -1 when looking is inverted.
func (s *Settings) lookSign() float32 {
	if s.InvertLook {
		return -1
	}
	return 1
}

// LoadSettings reads the settings saved in dir, falling back to the defaults
// for a missing file or any value it doesn't have.
func LoadSettings(dir string) (Settings, error) {
	s := DefaultSettings()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), err
	}
	s.clamp()
	return s, nil
}

// Save writes the settings to dir.
func (s Settings) Save(dir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), append(data, '\n'), 0o644)
}
