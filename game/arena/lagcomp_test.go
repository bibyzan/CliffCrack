package arena

import (
	"math"
	"testing"
)

// A shooter aims where a strafing target was 200 ms ago (as a lagged guest
// would see it): with that moment as the shot's ViewTime it hits; judged
// in the present, it misses by the distance the target has moved since.
func TestLagCompensatedShot(t *testing.T) {
	shoot := func(compensate bool) bool {
		a, shooter, target := duel(12)
		arm(shooter, WeaponPistol)
		strafe := Input{Move: [2]float32{1, 0}}
		run(a, 0.5, Input{}, strafe) // up to speed
		seen, seenAt := target.Chest(), a.Time
		run(a, 0.2, Input{}, strafe) // the lag
		to := seen.Sub(shooter.Eye(1))
		shooter.Yaw = float32(math.Atan2(float64(to[0]), float64(-to[2])))
		shooter.Pitch = float32(math.Asin(float64(to.Normalize()[1])))
		in := Input{Fire: true, FirePressed: true}
		if compensate {
			in.ViewTime = seenAt
		}
		ev := a.Step(frame, []Input{in, strafe})
		for _, s := range ev.Shots {
			if s.Victim == target {
				return true
			}
		}
		return false
	}
	if !shoot(true) {
		t.Error("aiming where the target was seen, with lag compensation: missed")
	}
	if shoot(false) {
		t.Error("without lag compensation the shot should miss a target that's moved on")
	}
}

func TestRewindIsBoundedAndRestored(t *testing.T) {
	a, shooter, target := duel(12)
	run(a, 1, Input{}, Input{Move: [2]float32{1, 0}})
	now, me := target.Body.Position, shooter.Body.Position
	undo := a.rewind(shooter, a.Time-1) // too far back: held to maxRewind
	back := target.Body.Position
	undo()
	if target.Body.Position != now {
		t.Fatal("rewind wasn't undone")
	}
	if d := now.Sub(back).Len(); d == 0 || d > walkSpeed*maxRewind*1.2 {
		t.Errorf("rewound %.2f m: want some, but no more than maxRewind's worth", d)
	}
	if shooter.Body.Position != me {
		t.Error("the shooter moved")
	}
}
