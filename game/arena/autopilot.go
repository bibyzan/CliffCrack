package arena

import (
	"math"

	"CliffCrack/engine/mathx"
)

// botTurnRate is how quickly the bot's aim closes on its target (1/s). It has
// to be quick: a drone crossing the view at w rad/s leaves the aim lagging by
// about w / botTurnRate.
const botTurnRate = 14.0

// Autopilot is a simple bot for demos and scripted tests. In the arena it
// turns towards the nearest drone it can see and fires; on a demolition site
// it walks up to the nearest standing structure, shells it with the
// launcher from range and finishes the job with the hammer up close.
func (a *Arena) Autopilot(dt float32) Input {
	if len(a.Structures) > 0 {
		if in, ok := a.demolitionBot(dt); ok {
			return in
		}
	}
	return a.droneBot(dt)
}

func (a *Arena) droneBot(dt float32) Input {
	p := &a.Player
	eye := p.Eye(1)
	var target *Drone
	best := float32(math.MaxFloat32)
	for _, d := range a.Drones {
		if d.Dead {
			continue
		}
		dist := d.Body.Position.Sub(eye).Len()
		if dist >= best || !a.canSee(eye, d) {
			continue
		}
		target, best = d, dist
	}

	in := Input{Move: [2]float32{float32(math.Sin(float64(a.Time) * 0.7)), 0.3}}
	if a.Current != WeaponRifle {
		in.Select = int(WeaponRifle) + 1
	}
	if a.Rifle.Ammo == 0 && a.Rifle.Reloading == 0 {
		in.Reload = true
	}
	if target == nil {
		in.Look[0] = 0.8 * dt // look around for something to shoot
		return in
	}
	// Lead the target slightly by its velocity.
	aim := target.Body.Position.Add(target.Body.Velocity.Scale(best / 300))
	var onTarget bool
	in.Look, onTarget = a.aimAt(aim, float32(math.Atan(float64(DroneRadius*0.8/best))), dt)
	if onTarget {
		in.Fire, in.FirePressed = true, true
	}
	return in
}

// demolitionBot targets the nearest standing chunk near the ground of a real
// building (crates and other small stuff aren't worth its time).
func (a *Arena) demolitionBot(dt float32) (Input, bool) {
	p := &a.Player
	eye := p.Eye(1)
	feet := p.Body.Position
	var target *Chunk
	best := float32(math.MaxFloat32)
	for _, s := range a.Structures {
		if s.Alive() < 20 {
			continue
		}
		for _, c := range s.Chunks {
			if !c.Alive || c.Centre[1] > 2.5 {
				continue
			}
			if d := c.distTo(feet); d < best {
				target, best = c, d
			}
		}
	}
	if target == nil {
		return Input{}, false
	}

	var in Input
	want := WeaponLauncher
	if best < 6 {
		want = WeaponHammer
	}
	if a.Current != want {
		in.Select = int(want) + 1
	}
	if a.Launcher.Ammo == 0 && a.Launcher.Reloading == 0 {
		in.Reload = true
	}
	aim := target.Centre
	if want == WeaponHammer {
		// Swing at the chunk's face nearest the eye.
		for k := 0; k < 3; k++ {
			aim[k] = clamp(eye[k], target.Centre[k]-target.Half[k]*0.8, target.Centre[k]+target.Half[k]*0.8)
		}
	} else {
		aim = eye.Add(lobDirection(aim.Sub(eye)).Scale(10)) // grenades drop: aim high enough to land on it
	}
	var onTarget bool
	in.Look, onTarget = a.aimAt(aim, 0.08, dt)

	switch {
	case want == WeaponHammer && best > 1.6:
		in.Move[1] = 1 // close in for the swing
	case want == WeaponLauncher && best > 22:
		in.Move[1] = 1 // too far to lob accurately
	}
	if onTarget && (want == WeaponLauncher || best <= 2.2) {
		in.Fire, in.FirePressed = true, true
	}
	return in, true
}

// lobDirection is the direction to launch a grenade so it lands at offset
// (from the muzzle), using the flatter of the two ballistic solutions. Out of
// range it aims at 45 degrees.
func lobDirection(offset mathx.Vec3) mathx.Vec3 {
	flat := mathx.Vec3{offset[0], 0, offset[2]}
	x := flat.Len()
	if x < 1e-3 {
		return offset.Normalize()
	}
	const v, g = grenadeSpeed, gravity
	y := offset[1]
	disc := v*v*v*v - g*(g*x*x+2*y*v*v)
	angle := float32(math.Pi / 4)
	if disc >= 0 {
		angle = float32(math.Atan((v*v - math.Sqrt(float64(disc))) / (g * float64(x))))
	}
	h := flat.Scale(1 / x)
	c, s := math.Cos(float64(angle)), math.Sin(float64(angle))
	return mathx.Vec3{h[0] * float32(c), float32(s), h[2] * float32(c)}
}

// aimAt eases the view towards point and reports whether it's within cone
// radians of it.
func (a *Arena) aimAt(point mathx.Vec3, cone, dt float32) ([2]float32, bool) {
	p := &a.Player
	dir := point.Sub(p.Eye(1)).Normalize()
	wantYaw := float32(math.Atan2(float64(dir[0]), float64(-dir[2])))
	wantPitch := float32(math.Asin(float64(clamp(dir[1], -1, 1))))
	dYaw := wrap(wantYaw - p.Yaw)
	dPitch := wantPitch - p.ViewPitch()
	k := 1 - float32(math.Exp(-botTurnRate*float64(dt)))
	return [2]float32{dYaw * k, dPitch * k}, float32(math.Hypot(float64(dYaw), float64(dPitch))) < cone
}

// canSee reports whether the drone is in the open from eye.
func (a *Arena) canSee(eye mathx.Vec3, d *Drone) bool {
	hit, ok := a.Phys.Raycast(eye, d.Body.Position.Sub(eye), 200, a.ignoreForAim)
	return ok && hit.Body == d.Body
}

// wrap brings an angle into [-pi, pi).
func wrap(a float32) float32 {
	return float32(math.Mod(float64(a)+3*math.Pi, 2*math.Pi) - math.Pi)
}
