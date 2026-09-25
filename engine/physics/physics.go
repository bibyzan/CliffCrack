// Package physics is a small rigid-body simulation in pure Go: dynamic spheres
// colliding with each other, with static or kinematic spheres and oriented
// boxes, and with static heightfield terrain. It runs at a fixed timestep with sequential impulses (restitution,
// Coulomb friction, rolling via angular velocity).
//
// Deliberately small: no dynamic boxes, no broadphase (O(n^2) pairs, fine for
// a few hundred bodies), no sleeping.
package physics

import (
	"errors"
	"math"

	"CliffCrack/engine/mathx"
)

type Kind int

const (
	Dynamic   Kind = iota // moved by the simulation
	Static                // never moves
	Kinematic             // moved by game code; pushes dynamic bodies, isn't pushed back
)

type Shape int

const (
	Sphere Shape = iota
	Box
	// Heightfield is static terrain given by a height function over world X/Z
	// (see Body.Height). Position and Rotation are ignored.
	Heightfield
)

// HeightFunc returns the terrain height at world (x, z).
type HeightFunc func(x, z float32) float32

// Body is a rigid body. Set Position/Rotation/Velocity directly; for kinematic
// bodies, also set Velocity so collisions feel their motion.
type Body struct {
	Kind        Kind
	Shape       Shape
	Radius      float32    // Sphere
	HalfExtents mathx.Vec3 // Box
	Height      HeightFunc // Heightfield

	Position        mathx.Vec3
	Rotation        mathx.Quat
	Velocity        mathx.Vec3
	AngularVelocity mathx.Vec3

	Mass        float32 // Dynamic only
	Restitution float32 // bounciness 0..1
	Friction    float32

	UserData any // e.g. the scene entity this body drives

	// Grounded is set by the world each step for dynamic bodies: true while
	// something below is supporting the body (a contact normal pointing up).
	Grounded bool

	// FixedRotation makes a dynamic body ignore torque: contacts and friction
	// only change its velocity, never its spin. Use it for characters, so
	// friction holds them still on slopes instead of rolling them away.
	FixedRotation bool

	invMass, invInertia float32

	// State before the latest step, for Interpolated.
	prevPosition mathx.Vec3
	prevRotation mathx.Quat
}

// Interpolated is where to draw the body: between its state before and after
// the latest step, alpha (see World.Alpha) of the way along. The simulation
// runs at a fixed rate that doesn't match the display, so drawing the raw
// state makes fast bodies stutter (some frames run no step, others two).
func (b *Body) Interpolated(alpha float32) (mathx.Vec3, mathx.Quat) {
	p := b.prevPosition.Add(b.Position.Sub(b.prevPosition).Scale(alpha))
	return p, mathx.Nlerp(b.prevRotation, b.Rotation, alpha)
}

// Teleported tells the body it was moved by hand, so Interpolated doesn't
// draw it sliding from where it was.
func (b *Body) Teleported() {
	b.prevPosition, b.prevRotation = b.Position, b.Rotation
}

// NewSphere returns a dynamic sphere of the given radius and mass.
func NewSphere(radius, mass float32) *Body {
	return &Body{Kind: Dynamic, Shape: Sphere, Radius: radius, Mass: mass,
		Rotation: mathx.QuatIdentity(), Restitution: 0.45, Friction: 0.5}
}

// NewBox returns a static or kinematic box.
func NewBox(halfExtents mathx.Vec3, kind Kind) *Body {
	return &Body{Kind: kind, Shape: Box, HalfExtents: halfExtents,
		Rotation: mathx.QuatIdentity(), Restitution: 0.3, Friction: 0.6}
}

// Impact reports a collision that started this step with a noticeable speed.
type Impact struct {
	A, B  *Body
	Point mathx.Vec3
	Speed float32 // closing speed along the normal, m/s
}

type World struct {
	Gravity    mathx.Vec3
	FixedStep  float32 // seconds per simulation step
	Iterations int     // solver passes per step
	MaxSteps   int     // per Update, to avoid a spiral of death after a hitch

	// Velocity decay per second for dynamic bodies. Angular damping stands in
	// for rolling resistance.
	LinearDamping, AngularDamping float32

	bodies  []*Body
	acc     float32
	impacts []Impact
	touched map[[2]*Body]bool // pairs in contact during the previous step
}

func NewWorld() *World {
	return &World{
		Gravity:    mathx.Vec3{0, -9.81, 0},
		FixedStep:  1.0 / 120,
		Iterations: 8,
		MaxSteps:   8,
		touched:    map[[2]*Body]bool{},

		LinearDamping:  0.02,
		AngularDamping: 1.0,
	}
}

