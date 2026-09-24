package geom

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

// checkWinding verifies every triangle is counter-clockwise when viewed from
// the side its vertex normals point to, i.e. back-face culling keeps it.
func checkWinding(t *testing.T, name string, m MeshData) {
	t.Helper()
	if len(m.Indices)%3 != 0 {
		t.Fatalf("%s: index count %d is not a multiple of 3", name, len(m.Indices))
	}
	for i := 0; i < len(m.Indices); i += 3 {
		a, b, c := m.Vertices[m.Indices[i]], m.Vertices[m.Indices[i+1]], m.Vertices[m.Indices[i+2]]
		face := b.Position.Sub(a.Position).Cross(c.Position.Sub(a.Position))
		if face.Len() < 1e-9 {
			continue // degenerate triangle at a sphere pole
		}
		avg := a.Normal.Add(b.Normal).Add(c.Normal)
		if face.Dot(avg) <= 0 {
			t.Fatalf("%s: triangle %d is wound clockwise relative to its normals", name, i/3)
		}
	}
}

func checkUnitNormals(t *testing.T, name string, m MeshData) {
	t.Helper()
	for i, v := range m.Vertices {
		if l := v.Normal.Len(); math.Abs(float64(l-1)) > 1e-4 {
			t.Fatalf("%s: vertex %d normal length %v", name, i, l)
		}
	}
}

func TestShapes(t *testing.T) {
	shapes := map[string]MeshData{
		"cube":   Cube(2),
		"plane":  Plane(10),
		"sphere": Sphere(1, 24, 12),
	}
	for name, m := range shapes {
		checkWinding(t, name, m)
		checkUnitNormals(t, name, m)
	}

	cube := Cube(2)
	if len(cube.Vertices) != 24 || len(cube.Indices) != 36 {
		t.Errorf("cube has %d verts / %d indices, want 24 / 36", len(cube.Vertices), len(cube.Indices))
	}
	if lo, hi := cube.Bounds(); lo != (mathx.Vec3{-1, -1, -1}) || hi != (mathx.Vec3{1, 1, 1}) {
		t.Errorf("cube bounds = %v..%v", lo, hi)
	}
}

func TestComputeNormalsMatchesAnalytic(t *testing.T) {
	cube := Cube(1)
	want := make([]mathx.Vec3, len(cube.Vertices))
	for i, v := range cube.Vertices {
		want[i] = v.Normal
	}
	cube.ComputeNormals()
	for i, v := range cube.Vertices {
		if v.Normal.Sub(want[i]).Len() > 1e-5 {
			t.Fatalf("vertex %d: computed normal %v, want %v", i, v.Normal, want[i])
		}
	}
}

func TestFitToSize(t *testing.T) {
	m := Cube(4)
	m.FitToSize(1)
	lo, hi := m.Bounds()
	if lo[1] != 0 || math.Abs(float64(hi[1]-1)) > 1e-6 {
		t.Errorf("after FitToSize: y range %v..%v, want 0..1", lo[1], hi[1])
	}
	if math.Abs(float64(lo[0]+hi[0])) > 1e-6 {
		t.Errorf("after FitToSize: not centred on X (%v..%v)", lo[0], hi[0])
	}
}
