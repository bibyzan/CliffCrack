package asset

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"path/filepath"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"

	"CliffCrack/engine/mathx"
)

func saveGLB(t *testing.T, doc *gltf.Document) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.glb")
	if err := gltf.SaveBinary(doc, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// A single triangle (without normals) under a parent node that translates by
// (10, 0, 0) and a child node that scales by 2. No material.
func TestLoadGLTFTransformsAndNormals(t *testing.T) {
	doc := gltf.NewDocument()
	pos := modeler.WritePosition(doc, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}})
	idx := modeler.WriteIndices(doc, []uint16{0, 1, 2})
	doc.Meshes = []*gltf.Mesh{{Primitives: []*gltf.Primitive{{
		Indices:    gltf.Index(idx),
		Mode:       gltf.PrimitiveTriangles,
		Attributes: gltf.PrimitiveAttributes{gltf.POSITION: pos},
	}}}}
	doc.Nodes = []*gltf.Node{
		{Translation: [3]float64{10, 0, 0}, Children: []int{1}},
		{Scale: [3]float64{2, 2, 2}, Mesh: gltf.Index(0)},
	}
	doc.Scenes[0].Nodes = []int{0}

	m, err := LoadGLTF(saveGLB(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || len(m.Materials) != 1 {
		t.Fatalf("got %d parts / %d materials, want 1 / 1", len(m.Parts), len(m.Materials))
	}
	if mat := m.Materials[0]; mat.BaseColor != [4]float32{1, 1, 1, 1} || mat.BaseColorImage != -1 {
		t.Errorf("default material = %+v, want white and untextured", mat)
	}
	mesh := m.Parts[0].Mesh
	if len(mesh.Vertices) != 3 || len(mesh.Indices) != 3 {
		t.Fatalf("got %d verts / %d indices, want 3 / 3", len(mesh.Vertices), len(mesh.Indices))
	}

	want := []mathx.Vec3{{10, 0, 0}, {12, 0, 0}, {10, 2, 0}} // translate(scale(p))
	for i, v := range mesh.Vertices {
		if v.Position.Sub(want[i]).Len() > 1e-5 {
			t.Errorf("vertex %d at %v, want %v (node transforms not applied?)", i, v.Position, want[i])
		}
		// Normals were missing from the file, so they are computed: CCW triangle faces +Z.
		if n := v.Normal; math.Abs(float64(n[2]-1)) > 1e-5 {
			t.Errorf("vertex %d normal %v, want (0,0,1)", i, n)
		}
	}
}

// Two primitives: one with a textured, tinted material and one without.
func TestLoadGLTFMaterials(t *testing.T) {
	doc := gltf.NewDocument()
	tri := modeler.WritePosition(doc, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}})
	uv := modeler.WriteTextureCoord(doc, [][2]float32{{0, 0}, {1, 0}, {0, 1}})
	idx := modeler.WriteIndices(doc, []uint16{0, 1, 2})

	// 2x1 PNG: red, then half-transparent blue.
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	src.SetNRGBA(1, 0, color.NRGBA{0, 0, 255, 128})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}
	img, err := modeler.WriteImage(doc, "tex", "image/png", &encoded)
	if err != nil {
		t.Fatal(err)
	}
	doc.Textures = []*gltf.Texture{{Source: gltf.Index(img)}}
	doc.Materials = []*gltf.Material{{PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
		BaseColorFactor:  &[4]float64{1, 0.5, 0.25, 1},
		BaseColorTexture: &gltf.TextureInfo{Index: 0},
	}}}
	textured := &gltf.Primitive{
		Indices:    gltf.Index(idx),
		Material:   gltf.Index(0),
		Attributes: gltf.PrimitiveAttributes{gltf.POSITION: tri, gltf.TEXCOORD_0: uv},
	}
	plain := &gltf.Primitive{Indices: gltf.Index(idx), Attributes: gltf.PrimitiveAttributes{gltf.POSITION: tri}}
	doc.Meshes = []*gltf.Mesh{{Primitives: []*gltf.Primitive{textured, plain, textured}}}
	doc.Nodes = []*gltf.Node{{Mesh: gltf.Index(0)}}
	doc.Scenes[0].Nodes = []int{0}

	m, err := LoadGLTF(saveGLB(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 2 {
		t.Fatalf("got %d parts, want 2 (primitives grouped by material)", len(m.Parts))
	}
	if n := len(m.Parts[0].Mesh.Indices); n != 6 {
		t.Errorf("textured part has %d indices, want 6 (two primitives merged)", n)
	}
	mat := m.Materials[m.Parts[0].Material]
	if mat.BaseColor != [4]float32{1, 0.5, 0.25, 1} {
		t.Errorf("base colour factor = %v", mat.BaseColor)
	}
	if mat.BaseColorImage != 0 || len(m.Images) != 1 {
		t.Fatalf("base colour image = %d with %d images, want 0 with 1", mat.BaseColorImage, len(m.Images))
	}
	got := m.Images[0]
	if got.Bounds().Dx() != 2 || got.NRGBAAt(0, 0) != (color.NRGBA{255, 0, 0, 255}) ||
		got.NRGBAAt(1, 0) != (color.NRGBA{0, 0, 255, 128}) {
		t.Errorf("decoded image pixels wrong: %v %v", got.NRGBAAt(0, 0), got.NRGBAAt(1, 0))
	}
	if uv := m.Parts[0].Mesh.Vertices[1].UV; uv != [2]float32{1, 0} {
		t.Errorf("uv = %v, want (1,0)", uv)
	}
	if plainMat := m.Materials[m.Parts[1].Material]; plainMat.BaseColorImage != -1 {
		t.Errorf("untextured part has image %d", plainMat.BaseColorImage)
	}
}

func TestLoadGLTFMissingFile(t *testing.T) {
	if _, err := LoadGLTF(filepath.Join(t.TempDir(), "nope.glb")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
