package game

import (
	"math"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// First-person arms: two-bone limbs from shoulders just out of view to the
// hands, which hold the weapon (the firing hand on the grip, the other on
// the fore grip or pump) or work it: pulling a magazine and bringing a new
// one, pushing shells in, swinging a drum out, racking a handle, throwing
// a grenade. Everything is in the weapon model's frame or the camera's, so
// it moves with the view.

// Arm proportions, in the view model's shrunken scale.
const (
	upperArm = 0.22
	forearm  = 0.26
)

var (
	sleeveColor = mathx.SRGB(0.16, 0.32, 0.66, 1)
	bracerColor = mathx.SRGB(0.13, 0.14, 0.17, 1)
	gloveColor  = mathx.SRGB(0.09, 0.09, 0.10, 1)
	shellColor  = mathx.SRGB(0.98, 0.45, 0.10, 1)
)

// limb draws a box from a to b, half as thick as r.
func (m *Arena) limb(out []render.DrawCmd, a, b mathx.Vec3, r float32, col [4]float32) []render.DrawCmd {
	d := b.Sub(a)
	l := d.Len()
	if l < 1e-4 {
		return out
	}
	mid := a.Add(b).Scale(0.5)
	model := mathx.Translate(mid[0], mid[1], mid[2]).Mul(mathx.LookRotation(d.Scale(1 / l)).Mat4()).
		Mul(mathx.Scale(r, r, l/2))
	return append(out, render.DrawCmd{Model: model, Color: col, Flags: gfx.DrawFlat, Mesh: m.as.cube})
}

// arm draws an arm from shoulder to hand (world space), the elbow bending
// towards pole: sleeve, armoured forearm and glove. If the hand is out of
// reach the shoulder follows it, so the hand is always where it's put.
func (m *Arena) arm(out []render.DrawCmd, shoulder, hand, pole mathx.Vec3) []render.DrawCmd {
	reach := float32(upperArm + forearm)
	to := hand.Sub(shoulder)
	d := to.Len()
	if d > reach*0.98 {
		shoulder = hand.Sub(to.Scale(reach * 0.98 / d))
		to, d = hand.Sub(shoulder), reach*0.98
	}
	d = max(d, 0.05)
	dir := to.Scale(1 / d)
	a := (upperArm*upperArm - forearm*forearm + d*d) / (2 * d)
	h := float32(math.Sqrt(float64(max(upperArm*upperArm-a*a, 0))))
	side := pole.Sub(dir.Scale(pole.Dot(dir))).Normalize()
	elbow := shoulder.Add(dir.Scale(a)).Add(side.Scale(h))

	out = m.limb(out, shoulder, elbow, 0.02, sleeveColor)
	out = m.limb(out, elbow, hand, 0.016, sleeveColor)
	wrist := hand.Sub(hand.Sub(elbow).Normalize().Scale(0.035))
	out = m.limb(out, elbow.Add(wrist.Sub(elbow).Scale(0.4)), wrist, 0.018, bracerColor)
	// The glove: a fist round the grip.
	fwd := hand.Sub(elbow).Normalize()
	out = m.limb(out, wrist, hand.Add(fwd.Scale(0.008)), 0.014, gloveColor)
	return out
}

// reloadPose is the supporting hand and the magazine partway through a
// reload: where the hand is and what it carries, in weapon space.
type reloadPose struct {
	hand     mathx.Vec3 // the supporting hand
	magOff   mathx.Vec3 // how far the magazine (or drum) is from its seat
	shell    bool       // holding a shell (at hand)
	handleAt float32    // 0..1 how far a charging handle is pulled
}

// belt is where the supporting hand goes for fresh ammo: down out of view.
var belt = mathx.Vec3{-0.1, -0.5, 0.15}

func lerp3(a, b mathx.Vec3, t float32) mathx.Vec3 { return a.Add(b.Sub(a).Scale(t)) }

// phase is how far t is through [a, b], eased.
func phase(t, a, b float32) float32 { return smooth((t - a) / (b - a)) }

// reloading works out the pose t (0..1) through mk's reload.
func reloading(mk *marker, t float32) reloadPose {
	var p reloadPose
	seat := mk.magHold
	drop := mk.magDrop.Normalize()
	switch mk.reload {
	case reloadShells:
		// Down for a shell, up to the port underneath, and push it in.
		port := mk.magHold
		under := port.Add(mathx.Vec3{-0.02, -0.07, 0.03})
		switch {
		case t < 0.35:
			p.hand = lerp3(mk.fore, belt, phase(t, 0, 0.35))
		case t < 0.75:
			p.hand, p.shell = lerp3(belt, under, phase(t, 0.35, 0.75)), true
		default:
			p.hand, p.shell = lerp3(under, port, phase(t, 0.75, 0.95)), t < 0.95
		}
	case reloadDrum:
		out := drop.Scale(0.08)
		switch {
		case t < 0.12: // take hold of the drum
			p.hand = lerp3(mk.fore, seat, phase(t, 0, 0.12))
		case t < 0.3: // swing it out
			p.magOff = out.Scale(phase(t, 0.12, 0.3))
			p.hand = seat.Add(p.magOff)
		case t < 0.45: // let it go, and reach for another
			s := (t - 0.3) / 0.15
			p.magOff = out.Add(mathx.Vec3{0, -0.7 * s * s, 0})
			p.hand = lerp3(seat.Add(out), belt, phase(t, 0.3, 0.45))
		case t < 0.7: // bring it up
			p.hand = lerp3(belt, seat.Add(out), phase(t, 0.45, 0.7))
			p.magOff = p.hand.Sub(seat)
		case t < 0.85: // swing it in
			p.magOff = out.Scale(1 - phase(t, 0.7, 0.85))
			p.hand = seat.Add(p.magOff)
		default: // slap it home and take hold again
			p.hand = lerp3(seat, mk.fore, phase(t, 0.88, 1))
		}
	default:
		pulled := drop.Scale(0.04)
		ready := drop.Scale(0.1)
		switch {
		case t < 0.12: // take hold of the magazine
			p.hand = lerp3(mk.fore, seat, phase(t, 0, 0.12))
		case t < 0.32: // pull it and let it fall
			s := (t - 0.12) / 0.2
			p.magOff = pulled.Scale(phase(t, 0.12, 0.18)).Add(mathx.Vec3{0, -0.9 * s * s, 0})
			p.hand = lerp3(seat.Add(pulled), belt, phase(t, 0.16, 0.32))
		case t < 0.55: // bring up a fresh one
			p.hand = lerp3(belt, seat.Add(ready), phase(t, 0.32, 0.55))
			p.magOff = p.hand.Sub(seat)
		case t < 0.68: // seat it
			p.magOff = ready.Scale(1 - phase(t, 0.55, 0.68))
			p.hand = seat.Add(p.magOff)
		case t < 0.88 && mk.charge != (mathx.Vec3{}): // work the handle
			c := (t - 0.68) / 0.2
			p.hand = lerp3(seat, mk.charge, phase(c, 0, 0.35))
			p.handleAt = phase(c, 0.35, 0.6) * (1 - phase(c, 0.7, 1))
			p.hand = p.hand.Add(mathx.Vec3{0, 0, 0.05 * p.handleAt})
		default: // take hold again
			from := seat
			if mk.charge != (mathx.Vec3{}) {
				from = mk.charge
			}
			p.hand = lerp3(from, mk.fore, phase(t, 0.88, 1))
		}
	}
	return p
}

// reloadCues are the moments in a reload that make a sound, by style:
// the magazine coming out, going in, and the handle.
var reloadCues = [...][3]float32{
	reloadMag:    {0.14, 0.62, 0.78},
	reloadShells: {-1, 0.82, -1},
	reloadDrum:   {0.2, 0.8, 0.9},
}

// shoulders are where the arms start, in camera space: low and just out
// of view. poles are which way each elbow bends.
var (
	rightShoulder = mathx.Vec3{0.24, -0.42, -0.2}
	leftShoulder  = mathx.Vec3{-0.2, -0.44, -0.3}
	rightPole     = mathx.Vec3{0.8, -1, 0.2}
	leftPole      = mathx.Vec3{-0.8, -1, 0.1}
)

// appendArms draws the view model's arms and whatever the supporting hand
// is carrying, for the pose worked out in appendWeapon.
func (m *Arena) appendArms(out []render.DrawCmd, model mathx.Mat4, mk *marker, pose reloadPose, reloadingNow bool) []render.DrawCmd {
	cam := m.camWorld()
	w := func(p mathx.Vec3) mathx.Vec3 { return model.TransformPoint(p) } // weapon space to world
	c := func(p mathx.Vec3) mathx.Vec3 { return cam.TransformPoint(p) }   // camera space to world
	dirC := func(v mathx.Vec3) mathx.Vec3 { return c(v).Sub(c(mathx.Vec3{})) }

	held := heldKind(&m.me().Weapons)
	var grip, fore mathx.Vec3
	switch {
	case held == arena.WeaponHammer:
		grip, fore = w(mathx.Vec3{0, -0.33, 0}), w(mathx.Vec3{0, -0.08, 0})
	case mk != nil:
		grip, fore = w(mk.grip), w(mk.fore)
		if mk.pump != nil {
			fore = w(mk.fore.Add(m.pumpOffset()))
		}
		if reloadingNow {
			fore = w(pose.hand)
		}
	default:
		return out
	}
	// Throwing a grenade: the supporting hand leaves the gun, comes back
	// past the ear and throws forward.
	if m.throwAnim > 0.05 {
		t := 1 - m.throwAnim
		back, release := mathx.Vec3{-0.2, 0.02, -0.12}, mathx.Vec3{-0.05, -0.02, -0.55}
		hand := c(lerp3(back, release, smooth(t*1.6)))
		fore = lerp3(fore, hand, min(m.throwAnim*3, 1))
		if t < 0.4 {
			out = append(out, render.DrawCmd{Model: bodyMatrix(fore, mathx.QuatIdentity(), 0.03), Color: fragOlive,
				Flags: gfx.DrawFlat, Mesh: m.sc.ball})
		}
	}
	out = m.arm(out, c(rightShoulder), grip, dirC(rightPole))
	out = m.arm(out, c(leftShoulder), fore, dirC(leftPole))
	if pose.shell && reloadingNow {
		// A shell in the fingers.
		sh := model.Mul(mathx.Translate(pose.hand[0], pose.hand[1]+0.02, pose.hand[2])).Mul(mathx.Scale(0.011, 0.011, 0.028))
		out = append(out, render.DrawCmd{Model: sh, Color: shellColor, Mesh: m.as.cube})
	}
	return out
}

// pumpOffset is how far the shotgun's pump is racked back right now: back
// and forward once after each shot.
func (m *Arena) pumpOffset() mathx.Vec3 {
	s := m.me().States[arena.WeaponShotgun].SinceShot
	p := (s - 0.12) / 0.45
	if p <= 0 || p >= 1 {
		return mathx.Vec3{}
	}
	return mathx.Vec3{0, 0, 0.075 * float32(math.Sin(math.Pi*float64(p)))}
}

// translate is a translation matrix by v.
func translate(v mathx.Vec3) mathx.Mat4 { return mathx.Translate(v[0], v[1], v[2]) }

// reloadCues plays a reload's sounds as it passes them: the magazine out
// and in, the handle; a shell going in; the shotgun's pump after a shot.
func (m *Arena) reloadCues(me *arena.Player) {
	t, ok := me.Reloading()
	mk, isGun := m.markerFor(me.Current)
	if ok && isGun {
		prev := m.reloadT
		if t < prev {
			prev = 0 // the next shell
		}
		cues := reloadCues[mk.reload]
		sounds := []struct {
			at  float32
			snd *audio.Sound
		}{{cues[0], m.sfx.magOut}, {cues[1], m.sfx.magIn}, {cues[2], m.sfx.charge}}
		if mk.reload == reloadShells {
			sounds[1].snd = m.sfx.shellIn
		}
		for _, c := range sounds {
			if c.at >= 0 && prev < c.at && t >= c.at {
				if c.at == cues[2] && mk.charge == (mathx.Vec3{}) {
					continue
				}
				m.play(c.snd, 0.9)
			}
		}
		m.reloadT = t
	} else {
		m.reloadT = 0
	}
	pump := me.States[arena.WeaponShotgun].SinceShot
	if me.Current == arena.WeaponShotgun && m.lastPump < 0.14 && pump >= 0.14 {
		m.play(m.sfx.pump, 0.9)
	}
	m.lastPump = pump
}
