package game

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// The guns are paintball markers: bright plastic bodies, a hopper of paint
// fed in from the side (so it never blocks the sights), and a gas tank.
// Shots fly as paintballs in the shooter's colour and burst into splats.
var (
	// paintColor is each side's paint: yours, and everyone else's.
	paintColor = [...][4]float32{mathx.SRGB(0.10, 0.78, 1.00, 1), mathx.SRGB(1.00, 0.45, 0.05, 1)}

	markerTeal   = mathx.SRGB(0.10, 0.62, 0.60, 1)
	markerOrange = mathx.SRGB(0.98, 0.50, 0.10, 1)
	markerPurple = mathx.SRGB(0.45, 0.22, 0.70, 1)
	markerLime   = mathx.SRGB(0.55, 0.85, 0.15, 1)
	markerWhite  = mathx.SRGB(0.88, 0.89, 0.90, 1)
	hopperYellow = mathx.SRGB(1.00, 0.85, 0.15, 0.8)
	gasSteel     = mathx.SRGB(0.55, 0.58, 0.62, 1)
	lensBlue     = mathx.SRGB(0.30, 0.75, 1.00, 1)
)

// team is which side a player paints for: you (0) or the others (1).
func team(p *arena.Player) int {
	if p.ID == local {
		return 0
	}
	return 1
}

// marker is a gun's model: its parts in weapon space (metres, barrel down
// -Z), where the paint leaves it, where its sight is, and how it's held.
type marker struct {
	parts  []gunPart
	muzzle mathx.Vec3
	sight  mathx.Vec3 // the point that lines up with the eye with the sights up
	size   float32    // drawn this much smaller than life, this close to the eye
	relief float32    // m from the eye to the sight with the sights up
	scope  bool       // with the sights up, the view goes through a scope instead

	// For the arms and the reload: where each hand holds it, the parts that
	// come away (a magazine, a drum) and the pump that slides.
	grip, fore mathx.Vec3 // the firing hand, and the supporting hand
	reload     reloadStyle
	mag        []gunPart  // the magazine (or drum), in its seat
	magHold    mathx.Vec3 // where the supporting hand takes it
	magDrop    mathx.Vec3 // which way it comes out
	charge     mathx.Vec3 // a handle the supporting hand works after a new magazine (zero: none)
	pump       []gunPart  // slides back and forth after each shot (a pump shotgun)
}

// reloadStyle is how a gun is reloaded: a magazine swapped, shells pushed
// in one at a time, or a drum swung out and replaced.
type reloadStyle int

const (
	reloadMag reloadStyle = iota
	reloadShells
	reloadDrum
)

// ring is a flat annulus in the XY plane, inner radius 1 and outer radius
// outer, facing +Z: the black round the sniper scope's view.
func ring(outer float32, segments int) geom.MeshData {
	var m geom.MeshData
	for i := 0; i <= segments; i++ {
		a := 2 * math.Pi * float64(i) / float64(segments)
		c, s := float32(math.Cos(a)), float32(math.Sin(a))
		n := mathx.Vec3{0, 0, 1}
		m.Vertices = append(m.Vertices, geom.Vertex{Position: mathx.Vec3{c, s, 0}, Normal: n},
			geom.Vertex{Position: mathx.Vec3{c * outer, s * outer, 0}, Normal: n})
	}
	for i := 0; i < segments; i++ {
		a := uint32(2 * i)
		m.Indices = append(m.Indices, a, a+1, a+3, a, a+3, a+2)
	}
	return m
}

// drawParts draws a model's parts under frame: boxes, or balls for the round ones.
func (m *Arena) drawParts(out []render.DrawCmd, frame mathx.Mat4, parts []gunPart) []render.DrawCmd {
	for _, part := range parts {
		c, h := part.centre, part.half
		model := frame.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2]))
		mesh := m.as.cube
		switch {
		case part.round:
			mesh = m.sc.ball
		case part.ring:
			mesh = m.as.thinRing
		}
		out = append(out, render.DrawCmd{Model: model, Color: part.color, Flags: part.flags, Mesh: mesh})
	}
	return out
}

// paintball is one shot's ball of paint in flight, from the muzzle to where
// the (instant) hit landed, drawn arriving at the gun's ball speed.
type paintball struct {
	from, to mathx.Vec3
	normal   mathx.Vec3   // of the surface it hits (zero: nothing, or a player)
	chunk    *arena.Chunk // the piece it hits, if any
	speed    float32
	size     float32
	colour   [4]float32
	age      float32
	player   bool // it hits a player
}

// splat is a burst of paint left on a surface: a blob with a couple of
// smaller drops round it.
type splat struct {
	at, normal mathx.Vec3
	chunk      *arena.Chunk // gone with it
	size       float32
	colour     [4]float32
	drops      [3]mathx.Vec3 // (x, z offset in the splat's plane, radius)
	spin       float32
	age        float32
	lift       float32 // tiny offset off the surface, different per splat so overlaps don't flicker
}

