// Package asset loads files from disk into engine data (pure Go, no cgo).
package asset

import (
	"errors"
	"fmt"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"

	"vkgame/engine/geom"
	"vkgame/engine/mathx"
)

// LoadGLTF reads a .gltf or .glb file and flattens every triangle primitive in
// its default scene into one mesh, with node transforms baked into the vertices.
// Missing normals are computed; materials, skins and animation are ignored for now.
func LoadGLTF(path string) (geom.MeshData, error) {
	doc, err := gltf.Open(path)
	if err != nil {
		return geom.MeshData{}, err
	}

	var out geom.MeshData
	var walk func(node int, parent mathx.Mat4) error
	walk = func(node int, parent mathx.Mat4) error {
		if node < 0 || node >= len(doc.Nodes) {
			return fmt.Errorf("node index %d out of range", node)
		}
		n := doc.Nodes[node]
		world := parent.Mul(localTransform(n))
		if n.Mesh != nil {
			if err := appendMesh(doc, *n.Mesh, world, &out); err != nil {
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
			return geom.MeshData{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	if len(out.Indices) == 0 {
		return geom.MeshData{}, fmt.Errorf("%s: no triangle meshes found", path)
	}
	return out, nil
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

func appendMesh(doc *gltf.Document, meshIndex int, world mathx.Mat4, out *geom.MeshData) error {
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
		out.Append(part)
	}
	return nil
}

func det3(m mathx.Mat4) float32 {
	return m[0]*(m[5]*m[10]-m[9]*m[6]) - m[4]*(m[1]*m[10]-m[9]*m[2]) + m[8]*(m[1]*m[6]-m[5]*m[2])
}

func flipWinding(indices []uint32) {
	for i := 0; i+2 < len(indices); i += 3 {
		indices[i+1], indices[i+2] = indices[i+2], indices[i+1]
	}
}
