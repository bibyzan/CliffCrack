// Package physics is a small rigid-body simulation in pure Go: dynamic spheres
// colliding with each other and with static or kinematic spheres and oriented
// boxes. It runs at a fixed timestep with sequential impulses (restitution,
// Coulomb friction, rolling via angular velocity).
//
// Deliberately small: no dynamic boxes, no broadphase (O(n^2) pairs, fine for
// a few hundred bodies), no sleeping.
package physics

import (
	"errors"
	"math"

	"vkgame/engine/mathx"
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
)

// Body is a rigid body. Set Position/Rotation/Velocity directly; for kinematic
// bodies, also set Velocity so collisions feel their motion.
type Body struct {
	Kind        Kind
	Shape       Shape
	Radius      float32    // Sphere
	HalfExtents mathx.Vec3 // Box

	Position        mathx.Vec3
	Rotation        mathx.Quat
	Velocity        mathx.Vec3
	AngularVelocity mathx.Vec3

	Mass        float32 // Dynamic only
	Restitution float32 // bounciness 0..1
	Friction    float32

	UserData any // e.g. the scene entity this body drives

	invMass, invInertia float32
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
	}
}

// Add inserts a body. Dynamic bodies must be spheres with positive mass.
func (w *World) Add(b *Body) error {
	switch {
	case b.Kind == Dynamic && b.Shape != Sphere:
		return errors.New("physics: dynamic bodies must be spheres")
	case b.Kind == Dynamic && (b.Mass <= 0 || b.Radius <= 0):
		return errors.New("physics: dynamic sphere needs positive mass and radius")
	}
	if b.Rotation == (mathx.Quat{}) {
		b.Rotation = mathx.QuatIdentity()
	}
	b.invMass, b.invInertia = 0, 0
	if b.Kind == Dynamic {
		b.invMass = 1 / b.Mass
		b.invInertia = 1 / (0.4 * b.Mass * b.Radius * b.Radius) // solid sphere: 2/5 m r^2
	}
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
	linearDamping   = 0.02  // per second
	angularDamping  = 1.0   // per second: stands in for rolling resistance
	impactSpeed     = 0.8   // m/s; slower new contacts aren't reported
)

func (w *World) step(h float32) {
	for _, b := range w.bodies {
		if b.Kind == Dynamic {
			b.Velocity = b.Velocity.Add(w.Gravity.Scale(h))
		}
	}

	contacts := w.detect()
	touched := make(map[[2]*Body]bool, len(contacts))
	for i := range contacts {
		c := &contacts[i]
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

	lin := float32(math.Exp(-linearDamping * float64(h)))
	ang := float32(math.Exp(-angularDamping * float64(h)))
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
