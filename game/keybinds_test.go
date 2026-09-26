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

func TestMouseButtonsBind(t *testing.T) {
	s := DefaultSettings()
	if s.key(ActFire) != input.KeyMouseLeft || s.key(ActAim) != input.KeyMouseRight {
		t.Fatal("defaults: fire on the left button, aim on the right")
	}
	var in input.State
	in.NewFrame()
	in.ButtonEvent(4, true) // a side button
	k, ok := in.PressedKey()
	if !ok || k != input.KeyMouse+4 {
		t.Fatalf("a side button's press reads as %v %v, want Mouse 5", k, ok)
	}
	s.bind(ActMelee, k)
	if !s.pressed(&in, ActMelee) || !s.down(&in, ActMelee) || keyName(s.key(ActMelee)) != "Mouse 5" {
		t.Errorf("melee on Mouse 5: pressed %v, down %v, named %q", s.pressed(&in, ActMelee), s.down(&in, ActMelee), keyName(s.key(ActMelee)))
	}
	s.bind(ActMelee, input.KeyMouseRight) // aim takes melee's old key
	if s.key(ActAim) != input.KeyMouse+4 {
		t.Errorf("after binding melee to RMB, aim is on %v, want Mouse 5", keyName(s.key(ActAim)))
	}
	in.NewFrame()
	if s.pressed(&in, ActAim) || !s.down(&in, ActAim) {
		t.Error("held into the next frame: down, not pressed")
	}
}
