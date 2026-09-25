package game

import (
	"math"
	"testing"

	"CliffCrack/game/arena"
)

func TestAimAssist(t *testing.T) {
	m := &Arena{match: arena.NewRange()} // dummies down range
	m.match.Phase = arena.PhaseFight
	me := m.me()
	// Aim just to the right of the first dummy's chest (10 m out, facing -X).
	d := m.sim().Players[1]
	run(m.sim(), 0.3)
	to := d.Chest().Sub(me.Eye(1))
	me.Yaw = float32(math.Atan2(float64(to[0]), float64(-to[2]))) + 0.03
	me.Pitch = float32(math.Asin(float64(to.Normalize()[1])))

	slow, pull := m.aimAssist(true, 1.0/60)
	if slow >= 1 || pull[0] >= 0 {
		t.Errorf("just right of a target: slow %v pull %v; want slowed, and pulled left onto it", slow, pull)
	}
	if _, still := m.aimAssist(false, 1.0/60); still != ([2]float32{}) {
		t.Errorf("standing still, the aim was pulled %v", still)
	}
	me.Yaw += 0.5 // well off
	if slow, pull := m.aimAssist(true, 1.0/60); slow != 1 || pull != ([2]float32{}) {
		t.Errorf("far off any target: slow %v pull %v; want no assist", slow, pull)
	}
}

// run steps a with no input.
func run(a *arena.Arena, seconds float32) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		a.Step(1.0/60, nil)
	}
}
