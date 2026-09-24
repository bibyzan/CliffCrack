// Package geom holds CPU-side mesh data and procedural shapes. It is pure Go
// (no cgo), so asset loading and tests don't depend on the renderer DLL.
package geom

import (
	"math"

	"CliffCrack/engine/mathx"
)

// Vertex must match RVertex in renderer/include/renderer.h.
type Vertex struct {
	Position mathx.Vec3
	Normal   mathx.Vec3
	UV       [2]float32
}

// MeshData is an indexed triangle list with counter-clockwise front faces.
type MeshData struct {
	Vertices []Vertex
	Indices  []uint32
}

// Append adds other's triangles to m.
func (m *MeshData) Append(other MeshData) {
	base := uint32(len(m.Vertices))
	m.Vertices = append(m.Vertices, other.Vertices...)
	for _, i := range other.Indices {
		m.Indices = append(m.Indices, base+i)
	}
}

// Bounds returns the axis-aligned bounding box of the vertices.
func (m MeshData) Bounds() (lo, hi mathx.Vec3) {
	if len(m.Vertices) == 0 {
		return
	}
	lo, hi = m.Vertices[0].Position, m.Vertices[0].Position
	for _, v := range m.Vertices[1:] {
		lo, hi = lo.Min(v.Position), hi.Max(v.Position)
	}
	return lo, hi
}

// FitToSize uniformly scales the mesh so its largest extent is size, centres
// it on X/Z and rests it on y = 0.
func (m *MeshData) FitToSize(size float32) {
	lo, hi := m.Bounds()
	offset, scale := FitTransform(lo, hi, size)
	m.OffsetScale(offset, scale)
}

// FitTransform returns the offset and uniform scale that make the box lo..hi
// size units across at its largest extent, centred on X/Z and resting on y = 0.
// Apply the offset first, then the scale.
func FitTransform(lo, hi mathx.Vec3, size float32) (offset mathx.Vec3, scale float32) {
	ext := hi.Sub(lo)
	largest := max(ext[0], ext[1], ext[2])
	if largest == 0 {
		return mathx.Vec3{}, 1
	}
	return mathx.Vec3{-(lo[0] + hi[0]) / 2, -lo[1], -(lo[2] + hi[2]) / 2}, size / largest
}

// OffsetScale moves every vertex by offset, then scales it uniformly.
func (m *MeshData) OffsetScale(offset mathx.Vec3, scale float32) {
	for i := range m.Vertices {
		m.Vertices[i].Position = m.Vertices[i].Position.Add(offset).Scale(scale)
	}
}

// ComputeNormals sets smooth, area-weighted vertex normals from the triangles.
func (m *MeshData) ComputeNormals() {
	for i := range m.Vertices {
		m.Vertices[i].Normal = mathx.Vec3{}
	}
	for t := 0; t+2 < len(m.Indices); t += 3 {
		a, b, c := m.Indices[t], m.Indices[t+1], m.Indices[t+2]
		pa, pb, pc := m.Vertices[a].Position, m.Vertices[b].Position, m.Vertices[c].Position
		n := pb.Sub(pa).Cross(pc.Sub(pa)) // length = 2 * area
		for _, i := range [3]uint32{a, b, c} {
			m.Vertices[i].Normal = m.Vertices[i].Normal.Add(n)
		}
	}
	for i := range m.Vertices {
		m.Vertices[i].Normal = m.Vertices[i].Normal.Normalize()
	}
}

// quad returns one square face of half-size h, centred at centre, facing n.
// up must be perpendicular to n; u = up x n gives a CCW order seen from outside.
func quad(centre, n, up mathx.Vec3, h float32) MeshData {
	u := up.Cross(n)
	corners := [4][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}
	var m MeshData
	for _, c := range corners {
		p := centre.Add(u.Scale(c[0] * h)).Add(up.Scale(c[1] * h))
		m.Vertices = append(m.Vertices, Vertex{
			Position: p,
			Normal:   n,
			UV:       [2]float32{(c[0] + 1) / 2, (1 - c[1]) / 2},
		})
	}
	m.Indices = []uint32{0, 1, 2, 0, 2, 3}
	return m
}

