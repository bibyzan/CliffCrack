// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
package game

import (
	"fmt"
	"image/color"
	"math"

	"vkgame/engine/asset"
	"vkgame/engine/geom"
	"vkgame/engine/input"
	"vkgame/engine/mathx"
	"vkgame/engine/render"
	"vkgame/engine/scene"
)

var worldUp = mathx.Vec3{0, 1, 0}

type Game struct {
	world  *scene.World
	camera cameraRig
}

// New builds the demo scene. If modelPath is set, that glTF file is shown in
// the centre instead of the sphere.
func New(modelPath string) (*Game, error) {
	g := &Game{world: scene.NewWorld(), camera: newCameraRig()}
	w := g.world

	groundMesh, err := render.CreateMesh(scaledUVs(geom.Plane(1), 7))
	if err != nil {
		return nil, err
	}
	groundTex, err := render.CreateTexture(checker(256, 2,
		color.NRGBA{0x8a, 0x93, 0x9e, 255}, color.NRGBA{0x6b, 0x72, 0x80, 255}), true)
	if err != nil {
		return nil, err
	}
	ground := w.Spawn("ground", scene.ID{})
	ground.Transform.Scale = mathx.Vec3{14, 1, 14}
	ground.Renderable = &scene.Renderable{Mesh: groundMesh, Texture: groundTex, Color: [4]float32{1, 1, 1, 1}}

	centre := w.Spawn("centre", scene.ID{}).AddBehaviour(spin(0.3))
	if modelPath != "" {
		if err := g.addModel(modelPath, centre.ID()); err != nil {
			return nil, err
		}
	} else {
		sphere, err := render.CreateMesh(geom.Sphere(0.75, 48, 24))
		if err != nil {
			return nil, err
		}
		centre.Transform.Position = mathx.Vec3{0, 1, 0}
		centre.Renderable = &scene.Renderable{Mesh: sphere, Color: mathx.Hex(0xe5e7eb)}
	}

	cube, err := render.CreateMesh(geom.Cube(1))
	if err != nil {
		return nil, err
	}
	panelTex, err := render.CreateTexture(panel(128, 12), true)
	if err != nil {
		return nil, err
	}
	// The cubes hang off a slowly turning ring, and each also spins on its own.
	ring := w.Spawn("ring", scene.ID{}).AddBehaviour(spin(-0.1))
	palette := []uint32{0xe05252, 0xf0923a, 0xe8cf45, 0x4cbf6b, 0x4a90e2, 0x9b6ce0}
	for i, hex := range palette {
		angle := float64(i) / float64(len(palette)) * 2 * math.Pi
		c := w.Spawn(fmt.Sprintf("cube%d", i), ring.ID()).AddBehaviour(spin(0.8 + 0.2*float32(i)))
		c.Transform.Position = mathx.Vec3{float32(math.Cos(angle)) * 3.5, 0.5, float32(math.Sin(angle)) * 3.5}
		c.Renderable = &scene.Renderable{Mesh: cube, Texture: panelTex, Color: mathx.Hex(hex)}
	}

	w.UpdateTransforms()
	return g, nil
}

// addModel loads a glTF file, fits it into a 2-unit box and adds one child
// entity per material under parent.
func (g *Game) addModel(path string, parent scene.ID) error {
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
	for i, part := range model.Parts {
		mesh, err := render.CreateMesh(part.Mesh)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		mat := model.Materials[part.Material]
		r := &scene.Renderable{Mesh: mesh, Color: mat.BaseColor}
		if mat.BaseColorImage >= 0 {
			r.Texture = textures[mat.BaseColorImage]
		}
		g.world.Spawn(fmt.Sprintf("part%d", i), parent).Renderable = r
		triangles += len(part.Mesh.Indices) / 3
	}
	fmt.Printf("loaded %s: %d parts, %d triangles, %d textures\n", path, len(model.Parts), triangles, len(textures))
	return nil
}

func (g *Game) Update(dt float32, in *input.State) {
	g.camera.update(dt, in)
	g.world.Update(dt)
}

// CursorLocked reports whether the mouse should be captured (mouse-look).
func (g *Game) CursorLocked() bool { return g.camera.locked }

// Render returns this frame's scene parameters and draw list. The draw list is
// appended to out[:0], so the caller can reuse one slice every frame.
func (g *Game) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	view, eye := g.camera.view()
	params := render.FrameParams{
		ViewProj:     g.camera.lens.Projection(aspect).Mul(view),
		CameraPos:    eye,
		SunDirection: mathx.Vec3{0.4, 1, 0.3},
		SunColor:     mathx.Vec3{1, 0.95, 0.85},
		Ambient:      mathx.Vec3{0.18, 0.2, 0.25},
		Clear:        mathx.Hex(0x9cc3e6),
	}
	return params, g.world.AppendDraws(out[:0])
}

// spin rotates an entity around world up at a constant rate (radians/second).
func spin(speed float32) scene.Behaviour {
	return func(_ *scene.World, e *scene.Entity, dt float32) {
		e.Transform.Rotation = mathx.AxisAngle(worldUp, speed*dt).Mul(e.Transform.Rotation).Normalize()
	}
}

// scaledUVs multiplies a mesh's texture coordinates, so textures repeat.
func scaledUVs(m geom.MeshData, repeat float32) geom.MeshData {
	for i := range m.Vertices {
		m.Vertices[i].UV[0] *= repeat
		m.Vertices[i].UV[1] *= repeat
	}
	return m
}