// NewHeightfield returns static terrain following height.
func NewHeightfield(height HeightFunc) *Body {
	return &Body{Kind: Static, Shape: Heightfield, Height: height,
		Rotation: mathx.QuatIdentity(), Restitution: 0.1, Friction: 0.6}
}

// Add inserts a body. Dynamic bodies must be spheres with positive mass.
func (w *World) Add(b *Body) error {
	switch {
	case b.Kind == Dynamic && b.Shape != Sphere:
		return errors.New("physics: dynamic bodies must be spheres")
	case b.Shape == Heightfield && (b.Kind != Static || b.Height == nil):
		return errors.New("physics: a heightfield must be static and have a height function")
	case b.Kind == Dynamic && (b.Mass <= 0 || b.Radius <= 0):
		return errors.New("physics: dynamic sphere needs positive mass and radius")
	}
	if b.Rotation == (mathx.Quat{}) {
		b.Rotation = mathx.QuatIdentity()
	}
	b.invMass, b.invInertia = 0, 0
	if b.Kind == Dynamic {
		b.invMass = 1 / b.Mass
		if !b.FixedRotation {
			b.invInertia = 1 / (0.4 * b.Mass * b.Radius * b.Radius) // solid sphere: 2/5 m r^2
		}
	}
	b.Teleported()
	w.bodies = append(w.bodies, b)
	return nil
}

func (w *World) Remove(b *Body) {
	for i, x := range w.bodies {
		if x == b {
			w.bodies = append(w.bodies[:i], w.bodies[i+1:]...)
			break
		}
	}
	for pair := range w.touched {
		if pair[0] == b || pair[1] == b {
			delete(w.touched, pair)
		}
	}
}

func (w *World) Bodies() []*Body { return w.bodies }

// Impacts lists the collisions that began during the last Update.
func (w *World) Impacts() []Impact { return w.impacts }

// Alpha is how far the unsimulated leftover time reaches into the next step
// (0..1): pass it to Body.Interpolated when drawing.
func (w *World) Alpha() float32 { return min(w.acc/w.FixedStep, 1) }

// Update advances the simulation by dt using as many fixed steps as fit, and
// returns how many ran. Leftover time carries over to the next call.
func (w *World) Update(dt float32) int {
	w.impacts = w.impacts[:0]
	w.acc += dt
	steps := 0
	for w.acc >= w.FixedStep && steps < w.MaxSteps {
		w.step(w.FixedStep)
		w.acc -= w.FixedStep
		steps++
	}
	if steps == w.MaxSteps {
		w.acc = 0 // drop the backlog rather than falling further behind
	}
	return steps
}

type contact struct {
	a, b   *Body
	normal mathx.Vec3 // from a to b
	point  mathx.Vec3
	depth  float32
	bias   float32 // target separating speed from restitution
	jn, jt float32 // accumulated impulses
}

const (
	bounceThreshold = 0.5   // m/s; slower hits don't bounce (stops jitter at rest)
	slop            = 0.002 // allowed penetration before position correction
	correction      = 0.6   // fraction of penetration removed per step
	impactSpeed     = 0.8   // m/s; slower new contacts aren't reported
)

func (w *World) step(h float32) {
	for _, b := range w.bodies {
		b.prevPosition, b.prevRotation = b.Position, b.Rotation
	}
	for _, b := range w.bodies {
		if b.Kind == Dynamic {
			b.Velocity = b.Velocity.Add(w.Gravity.Scale(h))
		}
	}

	contacts := w.detect()
	for _, b := range w.bodies {
		b.Grounded = false
	}
	touched := make(map[[2]*Body]bool, len(contacts))
	for i := range contacts {
		c := &contacts[i]
		const supportY = 0.5 // contact normals steeper than ~60 degrees don't count as ground
		if c.normal[1] > supportY {
			c.b.Grounded = c.b.Grounded || c.b.Kind == Dynamic // a is below b
		} else if c.normal[1] < -supportY {
			c.a.Grounded = c.a.Grounded || c.a.Kind == Dynamic
		}
		vn := relativeVelocity(c).Dot(c.normal)
		if vn < -bounceThreshold {
			c.bias = -max(c.a.Restitution, c.b.Restitution) * vn
		}
		pair := [2]*Body{c.a, c.b}
		touched[pair] = true
		if !w.touched[pair] && vn < -impactSpeed {
			w.impacts = append(w.impacts, Impact{A: c.a, B: c.b, Point: c.point, Speed: -vn})
		}
	}
	w.touched = touched

	for it := 0; it < w.Iterations; it++ {
		for i := range contacts {
			solve(&contacts[i])
		}
	}
	for i := range contacts {
		pushApart(&contacts[i])
	}

	lin := float32(math.Exp(-float64(w.LinearDamping * h)))
	ang := float32(math.Exp(-float64(w.AngularDamping * h)))
	for _, b := range w.bodies {
		if b.Kind == Static {
			continue
		}
		b.Position = b.Position.Add(b.Velocity.Scale(h))
		if b.Kind == Dynamic {
			b.Velocity = b.Velocity.Scale(lin)
			b.AngularVelocity = b.AngularVelocity.Scale(ang)
		}
		b.Rotation = integrateRotation(b.Rotation, b.AngularVelocity, h)
	}
}

