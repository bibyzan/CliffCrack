package game

import (
	"math"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
)

// The Arena's models share the mountains' look: faceted and flat-shaded,
// with no hard boxes. Boxes are chamfered on every edge and corner, round
// parts are low-poly gems, limbs are tapered prisms, and each model is baked
// into one mesh per colour, so the extra facets cost few draws.

// chamferBox is a box of the given half extents with every edge cut back
// by c at 45 degrees and every corner cut to a triangle: 26 flat faces.
// Its UVs are projected per face across -1..1 of each axis (for textured
// chunks).
func chamferBox(half mathx.Vec3, c float32) geom.MeshData {
	c = min(c, half[0]*0.6, half[1]*0.6, half[2]*0.6)
	// p is the vertex at corner s (signs) that lies on the face of axis k:
	// full out on k, pulled in by c on the other two.
	p := func(s [3]float32, k int) mathx.Vec3 {
		var v mathx.Vec3
		for i := range 3 {
			v[i] = s[i] * half[i]
			if i != k {
				v[i] = s[i] * (half[i] - c)
			}
		}
		return v
	}
	var m geom.MeshData
	face := func(pts ...mathx.Vec3) {
		// Wind it facing out (the box is convex round the origin).
		var centre mathx.Vec3
		for _, q := range pts {
			centre = centre.Add(q)
		}
		n := pts[1].Sub(pts[0]).Cross(pts[2].Sub(pts[0])).Normalize()
		if n.Dot(centre) < 0 {
			for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
				pts[i], pts[j] = pts[j], pts[i]
			}
			n = n.Scale(-1)
		}
		base := uint32(len(m.Vertices))
		// UVs projected on the face's main axis.
		ax := 0
		for i := range 3 {
			if abs32(n[i]) > abs32(n[ax]) {
				ax = i
			}
		}
		u, v := (ax+1)%3, (ax+2)%3
		for _, q := range pts {
			m.Vertices = append(m.Vertices, geom.Vertex{Position: q, Normal: n,
				UV: [2]float32{q[u]/(2*half[u]) + 0.5, 0.5 - q[v]/(2*half[v])}})
		}
		for i := uint32(1); i+1 < uint32(len(pts)); i++ {
			m.Indices = append(m.Indices, base, base+i, base+i+1)
		}
	}
	signs := []float32{-1, 1}
	for k := range 3 { // the six faces
		a, b := (k+1)%3, (k+2)%3
		for _, sk := range signs {
			var pts []mathx.Vec3
			for _, q := range [][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
				var s [3]float32
				s[k], s[a], s[b] = sk, q[0], q[1]
				pts = append(pts, p(s, k))
			}
			face(pts...)
		}
	}
	for k := range 3 { // the twelve edge chamfers, along axis k
		a, b := (k+1)%3, (k+2)%3
		for _, sa := range signs {
			for _, sb := range signs {
				var lo, hi [3]float32
				lo[k], lo[a], lo[b] = -1, sa, sb
				hi[k], hi[a], hi[b] = 1, sa, sb
				face(p(lo, a), p(hi, a), p(hi, b), p(lo, b))
			}
		}
	}
	for _, sx := range signs { // the eight corners
		for _, sy := range signs {
			for _, sz := range signs {
				s := [3]float32{sx, sy, sz}
				face(p(s, 0), p(s, 1), p(s, 2))
			}
		}
	}
	return m
}

// gem is a faceted ball: an icosphere of 80 faces, stretched to half.
func gem(half mathx.Vec3) geom.MeshData {
	m := geom.Icosphere(1, 1)
	for i := range m.Vertices {
		v := &m.Vertices[i]
		v.Position = mathx.Vec3{v.Position[0] * half[0], v.Position[1] * half[1], v.Position[2] * half[2]}
	}
	m.ComputeNormals()
	return m
}

// flatRing is a thin annulus of radius r in the XY plane, facing both ways.
func flatRing(r float32) geom.MeshData {
	const n = 16
	var m geom.MeshData
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		c, s := float32(math.Cos(a)), float32(math.Sin(a))
		m.Vertices = append(m.Vertices, geom.Vertex{Position: mathx.Vec3{c * r, s * r, 0}, Normal: mathx.Vec3{0, 0, 1}},
			geom.Vertex{Position: mathx.Vec3{c * r * 1.2, s * r * 1.2, 0}, Normal: mathx.Vec3{0, 0, 1}})
	}
	for i := 0; i < n; i++ {
		a := uint32(2 * i)
		m.Indices = append(m.Indices, a, a+1, a+3, a, a+3, a+2, a, a+3, a+1, a, a+2, a+3)
	}
	return m
}

