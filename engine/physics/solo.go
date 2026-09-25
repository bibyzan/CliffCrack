package physics

import "math"

// StepBody advances one dynamic body by dt against the static and
// kinematic bodies only, leaving everything else as it is: other dynamic
// bodies don't move and aren't collided with, and no impacts are reported.
// It's for prediction: replaying a player's own movement on a network
// client between the authoritative updates.
func (w *World) StepBody(b *Body, dt float32) {
	if b.Kind != Dynamic || dt <= 0 {
		return
	}
	n := max(1, int(math.Ceil(float64(dt/w.FixedStep))))
	h := dt / float32(n)
	bp := &w.bp
	if bp.dirty || bp.cells == nil {
		bp.rebuild(w.bodies)
	}
	lin := float32(math.Exp(-float64(w.LinearDamping * h)))
	ang := float32(math.Exp(-float64(w.AngularDamping * h)))
	for range n {
		b.prevPosition, b.prevRotation = b.Position, b.Rotation
		b.Velocity = b.Velocity.Add(w.Gravity.Scale(h))

		var contacts []contact
		test := func(o *Body) {
			if o == b || o.Kind == Dynamic || b.Ignore == o || o.Ignore == b {
				return
			}
			if c, ok := collide(b, o); ok {
				contacts = append(contacts, c)
			}
		}
		for _, o := range bp.always {
			test(o)
		}
		lo, hi, _ := aabb(b)
		c0, c1 := cellRange(lo, hi)
		bp.stamp++
		for x := c0[0]; x <= c1[0]; x++ {
			for y := c0[1]; y <= c1[1]; y++ {
				for z := c0[2]; z <= c1[2]; z++ {
					for _, o := range bp.cells[cellKey{x, y, z}] {
						if o.stamp != bp.stamp {
							o.stamp = bp.stamp
							test(o)
						}
					}
				}
			}
		}

		b.Grounded = false
		for i := range contacts {
			c := &contacts[i]
			const supportY = 0.5
			if (c.b == b && c.normal[1] > supportY) || (c.a == b && c.normal[1] < -supportY) {
				b.Grounded = true
			}
			if vn := relativeVelocity(c).Dot(c.normal); vn < -bounceThreshold {
				c.bias = -max(c.a.Restitution, c.b.Restitution) * vn
			}
		}
		for range w.Iterations {
			for i := range contacts {
				solve(&contacts[i])
			}
		}
		for i := range contacts {
			pushApart(&contacts[i])
		}
		b.Position = b.Position.Add(b.Velocity.Scale(h))
		b.Velocity = b.Velocity.Scale(lin)
		b.AngularVelocity = b.AngularVelocity.Scale(ang)
		b.Rotation = integrateRotation(b.Rotation, b.AngularVelocity, h)
	}
}
