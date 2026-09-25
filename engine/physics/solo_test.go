package physics

import (
	"testing"

	"CliffCrack/engine/mathx"
)

// StepBody moves one (player-like, non-rolling) body the way Update would (same resting height on a
// floor, same fall), and leaves the rest alone.
func TestStepBodyMatchesUpdate(t *testing.T) {
	build := func() (*World, *Body, *Body) {
		w := NewWorld()
		floor := NewBox(mathx.Vec3{10, 0.5, 10}, Static)
		floor.Position = mathx.Vec3{0, -0.5, 0}
		w.Add(floor)
		a := NewSphere(0.4, 80)
		a.FixedRotation = true // a player
		a.Position, a.Velocity = mathx.Vec3{0, 2, 0}, mathx.Vec3{3, 0, 0}
		w.Add(a)
		other := NewSphere(0.4, 1)
		other.Position = mathx.Vec3{5, 3, 0}
		w.Add(other)
		return w, a, other
	}
	w1, a1, _ := build()
	w2, a2, other2 := build()
	for range 120 {
		w1.Update(1.0 / 60)
		w2.StepBody(a2, 1.0/60)
	}
	if d := a1.Position.Sub(a2.Position).Len(); d > 0.02 {
		t.Errorf("StepBody ended at %v, Update at %v", a2.Position, a1.Position)
	}
	if !a2.Grounded {
		t.Error("the body should be resting on the floor")
	}
	if other2.Position != (mathx.Vec3{5, 3, 0}) {
		t.Errorf("another body moved: %v", other2.Position)
	}
}
