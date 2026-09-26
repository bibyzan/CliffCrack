package game

import (
	"fmt"

	"CliffCrack/engine/input"
)

// Rebindable keyboard controls: each action's key is in Settings.Keys (by
// the action's id), and Controls in Settings edits them. The mouse buttons,
// the arrows, Esc and Enter stay as they are.

// Action is something a key does.
type Action int

const (
	ActForward Action = iota
	ActBack
	ActLeft
	ActRight
	ActJump
	ActSprint
	ActCrouch
	ActReload
	ActPickUp
	ActMelee
	ActGadget
	ActGrenade
	ActGrenadeKind
	ActWeapon1
	ActWeapon2
	actionCount
)

// actionInfo is each action's id (in settings.json), label and default key.
var actionInfo = [actionCount]struct {
	id, label string
	key       input.Key
}{
	ActForward:     {"forward", "Move forward", input.KeyW},
	ActBack:        {"back", "Move back", input.KeyS},
	ActLeft:        {"left", "Move left", input.KeyA},
	ActRight:       {"right", "Move right", input.KeyD},
	ActJump:        {"jump", "Jump, vault, climb", input.KeySpace},
	ActSprint:      {"sprint", "Sprint", input.KeyLeftShift},
	ActCrouch:      {"crouch", "Crouch (sprinting: slide)", input.KeyLeftControl},
	ActReload:      {"reload", "Reload", input.KeyR},
	ActPickUp:      {"pickup", "Pick up", input.KeyE},
	ActMelee:       {"melee", "Elbow", input.KeyF},
	ActGadget:      {"gadget", "Gadget", input.KeyQ},
	ActGrenade:     {"grenade", "Throw grenade", input.KeyG},
	ActGrenadeKind: {"grenadekind", "Frag / sticky", input.KeyC},
	ActWeapon1:     {"weapon1", "Weapon 1", input.Key1},
	ActWeapon2:     {"weapon2", "Weapon 2", input.Key2},
}

// key is the key bound to a (its default unless rebound).
func (s *Settings) key(a Action) input.Key {
	if k, ok := s.Keys[actionInfo[a].id]; ok {
		return input.Key(k)
	}
	return actionInfo[a].key
}

// bind puts a on key k. Whatever else had k takes a's old key, so nothing
// is ever left without one (or two actions on one key).
func (s *Settings) bind(a Action, k input.Key) {
	old := s.key(a)
	for b := range actionCount {
		if b != a && s.key(b) == k {
			s.setKey(b, old)
		}
	}
	s.setKey(a, k)
}

func (s *Settings) setKey(a Action, k input.Key) {
	if s.Keys == nil {
		s.Keys = map[string]int{}
	}
	if k == actionInfo[a].key {
		delete(s.Keys, actionInfo[a].id) // the default: nothing to save
		return
	}
	s.Keys[actionInfo[a].id] = int(k)
}

// resetKeys puts every action back on its default key.
func (s *Settings) resetKeys() { s.Keys = nil }

// down, pressed and axis read an action's key.
func (s *Settings) down(in *input.State, a Action) bool    { return in.Down(s.key(a)) }
func (s *Settings) pressed(in *input.State, a Action) bool { return in.Pressed(s.key(a)) }
func (s *Settings) axis(in *input.State, neg, pos Action) float32 {
	return in.Axis(s.key(neg), s.key(pos))
}

// keyName is how a key is shown.
func keyName(k input.Key) string {
	switch {
	case k >= input.KeyA && k <= input.KeyZ, k >= input.Key0 && k <= input.Key9:
		return string(rune(k))
	case k >= input.KeyF1 && k <= input.KeyF12:
		return fmt.Sprintf("F%d", k-input.KeyF1+1)
	}
	if n, ok := keyNames[k]; ok {
		return n
	}
	if k > 32 && k < 127 {
		return string(rune(k)) // punctuation: GLFW uses ASCII
	}
	return fmt.Sprintf("Key %d", int(k))
}

var keyNames = map[input.Key]string{
	input.KeySpace: "Space", input.KeyEscape: "Esc", input.KeyEnter: "Enter", input.KeyTab: "Tab",
	input.KeyBackspace: "Backspace", input.KeyLeft: "Left", input.KeyRight: "Right", input.KeyUp: "Up",
	input.KeyDown: "Down", input.KeyLeftShift: "Shift", input.KeyRightShift: "Right Shift",
	input.KeyLeftControl: "Ctrl", input.KeyRightControl: "Right Ctrl", input.KeyLeftAlt: "Alt",
	input.KeyRightAlt: "Right Alt",
}

// keysHint is the Arena's keyboard controls, as bound, for its hint line.
func (s *Settings) keysHint() string {
	k := func(a Action) string { return keyName(s.key(a)) }
	return fmt.Sprintf("LMB  fire    RMB  aim    %s %s / wheel  swap    %s  reload    %s  pick up    %s  elbow    %s  gadget    %s  grenade    %s  frag / sticky    %s  crouch (sprinting: slide)    %s  jump, vault, climb",
		k(ActWeapon1), k(ActWeapon2), k(ActReload), k(ActPickUp), k(ActMelee), k(ActGadget), k(ActGrenade),
		k(ActGrenadeKind), k(ActCrouch), k(ActJump))
}
