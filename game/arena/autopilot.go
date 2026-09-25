package arena

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// botTurnRate is how quickly the bot's aim closes on its target (1/s). It has
// to be quick: a drone crossing the view at w rad/s leaves the aim lagging by
// about w / botTurnRate.
const botTurnRate = 14.0

// Autopilot is a simple bot for demos and scripted tests: it turns towards
// the nearest drone it can see, fires when on target, reloads when empty and
// strafes to keep moving.
func (a *Arena) Autopilot(dt float32) Input {
	p := &a.Player
	eye := p.Eye(1)
	var target *Drone
	best := float32(math.MaxFloat32)
	for _, d := range a.Drones {
		if d.Dead {
			continue
		}
		to := d.Body.Position.Sub(eye)
		dist := to.Len()
		if dist >= best || !a.canSee(eye, d) {
			continue
		}
		target, best = d, dist
	}

	in := Input{Move: [2]float32{float32(math.Sin(float64(a.Time) * 0.7)), 0.3}}
	if a.Weapon.Ammo == 0 && a.Weapon.Reloading == 0 {
		in.Reload = true
	}
	if target == nil {
		in.Look[0] = 0.8 * dt // look around for something to shoot
		return in
	}
	// Lead the target slightly by its velocity.
	aim := target.Body.Position.Add(target.Body.Velocity.Scale(best / 300)).Sub(eye).Normalize()
	wantYaw := float32(math.Atan2(float64(aim[0]), float64(-aim[2])))
	wantPitch := float32(math.Asin(float64(clamp(aim[1], -1, 1))))
	dYaw := wrap(wantYaw - p.Yaw)
	dPitch := wantPitch - p.ViewPitch()
	k := 1 - float32(math.Exp(-botTurnRate*float64(dt)))
	in.Look = [2]float32{dYaw * k, dPitch * k}
	// Fire when the aim is within most of the drone's apparent size.
	cone := float32(math.Atan(float64(DroneRadius * 0.8 / best)))
	if float32(math.Hypot(float64(dYaw), float64(dPitch))) < cone {
		in.Fire = true
		in.FirePressed = true
	}
	return in
}

// canSee reports whether the drone is in the open from eye.
func (a *Arena) canSee(eye mathx.Vec3, d *Drone) bool {
	hit, ok := a.Phys.Raycast(eye, d.Body.Position.Sub(eye), 200, func(b *physics.Body) bool {
		_, debris := b.UserData.(*Debris)
		return b == a.Player.Body || debris
	})
	return ok && hit.Body == d.Body
}

// wrap brings an angle into [-pi, pi).
func wrap(a float32) float32 {
	return float32(math.Mod(float64(a)+3*math.Pi, 2*math.Pi) - math.Pi)
}
