// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
package game

import (
	"fmt"
	"math"

	"vkgame/engine/asset"
	"vkgame/engine/geom"
	"vkgame/engine/mathx"
	"vkgame/engine/render"
)

type object struct {
	mesh     render.Mesh
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
	cube, err := render.CreateMesh(geom.Cube(1))
	if err != nil {
		return nil, err
	}
	ground, err := render.CreateMesh(geom.Plane(1))
	if err != nil {
		return nil, err
	}

	centre := geom.Sphere(0.75, 48, 24)
	centreY := float32(1)
	if modelPath != "" {
		if centre, err = asset.LoadGLTF(modelPath); err != nil {
			return nil, err
		}
		centre.FitToSize(2)
		centreY = 0
		fmt.Printf("loaded %s: %d vertices, %d triangles\n", modelPath, len(centre.Vertices), len(centre.Indices)/3)
	}
	centreMesh, err := render.CreateMesh(centre)
	if err != nil {
		return nil, err
	}

	g := &Game{}
	g.objects = append(g.objects,
		object{mesh: ground, scale: 14, color: mathx.Hex(0x6b7280)},
		object{mesh: centreMesh, position: mathx.Vec3{0, centreY, 0}, scale: 1, spin: 0.3, color: mathx.Hex(0xe5e7eb)},
	)

	palette := []uint32{0xe05252, 0xf0923a, 0xe8cf45, 0x4cbf6b, 0x4a90e2, 0x9b6ce0}
	for i, hex := range palette {
		angle := float64(i) / float64(len(palette)) * 2 * math.Pi
		g.objects = append(g.objects, object{
			mesh:     cube,
			position: mathx.Vec3{float32(math.Cos(angle)) * 3.5, 0.5, float32(math.Sin(angle)) * 3.5},
			scale:    1,
			spin:     0.8 + 0.2*float32(i),
			color:    mathx.Hex(hex),
		})
	}
	return g, nil
}

func (g *Game) Update(dt float32) {
	g.time += dt
}

// Draw appends this frame's draw list to out[:0] and returns it, so the caller
// can reuse one slice every frame instead of allocating.
func (g *Game) Draw(aspect float32, out []render.DrawCmd) []render.DrawCmd {
	orbit := g.time * 0.15
	eye := mathx.Vec3{float32(math.Cos(float64(orbit))) * 9, 4.5, float32(math.Sin(float64(orbit))) * 9}
	view := mathx.LookAt(eye, mathx.Vec3{0, 0.75, 0}, mathx.Vec3{0, 1, 0})
	viewProj := mathx.Perspective(math.Pi/4, aspect, 0.1, 100).Mul(view)

	out = out[:0]
	for _, o := range g.objects {
		model := mathx.Translate(o.position[0], o.position[1], o.position[2]).
			Mul(mathx.RotateY(g.time * o.spin)).
			Mul(mathx.Scale(o.scale, o.scale, o.scale))
		out = append(out, render.DrawCmd{
			MVP:          viewProj.Mul(model),
			NormalMatrix: mathx.NormalMatrix(model),
			Color:        o.color,
			Mesh:         o.mesh,
		})
	}
	return out
}
