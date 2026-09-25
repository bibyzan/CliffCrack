package arena

import (
	"math"

	"CliffCrack/engine/mathx"
)

const (
	padGrace    = 0.15 // s after a launch before the ground can catch you again
	padCooldown = 0.6  // s before the same player can be thrown again
)

// usePads throws p if they're standing on a launch pad. The launch bays'
// pads only fire once the round is live.
func (a *Arena) usePads(p *Player, ev *Events) {
	if p.Dead || p.sincePad < padCooldown || !p.Body.Grounded {
		return // only when standing on one: flying over a pad doesn't fire it
	}
	feet := p.Body.Position.Sub(mathx.Vec3{0, PlayerRadius, 0})
	for _, pad := range a.Pads {
		if pad.Spawn && !a.Live {
			continue
		}
		if abs(feet[1]-pad.Centre[1]) > 0.3 || flat(feet.Sub(pad.Centre)).Len() > pad.Radius {
			continue
		}
		if !a.PadWorks(pad) {
			continue
		}
		launch := pad.Launch
		if pad.Aimed {
			launch = pad.throwFrom(p.Body.Position)
		}
		p.Body.Velocity = launch
		p.onGround, p.sincePad, p.boosted = false, 0, true
		p.boostTop = flat(launch).Len()
		ev.act(p, ActBoost, 0)
		return
	}
}

// aimed is a pad that throws whoever steps on it (anywhere on it) up at
// lift m/s and across to land at target.
func aimed(centre, target mathx.Vec3, lift, radius float32) Pad {
	return Pad{Centre: centre, Radius: radius, Launch: mathx.Vec3{0, lift, 0}, Target: target, Aimed: true}
}

// throwFrom is the launch that takes a player at pos to the pad's target:
// straight up at its lift, and across as far as the flight allows.
func (pad Pad) throwFrom(pos mathx.Vec3) mathx.Vec3 {
	lift := pad.Launch[1]
	rise := pad.Target[1] + PlayerRadius + 0.02 - pos[1]
	disc := max(lift*lift-2*gravity*rise, 0)
	t := (lift + float32(math.Sqrt(float64(disc)))) / gravity // coming down onto it
	across := flat(pad.Target.Sub(pos)).Scale(1 / t)
	return mathx.Vec3{across[0], lift, across[2]}
}

// PadWorks reports whether the floor under a pad is still there: blow it
// out and the pad goes with it.
func (a *Arena) PadWorks(pad Pad) bool {
	from := pad.Centre.Add(mathx.Vec3{0, 0.2, 0})
	hit, ok := a.Phys.Raycast(from, mathx.Vec3{0, -1, 0}, 0.5, a.ignoreForAim)
	return ok && hit.Distance < 0.4
}
