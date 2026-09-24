// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
package game

import (
	"fmt"
	"image/color"
	"math"

	"vkgame/engine/asset"
	"vkgame/engine/geom"
	"vkgame/engine/mathx"
	"vkgame/engine/render"
)

type object struct {
	mesh     render.Mesh
	texture  render.Texture
	position mathx.Vec3
	scale    float32
	spin     float32 // radians per second around Y
	color    [4]float32
}

type Game struct {
	time    float32
	objects []object
}

// New builds the demo scene. If modelPath is set, that glTF file is shown in
// the centre instead of the sphere.
func New(modelPath string) (*Game, error) {
	g := &Game{}

	ground, err := render.CreateMesh(scaledUVs(geom.Plane(1), 7))
	if err != nil {
		return nil, err
	}
	groundTex, err := render.CreateTexture(checker(256, 2,
		color.NRGBA{0x8a, 0x93, 0x9e, 255}, color.NRGBA{0x6b, 0x72, 0x80, 255}), true)
	if err != nil {
		return nil, err
	}
	g.objects = append(g.objects, object{mesh: ground, texture: groundTex, scale: 14, color: [4]float32{1, 1, 1, 1}})

	if modelPath != "" {
		if err := g.addModel(modelPath); err != nil {
			return nil, err
		}
	} else {
		sphere, err := render.CreateMesh(geom.Sphere(0.75, 48, 24))
		if err != nil {
			return nil, err
		}
		g.objects = append(g.objects, object{mesh: sphere, position: mathx.Vec3{0, 1, 0}, scale: 1, color: mathx.Hex(0xe5e7eb)})
	}

	cube, err := render.CreateMesh(geom.Cube(1))
	if err != nil {
		return nil, err
	}
	panelTex, err := render.CreateTexture(panel(128, 12), true)
	if err != nil {
		return nil, err
	}
	palette := []uint32{0xe05252, 0xf0923a, 0xe8cf45, 0x4cbf6b, 0x4a90e2, 0x9b6ce0}
	for i, hex := range palette {
		angle := float64(i) / float64(len(palette)) * 2 * math.Pi
		g.objects = append(g.objects, object{
			mesh:     cube,
			texture:  panelTex,
			position: mathx.Vec3{float32(math.Cos(angle)) * 3.5, 0.5, float32(math.Sin(angle)) * 3.5},
			scale:    1,
			spin:     0.8 + 0.2*float32(i),
			color:    mathx.Hex(hex),
		})
	}
	return g, nil
}

// addModel loads a glTF file, fits it into a 2-unit box at the origin and
// uploads one mesh per material, plus its base colour textures.
func (g *Game) addModel(path string) error {
	model, err := asset.LoadGLTF(path)
	if err != nil {
		return err
	}
	model.FitToSize(2)

	textures := make([]render.Texture, len(model.Images))
	for i, img := range model.Images {
		if textures[i], err = render.CreateTexture(img, true); err != nil {
			return fmt.Errorf("%s image %d: %w", path, i, err)
		}
	}
	triangles := 0
	for _, part := range model.Parts {
		mesh, err := render.CreateMesh(part.Mesh)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		mat := model.Materials[part.Material]
		obj := object{mesh: mesh, scale: 1, spin: 0.3, color: mat.BaseColor}
		if mat.BaseColorImage >= 0 {
			obj.texture = textures[mat.BaseColorImage]
		}
		g.objects = append(g.objects, obj)
		triangles += len(part.Mesh.Indices) / 3
	}
	fmt.Printf("loaded %s: %d parts, %d triangles, %d textures\n", path, len(model.Parts), triangles, len(textures))
	return nil
}

func (g *Game) Update(dt float32) {
	g.time += dt
}

// Render returns this frame's scene parameters and draw list. The draw list is
// appended to out[:0], so the caller can reuse one slice every frame.
func (g *Game) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	orbit := float64(g.time * 0.15)
	eye := mathx.Vec3{float32(math.Cos(orbit)) * 9, 4.5, float32(math.Sin(orbit)) * 9}
	view := mathx.LookAt(eye, mathx.Vec3{0, 0.75, 0}, mathx.Vec3{0, 1, 0})

	params := render.FrameParams{
		ViewProj:     mathx.Perspective(math.Pi/4, aspect, 0.1, 100).Mul(view),
		CameraPos:    eye,
		SunDirection: mathx.Vec3{0.4, 1, 0.3},
		SunColor:     mathx.Vec3{1, 0.95, 0.85},
		Ambient:      mathx.Vec3{0.18, 0.2, 0.25},
		Clear:        mathx.Hex(0x9cc3e6),
	}

	out = out[:0]
	for _, o := range g.objects {
		model := mathx.Translate(o.position[0], o.position[1], o.position[2]).
			Mul(mathx.RotateY(g.time * o.spin)).
			Mul(mathx.Scale(o.scale, o.scale, o.scale))
		out = append(out, render.DrawCmd{Model: model, Color: o.color, Texture: o.texture, Mesh: o.mesh})
	}
	return params, out
}

// scaledUVs multiplies a mesh's texture coordinates, so textures repeat.
func scaledUVs(m geom.MeshData, repeat float32) geom.MeshData {
	for i := range m.Vertices {
		m.Vertices[i].UV[0] *= repeat
		m.Vertices[i].UV[1] *= repeat
	}
	return m
}
