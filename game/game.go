// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
package game

import (
	"math"

	"vkgame/engine/mathx"
	"vkgame/engine/render"
)

type spinner struct {
	x, y  float32
	size  float32
	speed float32 // radians per second
	color [4]float32
}

type Game struct {
	time     float32
	spinners []spinner
}

func New() *Game {
	g := &Game{}
	g.spinners = append(g.spinners, spinner{size: 0.9, speed: 0.4, color: [4]float32{1, 1, 1, 1}})

	const ring = 6
	for i := 0; i < ring; i++ {
		angle := float64(i) / ring * 2 * math.Pi
		g.spinners = append(g.spinners, spinner{
			x:     float32(math.Cos(angle)) * 0.75,
			y:     float32(math.Sin(angle)) * 0.75,
			size:  0.25,
			speed: -1.5,
			color: [4]float32{1, 1, 1, 0.8},
		})
	}
	return g
}

func (g *Game) Update(dt float32) {
	g.time += dt
}

// Draw appends this frame's draw list to out[:0] and returns it, so the caller
// can reuse one slice every frame instead of allocating.
func (g *Game) Draw(aspect float32, out []render.DrawCmd) []render.DrawCmd {
	proj := mathx.Ortho(-aspect, aspect, -1, 1, -1, 1)
	out = out[:0]
	for _, s := range g.spinners {
		model := mathx.Translate(s.x, s.y, 0).
			Mul(mathx.RotateZ(g.time * s.speed)).
			Mul(mathx.Scale(s.size, s.size, 1))
		out = append(out, render.DrawCmd{MVP: proj.Mul(model), Color: s.color})
	}
	return out
}
