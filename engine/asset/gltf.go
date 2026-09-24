// Package asset loads files from disk into engine data (pure Go, no cgo).
package asset

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // glTF base colour maps are PNG or JPEG
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"

	"vkgame/engine/geom"
	"vkgame/engine/mathx"
)

// Material is the subset of glTF PBR the renderer uses today.
type Material struct {
	BaseColor      [4]float32 // linear RGBA factor
	BaseColorImage int        // index into Model.Images, or -1 for none
}

// Part is all of a model's triangles that share one material.
type Part struct {
	Mesh     geom.MeshData
	Material int // index into Model.Materials
}

// Model is a glTF scene flattened into world-space parts.
type Model struct {
	Parts     []Part
	Materials []Material
	Images    []*image.NRGBA // decoded base colour maps (sRGB, straight alpha)
}

// Bounds returns the bounding box of all parts.
func (m *Model) Bounds() (lo, hi mathx.Vec3) {
	for i, p := range m.Parts {
		plo, phi := p.Mesh.Bounds()
		if i == 0 {
			lo, hi = plo, phi
			continue
		}
		lo, hi = lo.Min(plo), hi.Max(phi)
	}
	return lo, hi
}

// FitToSize scales the whole model so its largest extent is size, centred on
// X/Z and resting on y = 0 (see geom.FitTransform).
func (m *Model) FitToSize(size float32) {
	lo, hi := m.Bounds()
	offset, scale := geom.FitTransform(lo, hi, size)
	for i := range m.Parts {
		m.Parts[i].Mesh.OffsetScale(offset, scale)
	}
}

