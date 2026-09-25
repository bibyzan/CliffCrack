// Package arena is the FPS arena's simulation: a generated site of
// destructible structures, the players' movement and weapons, the rounds of
// a match and a bot to play against. It is pure Go (no GPU), so the whole
// game can be stepped and tested headlessly; package game draws it.
//
// The simulation is driven only by per-player Input commands, one per player
// per step, so a server can run it authoritatively and a bot is just another
// source of commands.
package arena

import (
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// BlockKind says what a block is, so the renderer can colour and trim it.
type BlockKind int

const (
	Floor BlockKind = iota
	Wall
)

// Block is an indestructible static box: the ground and the boundary walls.
type Block struct {
	Kind     BlockKind
	Centre   mathx.Vec3
	Half     mathx.Vec3
	Rotation mathx.Quat
}

// addLevel puts every block into the physics world as a static box.
func addLevel(w *physics.World, blocks []Block) {
	for _, b := range blocks {
		body := physics.NewBox(b.Half, physics.Static)
		body.Position, body.Rotation = b.Centre, b.Rotation
		body.Friction = 1
		body.Restitution = 0 // contacts use the bouncier body: the player lands dead, debris still bounces
		body.UserData = b.Kind
		if err := w.Add(body); err != nil {
			panic(err) // static boxes are always valid
		}
	}
}
