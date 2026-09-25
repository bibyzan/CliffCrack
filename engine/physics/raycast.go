package physics

import (
	"math"

	"CliffCrack/engine/mathx"
)

// RayHit is where a ray first touches a body.
type RayHit struct {
	Body     *Body
	Point    mathx.Vec3
	Normal   mathx.Vec3 // surface normal at Point, facing back along the ray
	Distance float32
}

// Raycast finds the nearest body the ray from origin along dir touches within
// maxDist. dir needn't be unit length. skip, if non-nil, excludes bodies (the
// shooter, for instance). A ray starting inside a sphere or box hits it at
// distance 0.
func (w *World) Raycast(origin, dir mathx.Vec3, maxDist float32, skip func(*Body) bool) (RayHit, bool) {
	d := dir.Normalize()
	if d == (mathx.Vec3{}) || maxDist <= 0 {
		return RayHit{}, false
	}
	best := RayHit{Distance: maxDist}
	found := false
	test := func(b *Body) {
		if skip != nil && skip(b) {
			return
		}
		var t float32
		var n mathx.Vec3
		var ok bool
		switch b.Shape {
		case Sphere:
			t, n, ok = raySphere(origin, d, b.Position, b.Radius)
		case Box:
			t, n, ok = rayBox(origin, d, b)
		case Heightfield:
			t, n, ok = rayHeightfield(origin, d, b.Height, best.Distance)
		}
		if ok && t <= best.Distance {
			best = RayHit{Body: b, Point: origin.Add(d.Scale(t)), Normal: n, Distance: t}
			found = true
		}
	}
	if maxDist > shortRay {
		for _, b := range w.bodies {
			test(b)
		}
		return best, found
	}
	// A short ray only needs the statics in the grid cells it passes
	// through, plus everything that isn't in the grid.
	bp := &w.bp
	if bp.dirty || bp.cells == nil {
		bp.rebuild(w.bodies)
	}
	end := origin.Add(d.Scale(maxDist))
	lo := mathx.Vec3{min(origin[0], end[0]), min(origin[1], end[1]), min(origin[2], end[2])}
	hi := mathx.Vec3{max(origin[0], end[0]), max(origin[1], end[1]), max(origin[2], end[2])}
	c0, c1 := cellRange(lo, hi)
	bp.stamp++
	for x := c0[0]; x <= c1[0]; x++ {
		for y := c0[1]; y <= c1[1]; y++ {
			for z := c0[2]; z <= c1[2]; z++ {
				for _, b := range bp.cells[cellKey{x, y, z}] {
					if b.stamp != bp.stamp {
						b.stamp = bp.stamp
						test(b)
					}
				}
			}
		}
	}
	for _, b := range bp.always {
		test(b)
	}
	for _, b := range w.bodies {
		if b.Kind == Dynamic {
			test(b)
		}
	}
	return best, found
}

// shortRay is the longest ray (m) cast through the broadphase grid rather
// than against every body.
const shortRay = 3 * gridCell

func raySphere(o, d, c mathx.Vec3, r float32) (float32, mathx.Vec3, bool) {
	oc := o.Sub(c)
	b := oc.Dot(d)
	cc := oc.Dot(oc) - r*r
	if cc <= 0 { // inside
		return 0, d.Scale(-1), true
	}
	if b > 0 {
		return 0, mathx.Vec3{}, false // pointing away
	}
	disc := b*b - cc
	if disc < 0 {
		return 0, mathx.Vec3{}, false
	}
	t := -b - float32(math.Sqrt(float64(disc)))
	return t, o.Add(d.Scale(t)).Sub(c).Scale(1 / r), true
}

// rayBox is the slab test in the box's local frame.
func rayBox(o, d mathx.Vec3, b *Body) (float32, mathx.Vec3, bool) {
	r := b.Rotation
	aligned := r.X == 0 && r.Y == 0 && r.Z == 0 // most statics: skip the rotations
	lo, ld := o.Sub(b.Position), d
	if !aligned {
		inv := r.Conjugate()
		lo, ld = inv.Rotate(lo), inv.Rotate(d)
	}
	tNear, tFar := float32(math.Inf(-1)), float32(math.Inf(1))
	axis, sign := -1, float32(0)
	for i := 0; i < 3; i++ {
		he := b.HalfExtents[i]
		if abs32(ld[i]) < 1e-8 {
			if lo[i] < -he || lo[i] > he {
				return 0, mathx.Vec3{}, false // parallel and outside this slab
			}
			continue
		}
		t1, t2 := (-he-lo[i])/ld[i], (he-lo[i])/ld[i]
		s := float32(-1) // entering through the -face
		if t1 > t2 {
			t1, t2, s = t2, t1, 1
		}
		if t1 > tNear {
			tNear, axis, sign = t1, i, s
		}
		tFar = min(tFar, t2)
		if tNear > tFar || tFar < 0 {
			return 0, mathx.Vec3{}, false
		}
	}
	if tNear < 0 || axis < 0 { // inside the box
		return 0, d.Scale(-1), true
	}
	var n mathx.Vec3
	n[axis] = sign
	if aligned {
		return tNear, n, true
	}
	return tNear, b.Rotation.Rotate(n), true
}

// rayHeightfield marches along the ray and refines the first crossing below
// the terrain by bisection.
func rayHeightfield(o, d mathx.Vec3, h HeightFunc, maxDist float32) (float32, mathx.Vec3, bool) {
	const step = 0.25
	below := func(t float32) bool {
		p := o.Add(d.Scale(t))
		return p[1] <= h(p[0], p[2])
	}
	if below(0) {
		return 0, mathx.Vec3{0, 1, 0}, true
	}
	prev := float32(0)
	for t := float32(step); t <= maxDist+step; t += step {
		t = min(t, maxDist)
		if below(t) {
			lo, hi := prev, t
			for range 16 {
				mid := (lo + hi) / 2
				if below(mid) {
					hi = mid
				} else {
					lo = mid
				}
			}
			p := o.Add(d.Scale(hi))
			return hi, TerrainNormal(h, p[0], p[2], 0.1), true
		}
		if t == maxDist {
			break
		}
		prev = t
	}
	return 0, mathx.Vec3{}, false
}
