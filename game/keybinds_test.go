package game

import (
	"reflect"
	"testing"

	"CliffCrack/engine/input"
)

func TestBindingsSwapAndSave(t *testing.T) {
	s := DefaultSettings()
	if s.key(ActJump) != input.KeySpace || s.key(ActCrouch) != input.KeyLeftControl {
		t.Fatal("defaults: jump on Space, crouch on Ctrl")
	}
	s.bind(ActCrouch, input.KeyC) // C was frag/sticky: it takes crouch's old key
	if s.key(ActCrouch) != input.KeyC || s.key(ActGrenadeKind) != input.KeyLeftControl {
		t.Errorf("after binding crouch to C: crouch %v, frag/sticky %v; want C and Ctrl", s.key(ActCrouch), s.key(ActGrenadeKind))
	}
	seen := map[input.Key]Action{}
	for a := range actionCount {
		if b, dup := seen[s.key(a)]; dup {
			t.Errorf("%s and %s share a key", actionInfo[a].label, actionInfo[b].label)
		}
		seen[s.key(a)] = a
	}
	dir := t.TempDir()
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSettings(dir)
	if err != nil || !reflect.DeepEqual(back.Keys, s.Keys) {
		t.Errorf("bindings didn't survive saving: %v, want %v", back.Keys, s.Keys)
	}
	s.bind(ActCrouch, input.KeyLeftControl) // and back: both on their defaults, nothing saved
	if len(s.Keys) != 0 {
		t.Errorf("back on the defaults, %v still saved", s.Keys)
	}
}

func TestKeyNames(t *testing.T) {
	for k, want := range map[input.Key]string{input.KeyW: "W", input.Key1: "1", input.KeySpace: "Space",
		input.KeyLeftControl: "Ctrl", input.KeyF1 + 4: "F5"} {
		if got := keyName(k); got != want {
			t.Errorf("keyName(%d) = %q, want %q", k, got, want)
		}
	}
}
