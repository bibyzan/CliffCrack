package physics

import (
	"math"

	"CliffCrack/engine/mathx"
)

// The broadphase keeps static spheres and boxes in a uniform grid, so a
// dynamic body only tests the statics near it instead of every body in the
// world. That matters once a level has thousands of pieces (destructible
// buildings). Kinematic bodies move every step and heightfields cover
// everything, so they sit in a short list tested against every dynamic body.
//
// The grid is rebuilt lazily after static bodies are added or removed. Static
// bodies must not be moved after being added; if one is, call StaticsChanged.

const (
	gridCell      = 4.0  // metres
	maxBodyCells  = 4096 // a static spanning more cells than this goes in the always-test list
	gridCellFloat = float32(gridCell)
)

type cellKey [3]int32

type broadphase struct {
	cells  map[cellKey][]*Body
	always []*Body // kinematic bodies, heightfields and huge statics
	dirty  bool
	stamp  uint32 // per-query marker, so a body spanning several cells is tested once
}

// StaticsChanged tells the world a static body was moved after being added.
func (w *World) StaticsChanged() { w.bp.dirty = true }

// aabb is a body's world-space bounding box; ok is false for unbounded shapes.
func aabb(b *Body) (lo, hi mathx.Vec3, ok bool) {
	switch b.Shape {
	case Sphere:
		r := mathx.Vec3{b.Radius, b.Radius, b.Radius}
		return b.Position.Sub(r), b.Position.Add(r), true
	case Box:
		m := b.Rotation.Mat4()
		he := b.HalfExtents
		var ext mathx.Vec3
		for i := 0; i < 3; i++ { // extent along world axis i: sum over the box axes
			ext[i] = abs32(m[i])*he[0] + abs32(m[4+i])*he[1] + abs32(m[8+i])*he[2]
		}
		return b.Position.Sub(ext), b.Position.Add(ext), true
	}
	return mathx.Vec3{}, mathx.Vec3{}, false
}

func cellOf(v float32) int32 { return int32(math.Floor(float64(v / gridCellFloat))) }

func cellRange(lo, hi mathx.Vec3) (a, b cellKey) {
	return cellKey{cellOf(lo[0]), cellOf(lo[1]), cellOf(lo[2])},
		cellKey{cellOf(hi[0]), cellOf(hi[1]), cellOf(hi[2])}
}

func (bp *broadphase) rebuild(bodies []*Body) {
	bp.cells = make(map[cellKey][]*Body, len(bodies))
	bp.always = bp.always[:0]
	for _, b := range bodies {
		if b.Kind == Dynamic {
			continue
		}
		lo, hi, ok := aabb(b)
		if b.Kind == Kinematic || !ok {
			bp.always = append(bp.always, b)
			continue
		}
		c0, c1 := cellRange(lo, hi)
		if n := int64(c1[0]-c0[0]+1) * int64(c1[1]-c0[1]+1) * int64(c1[2]-c0[2]+1); n > maxBodyCells {
			bp.always = append(bp.always, b)
			continue
		}
		for x := c0[0]; x <= c1[0]; x++ {
			for y := c0[1]; y <= c1[1]; y++ {
				for z := c0[2]; z <= c1[2]; z++ {
					k := cellKey{x, y, z}
					bp.cells[k] = append(bp.cells[k], b)
				}
			}
		}
	}
	bp.dirty = false
}

// detect finds every contact involving a dynamic body.
func (w *World) detect() []contact {
	bp := &w.bp
	if bp.dirty || bp.cells == nil {
		bp.rebuild(w.bodies)
	}
	var out []contact
	test := func(a, b *Body) {
		if a.Ignore == b || b.Ignore == a {
			return
		}
		if c, ok := collide(a, b); ok {
			out = append(out, c)
		}
	}

	dyn := w.dynamic[:0]
	for _, b := range w.bodies {
		if b.Kind == Dynamic {
			dyn = append(dyn, b)
		}
	}
	w.dynamic = dyn

	for i, a := range dyn {
		for _, b := range dyn[i+1:] {
			test(a, b)
		}
		for _, b := range bp.always {
			test(a, b)
		}
		lo, hi, _ := aabb(a)
		c0, c1 := cellRange(lo, hi)
		bp.stamp++
		for x := c0[0]; x <= c1[0]; x++ {
			for y := c0[1]; y <= c1[1]; y++ {
				for z := c0[2]; z <= c1[2]; z++ {
					for _, b := range bp.cells[cellKey{x, y, z}] {
						if b.stamp == bp.stamp {
							continue
						}
						b.stamp = bp.stamp
						test(a, b)
					}
				}
			}
		}
	}
	return out
}