// integrateRotation applies angular velocity w for time h: q += h/2 * (w,0) q.
func integrateRotation(q mathx.Quat, w mathx.Vec3, h float32) mathx.Quat {
	if w == (mathx.Vec3{}) {
		return q
	}
	dq := mathx.Quat{X: w[0], Y: w[1], Z: w[2]}.Mul(q)
	s := h / 2
	return mathx.Quat{X: q.X + dq.X*s, Y: q.Y + dq.Y*s, Z: q.Z + dq.Z*s, W: q.W + dq.W*s}.Normalize()
}

func (w *World) detect() []contact {
	var out []contact
	for i, a := range w.bodies {
		for _, b := range w.bodies[i+1:] {
			if a.Kind != Dynamic && b.Kind != Dynamic {
				continue
			}
			if c, ok := collide(a, b); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

func collide(a, b *Body) (contact, bool) {
	switch {
	case a.Shape == Sphere && b.Shape == Sphere:
		return sphereSphere(a, b)
	case a.Shape == Box && b.Shape == Sphere:
		return boxSphere(a, b)
	case a.Shape == Sphere && b.Shape == Box:
		c, ok := boxSphere(b, a)
		return c, ok
	case a.Shape == Heightfield && b.Shape == Sphere:
		return terrainSphere(a, b)
	case a.Shape == Sphere && b.Shape == Heightfield:
		return terrainSphere(b, a)
	}
	return contact{}, false // box-box: only static/kinematic boxes exist
}

func sphereSphere(a, b *Body) (contact, bool) {
	d := b.Position.Sub(a.Position)
	dist := d.Len()
	r := a.Radius + b.Radius
	if dist >= r {
		return contact{}, false
	}
	n := mathx.Vec3{0, 1, 0} // coincident centres: pick any axis
	if dist > 1e-6 {
		n = d.Scale(1 / dist)
	}
	return contact{a: a, b: b, normal: n, depth: r - dist, point: a.Position.Add(n.Scale(a.Radius))}, true
}

// boxSphere collides an oriented box (a) with a sphere (b); the normal points
// from the box to the sphere.
func boxSphere(box, s *Body) (contact, bool) {
	inv := box.Rotation.Conjugate()
	local := inv.Rotate(s.Position.Sub(box.Position))
	he := box.HalfExtents
	closest := mathx.Vec3{
		max(-he[0], min(he[0], local[0])),
		max(-he[1], min(he[1], local[1])),
		max(-he[2], min(he[2], local[2])),
	}

	var nLocal mathx.Vec3
	var depth float32
	if closest == local {
		// Centre inside the box: push out through the nearest face.
		best, bestAxis, bestSign := float32(math.MaxFloat32), 0, float32(1)
		for axis := 0; axis < 3; axis++ {
			for _, sign := range [2]float32{1, -1} {
				if gap := he[axis] - sign*local[axis]; gap < best {
					best, bestAxis, bestSign = gap, axis, sign
				}
			}
		}
		nLocal[bestAxis] = bestSign
		depth = best + s.Radius
		closest[bestAxis] = bestSign * he[bestAxis] // contact point on that face
	} else {
		d := local.Sub(closest)
		dist := d.Len()
		if dist >= s.Radius {
			return contact{}, false
		}
		nLocal = d.Scale(1 / dist)
		depth = s.Radius - dist
	}
	n := box.Rotation.Rotate(nLocal)
	point := box.Position.Add(box.Rotation.Rotate(closest))
	return contact{a: box, b: s, normal: n, depth: depth, point: point}, true
}

// TerrainNormal is the unit surface normal of a height function at (x, z),
// from central differences over +-eps.
func TerrainNormal(h HeightFunc, x, z, eps float32) mathx.Vec3 {
	dx := h(x+eps, z) - h(x-eps, z)
	dz := h(x, z+eps) - h(x, z-eps)
	return mathx.Vec3{-dx, 2 * eps, -dz}.Normalize()
}

// terrainSphere collides a heightfield (a) with a sphere (b).
//
// Where the terrain is smooth on the scale of the sphere, it is treated as the
// tangent plane under the sphere's centre: exact for planes and free of
// jitter on slopes. Near sharp features (a cliff lip, a crack's wall) that
// plane is meaningless, so the contact is taken to the nearest of a ring of
// surface samples instead, which lets balls roll off edges and hit walls. A
// centre that has sunk below the surface is pushed back up through it.
func terrainSphere(t, s *Body) (contact, bool) {
	p, r := s.Position, s.Radius
	under := mathx.Vec3{p[0], t.Height(p[0], p[2]), p[2]}
	n := TerrainNormal(t.Height, p[0], p[2], max(0.25, r*0.5))
	dist := p.Sub(under).Dot(n) // signed distance from the tangent plane
	if p[1] < under[1] {
		return contact{a: t, b: s, normal: n, depth: r - dist, point: under}, true
	}
	if p[1]-under[1] >= r/max(n[1], 0.2)+r { // well clear of anything below
		return contact{}, false
	}

	// Sample a ring at the sphere's footprint; if any sample strays from the
	// tangent plane, the terrain has an edge here.
	const samples = 12
	var ring [samples]mathx.Vec3
	smooth := true
	for i := range ring {
		a := 2 * math.Pi * float64(i) / samples
		x, z := p[0]+r*float32(math.Cos(a)), p[2]+r*float32(math.Sin(a))
		ring[i] = mathx.Vec3{x, t.Height(x, z), z}
		// Height of the tangent plane at (x, z).
		plane := under[1] - (n[0]*(x-p[0])+n[2]*(z-p[2]))/max(n[1], 1e-3)
		if abs32(ring[i][1]-plane) > 0.1*r {
			smooth = false
		}
	}
	if smooth {
		if dist >= r {
			return contact{}, false
		}
		return contact{a: t, b: s, normal: n, depth: r - dist, point: p.Sub(n.Scale(dist))}, true
	}
	nearest, best := under, p.Sub(under).Len()
	for _, q := range ring {
		if d := p.Sub(q).Len(); d < best {
			nearest, best = q, d
		}
	}
	if best >= r || best < 1e-6 {
		return contact{}, false
	}
	n = p.Sub(nearest).Scale(1 / best)
	return contact{a: t, b: s, normal: n, depth: r - best, point: nearest}, true
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// velocityAt is the velocity of body b's material at world point p.
func velocityAt(b *Body, p mathx.Vec3) mathx.Vec3 {
	return b.Velocity.Add(b.AngularVelocity.Cross(p.Sub(b.Position)))
}

func relativeVelocity(c *contact) mathx.Vec3 {
	return velocityAt(c.b, c.point).Sub(velocityAt(c.a, c.point))
}

// effectiveMass is 1 / (the inverse mass the impulse along dir sees at the contact).
func effectiveMass(c *contact, dir mathx.Vec3) float32 {
	ra := c.point.Sub(c.a.Position).Cross(dir)
	rb := c.point.Sub(c.b.Position).Cross(dir)
	k := c.a.invMass + c.b.invMass + c.a.invInertia*ra.Dot(ra) + c.b.invInertia*rb.Dot(rb)
	if k == 0 {
		return 0
	}
	return 1 / k
}

func applyImpulse(c *contact, p mathx.Vec3) {
	a, b := c.a, c.b
	a.Velocity = a.Velocity.Sub(p.Scale(a.invMass))
	a.AngularVelocity = a.AngularVelocity.Sub(c.point.Sub(a.Position).Cross(p).Scale(a.invInertia))
	b.Velocity = b.Velocity.Add(p.Scale(b.invMass))
	b.AngularVelocity = b.AngularVelocity.Add(c.point.Sub(b.Position).Cross(p).Scale(b.invInertia))
}

// solve runs one sequential-impulse pass for a contact: non-penetration with
// restitution, then friction bounded by the normal impulse.
func solve(c *contact) {
	n := c.normal
	vn := relativeVelocity(c).Dot(n)
	jn := (c.bias - vn) * effectiveMass(c, n)
	newJn := max(c.jn+jn, 0) // contacts push, never pull
	applyImpulse(c, n.Scale(newJn-c.jn))
	c.jn = newJn

	v := relativeVelocity(c)
	tangent := v.Sub(n.Scale(v.Dot(n)))
	if l := tangent.Len(); l > 1e-6 {
		tangent = tangent.Scale(1 / l)
		limit := float32(math.Sqrt(float64(c.a.Friction*c.b.Friction))) * c.jn
		jt := -v.Dot(tangent) * effectiveMass(c, tangent)
		newJt := max(-limit, min(limit, c.jt+jt))
		applyImpulse(c, tangent.Scale(newJt-c.jt))
		c.jt = newJt
	}
}

// pushApart removes most of the remaining penetration by moving the bodies.
func pushApart(c *contact) {
	total := c.a.invMass + c.b.invMass
	if total == 0 {
		return
	}
	move := max(c.depth-slop, 0) * correction / total
	c.a.Position = c.a.Position.Sub(c.normal.Scale(move * c.a.invMass))
	c.b.Position = c.b.Position.Add(c.normal.Scale(move * c.b.invMass))
}