// limbMesh is an octagonal prism along Z from -1 to 1, radius 1, its ends
// tapered to a point-ish cap: a limb when scaled (r, r, length/2).
func limbMesh() geom.MeshData {
	rings := []struct{ z, r float32 }{{-1, 0.35}, {-0.86, 0.9}, {-0.7, 1}, {0.7, 1}, {0.86, 0.9}, {1, 0.35}}
	const sides = 8
	m := geom.Grid(sides+1, len(rings), func(i, j int) mathx.Vec3 {
		a := 2 * math.Pi * (float64(i) + 0.5) / sides
		return mathx.Vec3{rings[j].r * float32(math.Cos(a)), rings[j].r * float32(math.Sin(a)), rings[j].z}
	})
	// Caps.
	for _, end := range []int{0, len(rings) - 1} {
		centre := uint32(len(m.Vertices))
		m.Vertices = append(m.Vertices, geom.Vertex{Position: mathx.Vec3{0, 0, rings[end].z}})
		for i := 0; i < sides; i++ {
			a, b := uint32(end*(sides+1)+i), uint32(end*(sides+1)+i+1)
			if end == 0 {
				m.Indices = append(m.Indices, centre, b, a)
			} else {
				m.Indices = append(m.Indices, centre, a, b)
			}
		}
	}
	m.ComputeNormals()
	return m
}

// meshBuilder merges shapes into one mesh.
type meshBuilder struct{ m geom.MeshData }

func (mb *meshBuilder) add(shape geom.MeshData, at mathx.Vec3) {
	base := uint32(len(mb.m.Vertices))
	for _, v := range shape.Vertices {
		v.Position = v.Position.Add(at)
		mb.m.Vertices = append(mb.m.Vertices, v)
	}
	for _, i := range shape.Indices {
		mb.m.Indices = append(mb.m.Indices, base+i)
	}
}

// partShape is a part's shape, at the origin.
func partShape(p gunPart) geom.MeshData {
	switch {
	case p.round:
		return gem(p.half)
	case p.ring:
		return flatRing(p.half[0])
	}
	return chamferBox(p.half, 0.5*min(p.half[0], p.half[1], p.half[2])) // octagonal: as round as a box gets
}

// bakedGroup is the parts of a model of one colour (and lighting), merged.
type bakedGroup struct {
	mesh  render.Mesh
	color [4]float32 // zero: tinted when drawn (a character's suit or visor)
	flags gfx.DrawFlags
}

// baked models, by their parts slice (its first element and length).
type bakeKey struct {
	first *gunPart
	n     int
}

var baked = map[bakeKey][]bakedGroup{}

// bake merges parts into one mesh per colour and lighting, once per slice.
func bake(parts []gunPart) []bakedGroup {
	if len(parts) == 0 {
		return nil
	}
	key := bakeKey{&parts[0], len(parts)}
	if g, ok := baked[key]; ok {
		return g
	}
	type groupKey struct {
		color [4]float32
		flags gfx.DrawFlags
	}
	builders := map[groupKey]*meshBuilder{}
	var order []groupKey
	for _, p := range parts {
		k := groupKey{p.color, p.flags}
		if builders[k] == nil {
			builders[k] = &meshBuilder{}
			order = append(order, k)
		}
		builders[k].add(partShape(p), p.centre)
	}
	var groups []bakedGroup
	for _, k := range order {
		mesh, err := render.CreateMesh(builders[k].m)
		if err != nil {
			logf("arena: baking a model: %v", err)
			continue
		}
		flags := k.flags
		if flags&gfx.DrawUnlit == 0 {
			flags |= gfx.DrawFlat // faceted, like the mountains
		}
		groups = append(groups, bakedGroup{mesh: mesh, color: k.color, flags: flags})
	}
	baked[key] = groups
	return groups
}

// drawBaked draws a baked model under frame. Parts with no colour of their
// own take tint (lit) or glowTint (unlit).
func drawBaked(out []render.DrawCmd, frame mathx.Mat4, groups []bakedGroup, tint, glowTint [4]float32) []render.DrawCmd {
	for _, g := range groups {
		col := g.color
		if col == ([4]float32{}) {
			col = tint
			if g.flags&gfx.DrawUnlit != 0 {
				col = glowTint
			}
		}
		out = append(out, render.DrawCmd{Model: frame, Color: col, Flags: g.flags, Mesh: g.mesh})
	}
	return out
}
