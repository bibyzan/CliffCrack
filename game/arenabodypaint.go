package game

import (
	"math"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// Getting shot, after THE FINALS' coins: every paintball that hits a player
// leaves a splat on their suit, where it hit, that stays for the round; and
// the paint bursts off them in droplets that fly, bounce once and splat
// where they land. Off armour it ricochets, a big spray; into a popped
// player it mostly sticks.

// The body's bones a splat can be on, and how it's drawn off them.
const (
	boneTorso = iota
	boneHead
	boneRightLeg
	boneLeftLeg
)

const (
	maxBodyPaint = 48  // splats per player (the oldest go)
	maxDroplets  = 360 // paint in flight
	dropGravity  = 14  // m/s^2 (a bit more than the players', to look weighty)
	dropLife     = 1.6 // s at most in the air
)

// bodyPaint is a splat on a player: on a bone, at a point on its surface
// and facing along its normal, both in the bone's frame.
type bodyPaint struct {
	bone       int
	at, normal mathx.Vec3
	size, spin float32
	colour     [4]float32
	drops      [3]mathx.Vec3 // as splat.drops
}

// droplet is paint flying off a hit.
type droplet struct {
	at, vel mathx.Vec3
	size    float32
	colour  [4]float32
	age     float32
	bounced bool
}

// onEllipse pushes q (relative to a centre) out or in along its horizontal
// direction onto an ellipse of half widths a (x) and b (z), returning the
// point and its outward normal.
func onEllipse(q mathx.Vec3, a, b float32) (mathx.Vec3, mathx.Vec3) {
	x, z := q[0], q[2]
	if x*x+z*z < 1e-8 {
		z = -1 // dead centre: the front
	}
	t := 1 / float32(math.Sqrt(float64(x*x/(a*a)+z*z/(b*b))))
	x, z = x*t, z*t
	return mathx.Vec3{x, q[1], z}, mathx.Vec3{x / (a * a), 0, z / (b * b)}.Normalize()
}

// onEllipsoid pushes q (relative to a centre) onto an ellipsoid of half
// sizes r, returning the point and its outward normal.
func onEllipsoid(q, r mathx.Vec3) (mathx.Vec3, mathx.Vec3) {
	if q.Len() < 1e-4 {
		q = mathx.Vec3{0, 0, -1}
	}
	s := float32(math.Sqrt(float64(q[0]*q[0]/(r[0]*r[0]) + q[1]*q[1]/(r[1]*r[1]) + q[2]*q[2]/(r[2]*r[2]))))
	p := q.Scale(1 / s)
	return p, mathx.Vec3{p[0] / (r[0] * r[0]), p[1] / (r[1] * r[1]), p[2] / (r[2] * r[2])}.Normalize()
}

// paintOn sticks a splat on p where the paint hit (world space), onto the
// nearest part of their body as it's drawn (the hitbox is roomier than the
// model), and returns where that is and which way it faces, in the world.
func (m *Arena) paintOn(p *arena.Player, hit mathx.Vec3, size float32, colour [4]float32) (at, normal mathx.Vec3) {
	feet := p.Body.Position.Sub(mathx.Vec3{0, arena.PlayerRadius, 0})
	yaw := mathx.RotateY(p.Yaw)
	local := yaw.TransformPoint(hit.Sub(feet)) // the body's frame: -Z ahead, feet at the origin
	drop := arena.CrouchDrop * p.Crouch
	if local[1] > hipHeight-drop {
		local[1] += drop // (the upper body is drawn lowered by drop: see appendCharacter)
	}
	bp := bodyPaint{size: size * (0.8 + 0.4*m.rng.Float32()), spin: m.rng.Float32() * 2 * math.Pi, colour: colour}
	switch {
	case local[1] > neckHeight+0.02:
		bp.bone = boneHead
		bp.at, bp.normal = onEllipsoid(local.Sub(mathx.Vec3{0, neckHeight + 0.16, 0.01}), mathx.Vec3{0.155, 0.17, 0.165})
		bp.at = bp.at.Add(mathx.Vec3{0, 0.16, 0.01})
	case local[1] > hipHeight:
		bp.bone = boneTorso
		q := mathx.Vec3{local[0], clampf(local[1], hipHeight+0.02, neckHeight-0.02), local[2]}
		bp.at, bp.normal = onEllipse(q, 0.24, 0.165)
	default:
		side := float32(1)
		bp.bone = boneRightLeg
		if local[0] < 0 {
			side, bp.bone = -1, boneLeftLeg
		}
		q := local.Sub(mathx.Vec3{0.12 * side, hipHeight, 0})
		q[1] = clampf(q[1], -0.82, -0.02)
		bp.at, bp.normal = onEllipse(q, 0.1, 0.105)
	}
	for i := range bp.drops {
		a := m.rng.Float64() * 2 * math.Pi
		r := bp.size * (0.9 + 0.5*m.rng.Float32())
		bp.drops[i] = mathx.Vec3{r * float32(math.Cos(a)), r * float32(math.Sin(a)), bp.size * (0.15 + 0.2*m.rng.Float32())}
	}
	for len(m.bodyPaint) <= p.ID {
		m.bodyPaint = append(m.bodyPaint, nil)
	}
	list := append(m.bodyPaint[p.ID], bp)
	if len(list) > maxBodyPaint {
		list = list[1:]
	}
	m.bodyPaint[p.ID] = list

	// Back to the world, roughly (the bone's current pose isn't applied).
	back := mathx.RotateY(-p.Yaw)
	w := bp.at
	switch bp.bone {
	case boneHead:
		w = w.Add(mathx.Vec3{0, neckHeight, 0})
	case boneRightLeg:
		w = w.Add(mathx.Vec3{0.12, hipHeight, 0})
	case boneLeftLeg:
		w = w.Add(mathx.Vec3{-0.12, hipHeight, 0})
	}
	if w[1] > hipHeight {
		w[1] -= drop
	}
	return feet.Add(back.TransformPoint(w)), back.TransformPoint(bp.normal).Normalize()
}

// splash bursts paint off a hit at at (on the surface, facing normal), the
// ball having come along dir: off armour a big ricochet, into a popped
// player a smaller spray.
func (m *Arena) splash(at, normal, dir mathx.Vec3, colour [4]float32, armoured bool) {
	n, speed, bounce := 4, float32(3), float32(0.35)
	if armoured {
		n, speed, bounce = 9, float32(5.5), float32(0.8) // bouncing off: more, faster, more of it glancing away
	}
	reflect := dir.Sub(normal.Scale(2 * dir.Dot(normal)))
	for range n {
		if len(m.droplets) >= maxDroplets {
			m.droplets = m.droplets[1:]
		}
		jitter := mathx.Vec3{m.rng.Float32()*2 - 1, m.rng.Float32()*2 - 1, m.rng.Float32()*2 - 1}
		v := reflect.Scale(bounce).Add(normal.Scale(0.6)).Add(jitter.Scale(0.55)).Normalize()
		v = v.Scale(speed * (0.6 + 0.8*m.rng.Float32())).Add(mathx.Vec3{0, 1.2 + 1.5*m.rng.Float32(), 0})
		m.droplets = append(m.droplets, droplet{at: at.Add(normal.Scale(0.03)), vel: v,
			size: 0.012 + 0.014*m.rng.Float32(), colour: colour})
	}
}

// droplets are only stopped by the level and what's standing in it.
func paintPasses(b *physics.Body) bool {
	switch b.UserData.(type) {
	case *arena.Player, *arena.Debris, *arena.Grenade:
		return true
	}
	return false
}

// updateDroplets flies the paint, bouncing each once off what it meets and
// splatting it on the second.
func (m *Arena) updateDroplets(dt float32) {
	phys := m.sim().Phys
	kept := m.droplets[:0]
	for _, d := range m.droplets {
		d.age += dt
		d.vel[1] -= dropGravity * dt
		step := d.vel.Scale(dt)
		dist := step.Len()
		if d.age > dropLife || dist < 1e-6 {
			continue
		}
		hit, ok := phys.Raycast(d.at, step.Scale(1/dist), dist+d.size, paintPasses)
		if !ok {
			d.at = d.at.Add(step)
			kept = append(kept, d)
			continue
		}
		if !d.bounced && d.vel.Len() > 2.5 {
			// First contact: a little splat, and off again, much slower.
			m.addSplat(hit, d.size*2.4, d.colour)
			v := d.vel
			d.vel = v.Sub(hit.Normal.Scale(2 * v.Dot(hit.Normal))).Scale(0.35)
			d.at, d.bounced = hit.Point.Add(hit.Normal.Scale(0.01)), true
			kept = append(kept, d)
			continue
		}
		m.addSplat(hit, d.size*3.2, d.colour) // it lands
	}
	m.droplets = kept
}

// addSplat leaves a droplet's splat where a ray hit.
func (m *Arena) addSplat(hit physics.RayHit, size float32, colour [4]float32) {
	chunk, _ := hit.Body.UserData.(*arena.Chunk)
	if len(m.splats) == maxSplats {
		m.splats = m.splats[1:]
	}
	m.splatCount++
	m.splats = append(m.splats, newSplat(m.rng, hit.Point, hit.Normal, chunk, size, colour, m.splatCount))
}

// appendDroplets draws the paint in flight, stretched along its path.
func (m *Arena) appendDroplets(out []render.DrawCmd) []render.DrawCmd {
	for _, d := range m.droplets {
		speed := d.vel.Len()
		stretch := 1 + min(speed*0.05, 1.5)
		model := mathx.Translate(d.at[0], d.at[1], d.at[2])
		if speed > 0.1 {
			model = model.Mul(mathx.LookRotation(d.vel.Scale(1 / speed)).Mat4())
		}
		model = model.Mul(mathx.Scale(d.size, d.size, d.size*stretch))
		out = append(out, render.DrawCmd{Model: model, Color: d.colour, Mesh: m.as.ball})
	}
	return out
}

// appendBodyPaint draws the splats on p, each on its bone's frame as the
// character is posed now.
func (m *Arena) appendBodyPaint(out []render.DrawCmd, p *arena.Player, upper, head, rightLeg, leftLeg mathx.Mat4) []render.DrawCmd {
	if p.ID >= len(m.bodyPaint) {
		return out
	}
	for i, bp := range m.bodyPaint[p.ID] {
		frame := upper
		switch bp.bone {
		case boneHead:
			frame = head
		case boneRightLeg:
			frame = rightLeg
		case boneLeftLeg:
			frame = leftLeg
		}
		lift := 0.012 + 0.0015*float32(i%6) // off the suit, a hair apart so overlaps don't flicker
		f := frame.Mul(mathx.Translate(bp.at[0], bp.at[1], bp.at[2])).Mul(alignUp(bp.normal).Mat4()).
			Mul(mathx.RotateY(bp.spin)).Mul(mathx.Translate(0, lift, 0))
		out = append(out, render.DrawCmd{Model: f.Mul(mathx.Scale(bp.size, 1, bp.size*0.8)), Color: bp.colour,
			Flags: gfx.DrawFlat | gfx.DrawNoShadow, Mesh: m.sc.shadow})
		for _, d := range bp.drops {
			model := f.Mul(mathx.Translate(d[0], 0.0005, d[1])).Mul(mathx.Scale(d[2], 1, d[2]))
			out = append(out, render.DrawCmd{Model: model, Color: bp.colour, Flags: gfx.DrawFlat | gfx.DrawNoShadow, Mesh: m.sc.shadow})
		}
	}
	return out
}

// clearRevived takes the paint off a player who's got back up (a range
// dummy).
func (m *Arena) clearRevived() {
	players := m.sim().Players
	for len(m.wasDown) < len(players) {
		m.wasDown = append(m.wasDown, false)
	}
	for _, p := range players {
		if m.wasDown[p.ID] && !p.Dead && p.ID < len(m.bodyPaint) {
			m.bodyPaint[p.ID] = m.bodyPaint[p.ID][:0]
		}
		m.wasDown[p.ID] = p.Dead
	}
}