const (
	maxSplats  = 500
	splatLife  = 30 // s
	maxBalls   = 300
	splatLifts = 23 // distinct lifts before they repeat
)

// newSplat makes a splat of the given size and colour at a hit.
func newSplat(rng *rand.Rand, at, normal mathx.Vec3, chunk *arena.Chunk, size float32, colour [4]float32, n int) splat {
	s := splat{at: at, normal: normal, chunk: chunk, size: size * (0.8 + 0.4*rng.Float32()), colour: colour,
		spin: rng.Float32() * 2 * math.Pi, lift: 0.003 + 0.0004*float32(n%splatLifts)}
	for i := range s.drops {
		a := rng.Float64() * 2 * math.Pi
		r := s.size * (1 + 0.6*rng.Float32())
		s.drops[i] = mathx.Vec3{r * float32(math.Cos(a)), r * float32(math.Sin(a)), s.size * (0.15 + 0.25*rng.Float32())}
	}
	return s
}

// ballSize is how big a gun's paintballs are drawn.
func ballSize(k arena.WeaponKind) float32 {
	switch k {
	case arena.WeaponSniper:
		return 0.035
	case arena.WeaponShotgun:
		return 0.022
	}
	return 0.028
}

// splatSize is how big a gun's splats are.
func splatSize(k arena.WeaponKind) float32 {
	switch k {
	case arena.WeaponSniper:
		return 0.28
	case arena.WeaponShotgun:
		return 0.09
	case arena.WeaponPistol:
		return 0.17
	}
	return 0.13
}

// updatePaint flies the paintballs, bursting them into splats and drops
// where they land, and ages the splats (they go with the piece they're on).
func (m *Arena) updatePaint(dt float32) {
	heard := 0
	for i := range m.balls {
		b := &m.balls[i]
		b.age += dt
		if b.age*b.speed < b.to.Sub(b.from).Len() {
			continue
		}
		if b.normal != (mathx.Vec3{}) && (b.chunk == nil || b.chunk.Alive) {
			if len(m.splats) == maxSplats {
				m.splats = m.splats[1:]
			}
			m.splatCount++
			m.splats = append(m.splats, newSplat(m.rng, b.to, b.normal, b.chunk, b.size*5, b.colour, m.splatCount))
			if heard < 2 { // a burst of rifle fire doesn't need every splut
				m.playAt(m.sfx.splat, b.to, 0.7)
				heard++
			}
		}
		if b.normal != (mathx.Vec3{}) || b.player {
			for range 3 { // droplets spraying off it
				off := mathx.Vec3{m.rng.Float32() - 0.5, m.rng.Float32() - 0.2, m.rng.Float32() - 0.5}.Scale(b.size * 6)
				m.addBurst(burst{at: b.to.Add(b.normal.Scale(0.05)).Add(off), size: b.size * 0.8, life: 0.25, colour: b.colour})
			}
		}
	}
	m.balls = keep(m.balls, func(b paintball) bool { return b.age*b.speed < b.to.Sub(b.from).Len() })
	for i := range m.splats {
		m.splats[i].age += dt
	}
	m.splats = keep(m.splats, func(s splat) bool { return s.age < splatLife && (s.chunk == nil || s.chunk.Alive) })
}

// appendPaint draws the paintballs in flight and the splats.
func (m *Arena) appendPaint(out []render.DrawCmd) []render.DrawCmd {
	for _, b := range m.balls {
		path := b.to.Sub(b.from)
		at := b.from.Add(path.Normalize().Scale(b.age * b.speed))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, mathx.QuatIdentity(), b.size), Color: b.colour, Flags: gfx.DrawFlat,
			Mesh: m.sc.ball})
	}
	for _, s := range m.splats {
		fade := clampf((splatLife-s.age)/3, 0, 1)
		frame := mathx.Translate(s.at[0], s.at[1], s.at[2]).Mul(alignUp(s.normal).Mat4()).Mul(mathx.RotateY(s.spin)).
			Mul(mathx.Translate(0, s.lift, 0))
		col := withAlpha(s.colour, 0.95*fade)
		out = append(out, render.DrawCmd{Model: frame.Mul(mathx.Scale(s.size, 1, s.size*0.8)), Color: col, Flags: gfx.DrawFlat, Mesh: m.sc.shadow})
		for _, d := range s.drops {
			model := frame.Mul(mathx.Translate(d[0], 0.0004, d[1])).Mul(mathx.Scale(d[2], 1, d[2]))
			out = append(out, render.DrawCmd{Model: model, Color: col, Flags: gfx.DrawFlat, Mesh: m.sc.shadow})
		}
	}
	return out
}

