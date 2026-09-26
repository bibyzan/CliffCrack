package game

import (
	"math"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// First-person arms: two-bone limbs from shoulders just out of view to
// gloved hands closed round what they hold, round and smooth shaded. The
// hands hold the weapon (the firing hand on the grip, the other on
// the fore grip or pump) or work it: pulling a magazine and bringing a new
// one, pushing shells in, swinging a drum out, racking a handle, throwing
// a grenade. Everything is in the weapon model's frame or the camera's, so
// it moves with the view.

// Arm proportions, in the view model's shrunken scale: lengths, and the
// radii the round limbs taper through.
const (
	upperArm = 0.22
	forearm  = 0.26

	shoulderR = 0.042
	elbowR    = 0.031
	wristR    = 0.02
	fingerR   = 0.0058
	thumbR    = 0.0068
)

var (
	sleeveColor = mathx.SRGB(0.16, 0.32, 0.66, 1)
	bracerColor = mathx.SRGB(0.13, 0.14, 0.17, 1)
	gloveColor  = mathx.SRGB(0.16, 0.16, 0.18, 1)
	shellColor  = mathx.SRGB(0.98, 0.45, 0.10, 1)
)

// segment draws a round limb from a (radius ra) to b (radius rb), smooth
// shaded; the joints at its ends are drawn separately, as balls.
func (m *Arena) segment(out []render.DrawCmd, a, b mathx.Vec3, ra, rb float32, col [4]float32) []render.DrawCmd {
	d := b.Sub(a)
	l := d.Len()
	if l < 1e-4 || ra <= 0 {
		return out
	}
	mid := a.Add(b).Scale(0.5)
	model := mathx.Translate(mid[0], mid[1], mid[2]).Mul(mathx.LookRotation(d.Scale(1 / l)).Mat4()).
		Mul(mathx.Scale(ra, ra, l/2))
	return append(out, render.DrawCmd{Model: model, Color: col, Mesh: m.as.tube(rb / ra)})
}

// joint draws a smooth ball of radius r at p.
func (m *Arena) joint(out []render.DrawCmd, p mathx.Vec3, r float32, col [4]float32) []render.DrawCmd {
	return append(out, render.DrawCmd{Model: mathx.Translate(p[0], p[1], p[2]).Mul(mathx.Scale(r, r, r)), Color: col, Mesh: m.as.ball})
}

// ellipsoid draws a smooth ball at p with half extents along the unit axes
// x, y and z (which needn't be a rotation's: they only have to be unit).
func (m *Arena) ellipsoid(out []render.DrawCmd, p, x, y, z mathx.Vec3, hx, hy, hz float32, col [4]float32) []render.DrawCmd {
	sx, sy, sz := x.Scale(hx), y.Scale(hy), z.Scale(hz)
	model := mathx.Mat4{ // column-major: the scaled axes, then the position
		sx[0], sx[1], sx[2], 0,
		sy[0], sy[1], sy[2], 0,
		sz[0], sz[1], sz[2], 0,
		p[0], p[1], p[2], 1,
	}
	return append(out, render.DrawCmd{Model: model, Color: col, Mesh: m.as.ball})
}

// hold is what a hand closes round: a grip (or handle, or handguard) at
// at, running along axis, radius thick; the fingers wrap round it through
// the side towards. A firing hand's index finger rests on trigger (if set)
// instead of wrapping.
type hold struct {
	at, axis, towards mathx.Vec3
	radius            float32
	trigger           *mathx.Vec3
}

// arm draws an arm from shoulder to a hand holding h (world space), the
// elbow bending towards pole: a round shoulder and sleeve, an elbow pad,
// an armoured forearm, and a gloved hand closed round the grip: the palm
// flat against it on the side the arm comes from, four fingers wrapped
// round the front, the thumb over the other side. If the hand is out of
// reach the shoulder follows it, so the hand is always where it's put.
func (m *Arena) arm(out []render.DrawCmd, shoulder mathx.Vec3, h hold, pole mathx.Vec3) []render.DrawCmd {
	// The palm faces back along the arm; where the wrist goes depends on the
	// elbow, and the elbow on the wrist, so settle it in two passes.
	palmSide := func(from mathx.Vec3) mathx.Vec3 {
		u := from.Sub(h.at)
		u = u.Sub(h.axis.Scale(u.Dot(h.axis)))
		if u.Len() < 1e-4 {
			return pole.Normalize()
		}
		return u.Normalize()
	}
	u := palmSide(shoulder)
	var elbow, wrist mathx.Vec3
	for range 2 {
		wrist = h.at.Add(u.Scale(h.radius + 0.028))
		shoulder, elbow = solveArm(shoulder, wrist, pole)
		u = palmSide(elbow)
	}
	wrist = h.at.Add(u.Scale(h.radius + 0.028))
	v := h.axis.Cross(u).Normalize() // round the grip, towards the fingers' side
	if v.Dot(h.towards) < 0 {
		v = v.Scale(-1)
	}

	// Shoulder and upper arm, the elbow and its pad.
	out = m.joint(out, shoulder, shoulderR, sleeveColor)
	out = m.segment(out, shoulder, elbow, shoulderR, elbowR*1.04, sleeveColor)
	out = m.joint(out, elbow, elbowR*1.04, sleeveColor)
	out = m.joint(out, elbow.Add(pole.Normalize().Scale(elbowR*0.5)), elbowR*0.8, bracerColor)
	// The forearm, and the armoured bracer over its wrist half.
	out = m.segment(out, elbow, wrist, elbowR, wristR, sleeveColor)
	out = m.segment(out, lerp3(elbow, wrist, 0.5), lerp3(elbow, wrist, 0.93),
		elbowR+(wristR-elbowR)*0.5+0.003, wristR*1.1+0.003, bracerColor)
	out = m.joint(out, wrist, wristR*0.95, gloveColor)

	// The hand: a flat palm against the grip, reaching from the wrist.
	fr := float32(fingerR)
	span := 2.15 * fr // one finger's breadth
	ring := h.radius + fr
	at := func(deg, along float32) mathx.Vec3 { // a point on the fingers' ring round the grip
		a := float64(deg) * math.Pi / 180
		return h.at.Add(h.axis.Scale(along)).Add(u.Scale(ring * float32(math.Cos(a)))).Add(v.Scale(ring * float32(math.Sin(a))))
	}
	palm := h.at.Add(u.Scale(h.radius + 0.011)).Add(v.Scale(0.004))
	out = m.segment(out, wrist, palm, wristR*0.95, 0.017, gloveColor)
	out = m.ellipsoid(out, palm, h.axis, v, u, span*2.1, 0.019, 0.009, gloveColor)

	// Fingers: four, each three knuckles round the grip, stacked along it;
	// the firing hand's first rests on the trigger instead.
	finger := func(pts ...mathx.Vec3) {
		out = m.joint(out, pts[0], fr*1.15, gloveColor)
		for i := 1; i < len(pts); i++ {
			out = m.segment(out, pts[i-1], pts[i], fr*1.05, fr, gloveColor)
			out = m.joint(out, pts[i], fr, gloveColor)
		}
	}
	for k := range 4 {
		along := (float32(k) - 1.5) * span
		if k == 0 && h.trigger != nil {
			base := at(25, along)
			tip := *h.trigger
			bend := lerp3(base, tip, 0.5).Add(u.Scale(fr * 0.6))
			finger(base, bend, tip)
			continue
		}
		finger(at(25, along), at(95, along), at(170, along), at(225, along))
	}
	// The thumb: from the heel of the palm, over the top, round the other side.
	top := -2.1 * span
	out = m.joint(out, at(-35, top*0.7), thumbR*1.1, gloveColor)
	out = m.segment(out, at(-35, top*0.7), at(-90, top), thumbR*1.05, thumbR, gloveColor)
	out = m.joint(out, at(-90, top), thumbR, gloveColor)
	out = m.segment(out, at(-90, top), at(-140, top*0.9), thumbR, thumbR*0.9, gloveColor)
	out = m.joint(out, at(-140, top*0.9), thumbR*0.9, gloveColor)
	return out
}

// solveArm places the elbow for a hand (the wrist) at hand, bending
// towards pole. If the hand is out of reach the shoulder follows it.
func solveArm(shoulder, hand, pole mathx.Vec3) (mathx.Vec3, mathx.Vec3) {
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
	return shoulder, shoulder.Add(dir.Scale(a)).Add(side.Scale(h))
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
	dir := func(v mathx.Vec3) mathx.Vec3 { return w(v).Sub(w(mathx.Vec3{})).Normalize() } // a weapon-space direction, in world
	down, fwd, right := dir(mathx.Vec3{0, -1, 0}), dir(mathx.Vec3{0, 0, -1}), dir(mathx.Vec3{1, 0, 0})
	var grip, fore hold
	switch {
	case held == arena.WeaponHammer:
		// Both hands on the haft, the lower one at its end.
		grip = hold{at: w(mathx.Vec3{0, -0.33, 0}), axis: down, towards: fwd, radius: 0.011}
		fore = hold{at: w(mathx.Vec3{0, -0.08, 0}), axis: down, towards: fwd, radius: 0.011}
	case mk != nil:
		size := mk.size
		trigger := w(mk.grip.Add(mathx.Vec3{0, 0.03, -0.047}))
		grip = hold{at: w(mk.grip), axis: down, towards: fwd, radius: 0.02 * size, trigger: &trigger}
		if mk.bolt != nil {
			// Working the bolt: the firing hand leaves the grip for the knob,
			// rides it up and back and home, and returns.
			lift, back, reach := boltAt(m.me().States[arena.WeaponSniper].SinceShot)
			if reach > 0 {
				knob := w(boltFrame(mk, lift, back).TransformPoint(mk.charge))
				grip.at = lerp3(grip.at, knob, reach)
				grip.axis = lerp3(down, fwd, reach).Normalize()
				grip.towards = lerp3(fwd, down, reach).Normalize()
				grip.radius = 0.02*size + (0.012-0.02*size)*reach
				if reach > 0.3 {
					grip.trigger = nil
				}
			}
		}
		foreAt := mk.fore
		if mk.pump != nil {
			foreAt = foreAt.Add(m.pumpOffset())
		}
		fore = hold{at: w(foreAt), axis: down, towards: fwd, radius: 0.022 * size}
		if mk.foreFlat {
			// Under a pump or handguard: the palm beneath, the fingers up
			// its right side.
			fore.axis, fore.towards, fore.radius = fwd, right, 0.03*size
		}
		if reloadingNow {
			fore = hold{at: w(pose.hand), axis: right, towards: fwd, radius: 0.016}
		}
	default:
		return out
	}
	// Throwing a grenade: the supporting hand leaves the gun, comes back
	// past the ear and throws forward, a grenade in the fist until it goes.
	if m.throwAnim > 0.05 {
		t := 1 - m.throwAnim
		back, release := mathx.Vec3{-0.2, 0.02, -0.12}, mathx.Vec3{-0.05, -0.02, -0.55}
		hand := c(lerp3(back, release, smooth(t*1.6)))
		fore.at = lerp3(fore.at, hand, min(m.throwAnim*3, 1))
		fore.axis, fore.towards, fore.radius = dirC(mathx.Vec3{1, 0, 0}).Normalize(), dirC(mathx.Vec3{0, 0, -1}).Normalize(), 0.016
		if t < 0.4 {
			out = append(out, render.DrawCmd{Model: bodyMatrix(fore.at, mathx.QuatIdentity(), 0.03), Color: fragOlive,
				Flags: gfx.DrawFlat, Mesh: m.as.gem})
		}
	}
	// Vaulting or climbing: the hands go to the ledge's lip (the off hand
	// for a vault, both for a climb), flat on the top, fingers over the far
	// side, and stay there in the world as the body pulls up past them.
	if e := mantleReach(m.me()); e > 0 {
		mt := &m.me().Mantle
		edge := mt.Dir.Cross(mathx.Vec3{0, 1, 0}).Normalize() // along the lip, to the right
		lip := mt.Top.Sub(mt.Dir.Scale(0.14)).Add(mathx.Vec3{0, 0.012, 0})
		ledge := func(side float32) hold {
			return hold{at: lip.Add(edge.Scale(0.2 * side)), axis: edge, towards: mt.Dir, radius: 0.012}
		}
		blend := func(from hold, to hold) hold {
			return hold{at: lerp3(from.at, to.at, e), axis: lerp3(from.axis, to.axis, e).Normalize(),
				towards: lerp3(from.towards, to.towards, e).Normalize(), radius: from.radius + (to.radius-from.radius)*e}
		}
		fore = blend(fore, ledge(-1))
		if !mt.Vault {
			grip = blend(grip, ledge(1))
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

// The pump and the bolt, after each shot (times in s since it).
const (
	pumpStart, pumpBack, pumpHold, pumpHome = 0.1, 0.3, 0.4, 0.58 // snapped back, held a beat, slammed home
	pumpTravel                              = 0.12                // m, in the model's scale

	boltGrab, boltLift, boltBack, boltFwd, boltDown, boltLetGo = 0.12, 0.26, 0.44, 0.62, 0.76, 0.9
	boltTravel                                                 = 0.08 // m back
)

// pumpAt is how far back the shotgun's pump is (0..1) s after a shot.
func pumpAt(s float32) float32 {
	switch {
	case s < pumpStart || s > pumpHome:
		return 0
	case s < pumpBack:
		return phase(s, pumpStart, pumpBack)
	case s < pumpHold:
		return 1
	default:
		return 1 - phase(s, pumpHold, pumpHome)
	}
}

// pumpOffset is where the shotgun's pump is right now.
func (m *Arena) pumpOffset() mathx.Vec3 {
	return mathx.Vec3{0, 0, pumpTravel * pumpAt(m.me().States[arena.WeaponShotgun].SinceShot)}
}

// boltAt is the bolt s after a shot: how far it's turned up (0..1) and
// pulled back (0..1), and how far the firing hand has gone from the grip
// to its knob (0..1).
func boltAt(s float32) (lift, back, hand float32) {
	if s >= boltLetGo+0.12 {
		return 0, 0, 0
	}
	lift = phase(s, boltGrab, boltLift) * (1 - phase(s, boltFwd, boltDown))
	back = phase(s, boltLift, boltBack) * (1 - phase(s, boltBack+0.04, boltFwd))
	hand = phase(s, 0.02, boltGrab) * (1 - phase(s, boltDown, boltLetGo+0.12))
	return
}

// boltFrame places the bolt's parts: turned about the bore and pulled back.
func boltFrame(mk *marker, lift, back float32) mathx.Mat4 {
	p := mk.boltPivot
	return translate(p).Mul(mathx.Translate(0, 0, boltTravel*back)).Mul(mathx.RotateZ(1.3 * lift)).
		Mul(mathx.Translate(-p[0], -p[1], -p[2]))
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
	// The shotgun's pump as it snaps back; the sniper's bolt as it's pulled.
	pump := me.States[arena.WeaponShotgun].SinceShot
	if me.Current == arena.WeaponShotgun && m.lastPump < pumpBack-0.06 && pump >= pumpBack-0.06 {
		m.play(m.sfx.pump, 1)
	}
	m.lastPump = pump
	bolt := me.States[arena.WeaponSniper].SinceShot
	if me.Current == arena.WeaponSniper && m.lastBolt < boltBack-0.08 && bolt >= boltBack-0.08 {
		m.play(m.sfx.charge, 1)
	}
	m.lastBolt = bolt
}