// Cube returns an axis-aligned cube with the given edge length, centred on the origin.
func Cube(size float32) MeshData {
	h := size / 2
	faces := []struct{ n, up mathx.Vec3 }{
		{mathx.Vec3{1, 0, 0}, mathx.Vec3{0, 1, 0}},
		{mathx.Vec3{-1, 0, 0}, mathx.Vec3{0, 1, 0}},
		{mathx.Vec3{0, 0, 1}, mathx.Vec3{0, 1, 0}},
		{mathx.Vec3{0, 0, -1}, mathx.Vec3{0, 1, 0}},
		{mathx.Vec3{0, 1, 0}, mathx.Vec3{0, 0, -1}},
		{mathx.Vec3{0, -1, 0}, mathx.Vec3{0, 0, 1}},
	}
	var m MeshData
	for _, f := range faces {
		m.Append(quad(f.n.Scale(h), f.n, f.up, h))
	}
	return m
}

// Plane returns a square on the XZ plane facing +Y, centred on the origin.
func Plane(size float32) MeshData {
	return quad(mathx.Vec3{}, mathx.Vec3{0, 1, 0}, mathx.Vec3{0, 0, -1}, size/2)
}

// Sphere returns a UV sphere centred on the origin.
func Sphere(radius float32, segments, rings int) MeshData {
	var m MeshData
	for r := 0; r <= rings; r++ {
		theta := math.Pi * float64(r) / float64(rings) // 0 at the north pole
		for s := 0; s <= segments; s++ {
			phi := 2 * math.Pi * float64(s) / float64(segments)
			n := mathx.Vec3{
				float32(math.Sin(theta) * math.Cos(phi)),
				float32(math.Cos(theta)),
				float32(math.Sin(theta) * math.Sin(phi)),
			}
			m.Vertices = append(m.Vertices, Vertex{
				Position: n.Scale(radius),
				Normal:   n,
				UV:       [2]float32{float32(s) / float32(segments), float32(r) / float32(rings)},
			})
		}
	}
	stride := uint32(segments + 1)
	for r := uint32(0); r < uint32(rings); r++ {
		for s := uint32(0); s < uint32(segments); s++ {
			a := r*stride + s // this ring
			b := a + stride   // ring below
			m.Indices = append(m.Indices, a, a+1, b, a+1, b+1, b)
		}
	}
	return m
}

// FlipWinding reverses every triangle and normal, turning a closed mesh inside
// out (e.g. a sphere into a sky dome seen from within).
func (m *MeshData) FlipWinding() {
	for t := 0; t+2 < len(m.Indices); t += 3 {
		m.Indices[t+1], m.Indices[t+2] = m.Indices[t+2], m.Indices[t+1]
	}
	for i := range m.Vertices {
		m.Vertices[i].Normal = m.Vertices[i].Normal.Scale(-1)
	}
}

// Grid returns a surface of cols x rows vertices placed by pos(i, j), with i
// counting along a row and j counting rows. It faces +Y when i runs along +X
// and j along -Z. UVs span 0..1; normals are smooth (area-weighted).
func Grid(cols, rows int, pos func(i, j int) mathx.Vec3) MeshData {
	var m MeshData
	if cols < 2 || rows < 2 {
		return m
	}
	m.Vertices = make([]Vertex, 0, cols*rows)
	for j := 0; j < rows; j++ {
		for i := 0; i < cols; i++ {
			m.Vertices = append(m.Vertices, Vertex{
				Position: pos(i, j),
				UV:       [2]float32{float32(i) / float32(cols-1), float32(j) / float32(rows-1)},
			})
		}
	}
	m.Indices = make([]uint32, 0, (cols-1)*(rows-1)*6)
	for j := 0; j < rows-1; j++ {
		for i := 0; i < cols-1; i++ {
			a := uint32(j*cols + i) // this row
			b := a + uint32(cols)   // next row
			m.Indices = append(m.Indices, a, a+1, b+1, a, b+1, b)
		}
	}
	m.ComputeNormals()
	return m
}

