package asset

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"

	"vkgame/engine/mathx"
)

// writeTestGLB writes a single triangle (without normals) under a parent node
// that translates by (10, 0, 0) and a child node that scales by 2.
func writeTestGLB(t *testing.T) string {
	t.Helper()
	doc := gltf.NewDocument()
	pos := modeler.WritePosition(doc, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}})
	idx := modeler.WriteIndices(doc, []uint16{0, 1, 2})
	doc.Meshes = []*gltf.Mesh{{
		Primitives: []*gltf.Primitive{{
			Indices:    gltf.Index(idx),
			Mode:       gltf.PrimitiveTriangles,
			Attributes: gltf.PrimitiveAttributes{gltf.POSITION: pos},
		}},
	}}
	doc.Nodes = []*gltf.Node{
		{Translation: [3]float64{10, 0, 0}, Children: []int{1}},
		{Scale: [3]float64{2, 2, 2}, Mesh: gltf.Index(0)},
	}
	doc.Scenes[0].Nodes = []int{0}

	path := filepath.Join(t.TempDir(), "triangle.glb")
	if err := gltf.SaveBinary(doc, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadGLTF(t *testing.T) {
	m, err := LoadGLTF(writeTestGLB(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Vertices) != 3 || len(m.Indices) != 3 {
		t.Fatalf("got %d verts / %d indices, want 3 / 3", len(m.Vertices), len(m.Indices))
	}

	want := []mathx.Vec3{{10, 0, 0}, {12, 0, 0}, {10, 2, 0}} // translate(scale(p))
	for i, v := range m.Vertices {
		if v.Position.Sub(want[i]).Len() > 1e-5 {
			t.Errorf("vertex %d at %v, want %v (node transforms not applied?)", i, v.Position, want[i])
		}
		// Normals were missing from the file, so they are computed: CCW triangle faces +Z.
		if n := v.Normal; math.Abs(float64(n[2]-1)) > 1e-5 {
			t.Errorf("vertex %d normal %v, want (0,0,1)", i, n)
		}
	}
}

func TestLoadGLTFMissingFile(t *testing.T) {
	if _, err := LoadGLTF(filepath.Join(t.TempDir(), "nope.glb")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
