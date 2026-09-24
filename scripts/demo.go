// Package scripts holds hot-reloadable gameplay behaviours. The game runs these
// files through an interpreter and reloads them when they change: edit a
// number, save, and watch the running game update. `go vet ./...` still
// type-checks them like normal Go.
//
// A behaviour is any exported func with this signature:
//
//	func Name(w *scene.World, e *scene.Entity, dt float32)
package scripts

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/scene"
)

// Bob floats an entity up and down around y = 1.
func Bob(w *scene.World, e *scene.Entity, dt float32) {
	const height, amplitude, speed = 1.0, 0.25, 2.0
	e.Transform.Position[1] = height + amplitude*float32(math.Sin(float64(w.Time()*speed)))
}

// Pulse breathes an entity's scale around 1.
func Pulse(w *scene.World, e *scene.Entity, dt float32) {
	const amount, speed = 0.08, 3.0
	s := 1 + amount*float32(math.Sin(float64(w.Time()*speed)))
	e.Transform.Scale = mathx.Vec3{s, s, s}
}