// LoadGLTF reads a .gltf or .glb file. Every triangle primitive in the default
// scene is baked into world space and grouped by material. Missing normals are
// computed. Skins, animation and non-base-colour material maps are ignored.
func LoadGLTF(path string) (*Model, error) {
	doc, err := gltf.Open(path)
	if err != nil {
		return nil, err
	}
	l := loader{doc: doc, dir: filepath.Dir(path), partOf: map[int]int{}, imageOf: map[int]int{}}

	var walk func(node int, parent mathx.Mat4) error
	walk = func(node int, parent mathx.Mat4) error {
		if node < 0 || node >= len(doc.Nodes) {
			return fmt.Errorf("node index %d out of range", node)
		}
		n := doc.Nodes[node]
		world := parent.Mul(localTransform(n))
		if n.Mesh != nil {
			if err := l.appendMesh(*n.Mesh, world); err != nil {
				return fmt.Errorf("mesh %d: %w", *n.Mesh, err)
			}
		}
		for _, child := range n.Children {
			if err := walk(child, world); err != nil {
				return err
			}
		}
		return nil
	}

	for _, root := range sceneRoots(doc) {
		if err := walk(root, mathx.Identity()); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if len(l.model.Parts) == 0 {
		return nil, fmt.Errorf("%s: no triangle meshes found", path)
	}
	return &l.model, nil
}

type loader struct {
	doc     *gltf.Document
	dir     string
	model   Model
	partOf  map[int]int // glTF material index (-1 = none) -> index in model.Parts/Materials
	imageOf map[int]int // glTF image index -> index in model.Images
}

// sceneRoots returns the root nodes of the default scene (or the first scene).
func sceneRoots(doc *gltf.Document) []int {
	scene := 0
	if doc.Scene != nil {
		scene = *doc.Scene
	}
	if scene < len(doc.Scenes) {
		return doc.Scenes[scene].Nodes
	}
	return nil
}

func localTransform(n *gltf.Node) mathx.Mat4 {
	if m := n.MatrixOrDefault(); m != gltf.DefaultMatrix {
		var out mathx.Mat4
		for i, v := range m {
			out[i] = float32(v)
		}
		return out
	}
	t, r, s := n.TranslationOrDefault(), n.RotationOrDefault(), n.ScaleOrDefault()
	return mathx.Translate(float32(t[0]), float32(t[1]), float32(t[2])).
		Mul(mathx.FromQuat(float32(r[0]), float32(r[1]), float32(r[2]), float32(r[3]))).
		Mul(mathx.Scale(float32(s[0]), float32(s[1]), float32(s[2])))
}

func (l *loader) appendMesh(meshIndex int, world mathx.Mat4) error {
	doc := l.doc
	if meshIndex < 0 || meshIndex >= len(doc.Meshes) {
		return errors.New("index out of range")
	}
	normalMatrix := mathx.NormalMatrix(world)

	for _, prim := range doc.Meshes[meshIndex].Primitives {
		if prim.Mode != gltf.PrimitiveTriangles {
			continue // points, lines and strips aren't supported yet
		}
		posIdx, ok := prim.Attributes[gltf.POSITION]
		if !ok {
			continue
		}
		positions, err := modeler.ReadPosition(doc, doc.Accessors[posIdx], nil)
		if err != nil {
			return fmt.Errorf("positions: %w", err)
		}

		var normals [][3]float32
		if idx, ok := prim.Attributes[gltf.NORMAL]; ok {
			if normals, err = modeler.ReadNormal(doc, doc.Accessors[idx], nil); err != nil {
				return fmt.Errorf("normals: %w", err)
			}
		}
		var uvs [][2]float32
		if idx, ok := prim.Attributes[gltf.TEXCOORD_0]; ok {
			if uvs, err = modeler.ReadTextureCoord(doc, doc.Accessors[idx], nil); err != nil {
				return fmt.Errorf("uvs: %w", err)
			}
		}

		var indices []uint32
		if prim.Indices != nil {
			if indices, err = modeler.ReadIndices(doc, doc.Accessors[*prim.Indices], nil); err != nil {
				return fmt.Errorf("indices: %w", err)
			}
		} else {
			indices = make([]uint32, len(positions))
			for i := range indices {
				indices[i] = uint32(i)
			}
		}

		part := geom.MeshData{Vertices: make([]geom.Vertex, len(positions)), Indices: indices}
		for i, p := range positions {
			v := &part.Vertices[i]
			v.Position = world.TransformPoint(p)
			if i < len(normals) {
				v.Normal = mathx.TransformDir(normalMatrix, normals[i]).Normalize()
			}
			if i < len(uvs) {
				v.UV = uvs[i]
			}
		}
		if len(normals) != len(positions) {
			part.ComputeNormals()
		}
		if det3(world) < 0 {
			flipWinding(part.Indices) // mirrored transform: keep faces CCW
		}

		material := -1
		if prim.Material != nil {
			material = *prim.Material
		}
		slot, err := l.part(material)
		if err != nil {
			return err
		}
		l.model.Parts[slot].Mesh.Append(part)
	}
	return nil
}

// part returns the Part (and Material) slot for a glTF material, creating it.
func (l *loader) part(material int) (int, error) {
	if slot, ok := l.partOf[material]; ok {
		return slot, nil
	}
	mat := Material{BaseColor: [4]float32{1, 1, 1, 1}, BaseColorImage: -1}
	if material >= 0 {
		if material >= len(l.doc.Materials) {
			return 0, fmt.Errorf("material %d out of range", material)
		}
		if pbr := l.doc.Materials[material].PBRMetallicRoughness; pbr != nil {
			f := pbr.BaseColorFactorOrDefault() // already linear
			mat.BaseColor = [4]float32{float32(f[0]), float32(f[1]), float32(f[2]), float32(f[3])}
			if pbr.BaseColorTexture != nil {
				img, err := l.textureImage(pbr.BaseColorTexture.Index)
				if err != nil {
					return 0, fmt.Errorf("material %d base colour: %w", material, err)
				}
				mat.BaseColorImage = img
			}
		}
	}
	slot := len(l.model.Parts)
	l.model.Parts = append(l.model.Parts, Part{Material: slot})
	l.model.Materials = append(l.model.Materials, mat)
	l.partOf[material] = slot
	return slot, nil
}

// textureImage decodes the image behind a glTF texture (once) and returns its
// index in model.Images.
func (l *loader) textureImage(texture int) (int, error) {
	if texture < 0 || texture >= len(l.doc.Textures) || l.doc.Textures[texture].Source == nil {
		return 0, fmt.Errorf("texture %d has no image", texture)
	}
	source := *l.doc.Textures[texture].Source
	if idx, ok := l.imageOf[source]; ok {
		return idx, nil
	}
	data, err := l.imageBytes(source)
	if err != nil {
		return 0, fmt.Errorf("image %d: %w", source, err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("image %d: %w", source, err)
	}
	idx := len(l.model.Images)
	l.model.Images = append(l.model.Images, toNRGBA(decoded))
	l.imageOf[source] = idx
	return idx, nil
}

// imageBytes returns encoded image data from a buffer view, a data: URI or a
// file next to the glTF.
func (l *loader) imageBytes(index int) ([]byte, error) {
	if index < 0 || index >= len(l.doc.Images) {
		return nil, errors.New("index out of range")
	}
	img := l.doc.Images[index]
	switch {
	case img.BufferView != nil:
		if *img.BufferView >= len(l.doc.BufferViews) {
			return nil, errors.New("buffer view out of range")
		}
		bv := l.doc.BufferViews[*img.BufferView]
		if bv.Buffer >= len(l.doc.Buffers) {
			return nil, errors.New("buffer out of range")
		}
		data := l.doc.Buffers[bv.Buffer].Data
		if bv.ByteOffset+bv.ByteLength > len(data) {
			return nil, errors.New("buffer view exceeds buffer")
		}
		return data[bv.ByteOffset : bv.ByteOffset+bv.ByteLength], nil
	case img.IsEmbeddedResource():
		return img.MarshalData()
	case img.URI != "":
		name, err := url.PathUnescape(img.URI)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(filepath.Join(l.dir, filepath.FromSlash(name)))
	}
	return nil, errors.New("image has no data")
}

// toNRGBA returns img as tightly packed, non-premultiplied 8-bit RGBA.
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	if n, ok := img.(*image.NRGBA); ok && b.Min == (image.Point{}) && n.Stride == 4*b.Dx() {
		return n
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func det3(m mathx.Mat4) float32 {
	return m[0]*(m[5]*m[10]-m[9]*m[6]) - m[4]*(m[1]*m[10]-m[9]*m[2]) + m[8]*(m[1]*m[6]-m[5]*m[2])
}

func flipWinding(indices []uint32) {
	for i := 0; i+2 < len(indices); i += 3 {
		indices[i+1], indices[i+2] = indices[i+2], indices[i+1]
	}
}