// Cone returns a closed cone with its base on y = 0 and its tip at y = height.
// Each side face gets its own vertices, so it shades faceted.
func Cone(radius, height float32, segments int) MeshData {
	var m MeshData
	tip := mathx.Vec3{0, height, 0}
	rim := func(s int) mathx.Vec3 {
		a := 2 * math.Pi * float64(s) / float64(segments)
		return mathx.Vec3{float32(math.Cos(a)) * radius, 0, -float32(math.Sin(a)) * radius}
	}
	for s := 0; s < segments; s++ {
		p0, p1 := rim(s), rim(s+1)
		n := p1.Sub(p0).Cross(tip.Sub(p0)).Normalize()
		base := uint32(len(m.Vertices))
		m.Vertices = append(m.Vertices,
			Vertex{Position: p0, Normal: n, UV: [2]float32{0, 1}},
			Vertex{Position: p1, Normal: n, UV: [2]float32{1, 1}},
			Vertex{Position: tip, Normal: n, UV: [2]float32{0.5, 0}})
		m.Indices = append(m.Indices, base, base+1, base+2)
	}
	m.Append(Disc(radius, segments, true))
	return m
}

// Disc returns a flat disc on y = 0 facing +Y (or -Y when down is set).
func Disc(radius float32, segments int, down bool) MeshData {
	n := mathx.Vec3{0, 1, 0}
	if down {
		n = mathx.Vec3{0, -1, 0}
	}
	m := MeshData{Vertices: []Vertex{{Normal: n, UV: [2]float32{0.5, 0.5}}}}
	for s := 0; s <= segments; s++ {
		a := 2 * math.Pi * float64(s) / float64(segments)
		c, sn := float32(math.Cos(a)), float32(math.Sin(a))
		m.Vertices = append(m.Vertices, Vertex{
			Position: mathx.Vec3{c * radius, 0, -sn * radius},
			Normal:   n,
			UV:       [2]float32{0.5 + c/2, 0.5 + sn/2},
		})
	}
	for s := uint32(1); s <= uint32(segments); s++ {
		if down {
			m.Indices = append(m.Indices, 0, s+1, s)
		} else {
			m.Indices = append(m.Indices, 0, s, s+1)
		}
	}
	return m
}

// Icosphere returns a geodesic sphere: an icosahedron split `subdivisions`
// times and pushed out to radius. At 0-1 subdivisions it is a chunky low-poly
// ball, good for rocks. Vertices are shared, so shade it with gfx.DrawFlat for facets.
func Icosphere(radius float32, subdivisions int) MeshData {
	t := float32((1 + math.Sqrt(5)) / 2)
	pos := []mathx.Vec3{
		{-1, t, 0}, {1, t, 0}, {-1, -t, 0}, {1, -t, 0},
		{0, -1, t}, {0, 1, t}, {0, -1, -t}, {0, 1, -t},
		{t, 0, -1}, {t, 0, 1}, {-t, 0, -1}, {-t, 0, 1},
	}
	for i := range pos {
		pos[i] = pos[i].Normalize()
	}
	tris := []uint32{
		0, 11, 5, 0, 5, 1, 0, 1, 7, 0, 7, 10, 0, 10, 11,
		1, 5, 9, 5, 11, 4, 11, 10, 2, 10, 7, 6, 7, 1, 8,
		3, 9, 4, 3, 4, 2, 3, 2, 6, 3, 6, 8, 3, 8, 9,
		4, 9, 5, 2, 4, 11, 6, 2, 10, 8, 6, 7, 9, 8, 1,
	}
	for ; subdivisions > 0; subdivisions-- {
		mid := map[[2]uint32]uint32{}
		midpoint := func(a, b uint32) uint32 {
			key := [2]uint32{min(a, b), max(a, b)}
			if i, ok := mid[key]; ok {
				return i
			}
			pos = append(pos, pos[a].Add(pos[b]).Normalize())
			mid[key] = uint32(len(pos) - 1)
			return mid[key]
		}
		next := make([]uint32, 0, len(tris)*4)
		for i := 0; i+2 < len(tris); i += 3 {
			a, b, c := tris[i], tris[i+1], tris[i+2]
			ab, bc, ca := midpoint(a, b), midpoint(b, c), midpoint(c, a)
			next = append(next, a, ab, ca, b, bc, ab, c, ca, bc, ab, bc, ca)
		}
		tris = next
	}
	m := MeshData{Indices: tris}
	for _, p := range pos {
		m.Vertices = append(m.Vertices, Vertex{
			Position: p.Scale(radius),
			Normal:   p,
			UV:       [2]float32{0.5 + float32(math.Atan2(float64(p[2]), float64(p[0])))/(2*math.Pi), 0.5 - p[1]/2},
		})
	}
	return m
}