// appendReticle draws the crosshair a little way in front of the camera:
// four ticks spread as wide as the gun's current cone, so bloom shows. With
// the sights up it's a dot; through a scope, the scope's ring and lines.
func (m *Arena) appendReticle(out []render.DrawCmd, fovY float32) []render.DrawCmd {
	me := m.me()
	if me.Dead {
		return out
	}
	cam := m.camWorld()
	const d = 0.1 // m in front of the eye
	col := withAlpha(uiWhite, 0.9)
	if m.hitMark > 0 {
		col = uiAccent
		if m.headMark {
			col = hurtColor
		}
	}
	tick := func(x, y, hx, hy float32, c [4]float32) {
		model := cam.Mul(mathx.Translate(x, y, -d)).Mul(mathx.Scale(hx, hy, 0.00001))
		out = append(out, render.DrawCmd{Model: model, Color: c, Flags: gfx.DrawUnlit, Mesh: m.as.cube})
	}
	px := d * float32(math.Tan(float64(fovY)/2)) / 360 // about a pixel at 720 lines
	mk, _ := m.markerFor(me.Current)
	halfH := d * float32(math.Tan(float64(fovY)/2)) // half the screen's height, at d
	disc := func(radius float32, col [4]float32) {
		z := float32(d) * 1.04 // behind every ring
		model := cam.Mul(mathx.Translate(0, 0, -z)).Mul(mathx.RotateX(math.Pi / 2)).Mul(mathx.Scale(radius*z/d, 1, radius*z/d))
		out = append(out, render.DrawCmd{Model: model, Color: col, Flags: gfx.DrawUnlit, Mesh: m.sc.shadow})
	}
	// Layers of the overlay each sit a hair nearer than the one before, so
	// where they overlap the later one wins cleanly.
	layer := float32(0)
	ringAt := func(mesh render.Mesh, radius float32, col [4]float32) {
		layer++
		z := d * (1 + 0.004*(8-layer))
		scale := radius * z / d
		model := cam.Mul(mathx.Translate(0, 0, -z)).Mul(mathx.Scale(scale, scale, 1))
		out = append(out, render.DrawCmd{Model: model, Color: col, Flags: gfx.DrawUnlit, Mesh: mesh})
	}
	if mk != nil && mk.scope && me.ADS > 0.85 {
		// The scope: a tinted lens with a cyan rim and a soft dark edge,
		// in black; duplex posts closing on fine crosshairs with mil-dots
		// and range ticks; and an illuminated red centre.
		r := halfH * 0.92
		black := [4]float32{0, 0, 0, 1}
		disc(r, withAlpha(lensBlue, 0.06))
		ringAt(m.as.thinRing, r*0.9, [4]float32{0, 0, 0, 0.35})
		ringAt(m.as.thinRing, r*0.975, withAlpha(lensBlue, 0.45))
		ringAt(m.as.ring, r, black)
		post, fine := px*3.5, px*0.7
		for _, sgn := range []float32{-1, 1} {
			tick(sgn*r*0.68, 0, r*0.32, post, black) // the posts
			tick(0, sgn*r*0.68, post, r*0.32, black)
			for i := 1; i <= 4; i++ { // mil-dots
				at := sgn * r * 0.08 * float32(i)
				tick(at, 0, px*1.8, px*1.8, black)
				tick(0, at, px*1.8, px*1.8, black)
			}
		}
		tick(0, 0, r*0.36, fine, black) // the fine lines
		tick(0, 0, fine, r*0.36, black)
		for i := 1; i <= 3; i++ { // range ticks on the lower line
			y := -r * (0.12 + 0.07*float32(i))
			tick(0, y, px*(9-2*float32(i)), fine, black)
		}
		glow := 0.8 + 0.2*float32(math.Sin(float64(m.elapsed)*3))
		ringAt(m.as.thinRing, px*5, withAlpha(sightRed, 0.9*glow))
		tick(0, 0, px*1.1, px*1.1, withAlpha(sightRed, glow))
		return out
	}
	if me.ADS > 0.1 {
		// Sights up: the gun's own sight is the reticle. The edges of the
		// view darken as it comes up.
		for i := range 3 { // three soft steps, not one hard edge
			ringAt(m.as.ring, halfH*(0.95+0.14*float32(i)), [4]float32{0, 0, 0, 0.13 * smooth(me.ADS)})
		}
		if me.ADS > 0.6 {
			return out
		}
		col = withAlpha(col, col[3]*(1-me.ADS/0.6))
	}
	gap := d*float32(math.Tan(float64(me.Spread()))) + px*5
	long, thin := px*9, px*1.6
	tick(gap+long, 0, long, thin, col)
	tick(-gap-long, 0, long, thin, col)
	tick(0, gap+long, thin, long, col)
	tick(0, -gap-long, thin, long, col)
	return out
}

// markerFor is the model of weapon k, if it's a gun.
func (m *Arena) markerFor(k arena.WeaponKind) (*marker, bool) {
	if int(k) < len(markers) && markers[k].parts != nil {
		return &markers[k], true
	}
	return nil, false
}
