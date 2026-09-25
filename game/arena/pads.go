package arena

import "CliffCrack/engine/mathx"

const (
	padGrace    = 0.15 // s after a launch before the ground can catch you again
	padCooldown = 0.6  // s before the same player can be thrown again
)

// usePads throws p if they're standing on a launch pad. The launch bays'
// pads only fire once the round is live.
func (a *Arena) usePads(p *Player, ev *Events) {
	if p.Dead || p.sincePad < padCooldown {
		return
	}
	feet := p.Body.Position.Sub(mathx.Vec3{0, PlayerRadius, 0})
	for _, pad := range a.Pads {
		if pad.Spawn && !a.Live {
			continue
		}
		if abs(feet[1]-pad.Centre[1]) > 0.3 || flat(feet.Sub(pad.Centre)).Len() > pad.Radius {
			continue
		}
		p.Body.Velocity = pad.Launch
		p.onGround, p.sincePad = false, 0
		ev.act(p, ActBoost, 0)
		return
	}
}
