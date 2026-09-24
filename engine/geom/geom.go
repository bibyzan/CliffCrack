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
