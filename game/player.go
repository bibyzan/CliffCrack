package game

import (
	"image/color"
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/engine/render"
	"CliffCrack/engine/scene"
)

const (
	playerRadius = 0.4
	rollAccel    = 40.0 // rad/s^2 of spin added while a direction is held
	maxSpin      = 16.0 // rad/s: top rolling speed = maxSpin * radius
	airControl   = 3.0  // m/s^2 of direct push, so steering still works mid-air
	jumpSpeed    = 5.5  // m/s
	brake        = 3.0  // 1/s: extra spin decay on the ground with no input, so it stops
	followRate   = 8.0  // how quickly the camera target catches up (1/s)
	bumpSpeed    = 1.0  // m/s: slower touches don't make cubes wobble
)

var playerSpawn = mathx.Vec3{0, 1, 5}

// player is the ball the user rolls around (WASD, Space, R).
type player struct {
	entity scene.ID
	body   *physics.Body
}

func (g *Game) spawnPlayer() error {
	tex, err := render.CreateTexture(checker(128, 4,
		color.NRGBA{0xf5, 0xf5, 0xf5, 255}, color.NRGBA{0xe0, 0x6a, 0x1b, 255}), true)
	if err != nil {
		return err
	}
	e := g.world.Spawn("player", scene.ID{})
	e.Transform.Scale = mathx.Vec3{playerRadius, playerRadius, playerRadius}
	e.Renderable = &scene.Renderable{Mesh: g.ballMesh, Texture: tex, Color: [4]float32{1, 1, 1, 1}}

	b := physics.NewSphere(playerRadius, 1.5)
	b.Restitution = 0.3
	b.Friction = 0.9 // grippy, so spin turns into motion
	b.Position = playerSpawn
	b.UserData = e.ID()
	if err := g.phys.Add(b); err != nil {
		return err
	}
	e.Transform.Position = b.Position
	g.links = append(g.links, physLink{entity: e.ID(), body: b, size: mathx.Vec3{playerRadius}})
	g.player = player{entity: e.ID(), body: b}
	return nil
}

// rollControl steers a ball like a marble: holding a direction spins it about
// the axis that rolls it that way, and ground friction turns spin into motion.
// A little direct force keeps it steerable in the air. move is a horizontal
// direction with length <= 1. It reports whether a jump happened.
func rollControl(b *physics.Body, move mathx.Vec3, jump bool, dt float32) bool {
	up := mathx.Vec3{0, 1, 0}
	if move.Len() > 0 {
		// Rolling without slipping along d needs spin about (up x d).
		b.AngularVelocity = b.AngularVelocity.Add(up.Cross(move).Scale(rollAccel * dt))
		if s := b.AngularVelocity.Len(); s > maxSpin {
			b.AngularVelocity = b.AngularVelocity.Scale(maxSpin / s)
		}
		b.Velocity = b.Velocity.Add(move.Scale(airControl * dt))
	} else if b.Grounded {
		b.AngularVelocity = b.AngularVelocity.Scale(float32(math.Exp(-brake * float64(dt))))
	}
	if jump && b.Grounded {
		b.Velocity[1] = jumpSpeed
		return true
	}
	return false
}

// updatePlayer reads input (orbit mode only; in fly mode WASD moves the camera).
func (g *Game) updatePlayer(dt float32, in *input.State) {
	b := g.player.body
	if b == nil {
		return
	}
	if in.Pressed(input.KeyR) || b.Position[1] < -5 {
		b.Position, b.Velocity, b.AngularVelocity = playerSpawn, mathx.Vec3{}, mathx.Vec3{}
	}
	if g.camera.fly {
		return
	}

	// Directions are relative to where the camera looks, flattened onto the ground.
	forward := camera.Direction(g.camera.orbit.Yaw, 0)
	right := forward.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	move := forward.Scale(in.Axis(input.KeyS, input.KeyW)).Add(right.Scale(in.Axis(input.KeyA, input.KeyD)))
	if l := move.Len(); l > 1 {
		move = move.Scale(1 / l)
	}
	jump := in.Pressed(input.KeySpace)
	if move.Len() > 0 || jump {
		g.camera.idle = 0 // no auto-orbit while playing: controls are camera-relative
	}
	if rollControl(b, move, jump, dt) {
		g.playAtVolume(g.dropSound, b.Position, 0.5)
	}
}

// followPlayer eases the orbit camera's target towards the ball.
func (g *Game) followPlayer(dt float32) {
	if g.player.body == nil {
		return
	}
	target := g.player.body.Position.Add(mathx.Vec3{0, 0.3, 0})
	k := float32(1 - math.Exp(-followRate*float64(dt)))
	g.camera.orbit.Target = g.camera.orbit.Target.Add(target.Sub(g.camera.orbit.Target).Scale(k))
}

// bump makes a cube wobble after the player (or a ball) hits it.
func (g *Game) bump(id scene.ID) {
	g.bumpedAt[id] = g.world.Time()
}

// wobble is the cube behaviour that plays the bump animation: a quick
// squash-and-stretch that dies away over about half a second.
func (g *Game) wobble() scene.Behaviour {
	return func(w *scene.World, e *scene.Entity, dt float32) {
		at, ok := g.bumpedAt[e.ID()]
		if !ok {
			return
		}
		t := float64(w.Time() - at)
		if t > 0.8 {
			delete(g.bumpedAt, e.ID())
			e.Transform.Scale = mathx.Vec3{1, 1, 1}
			return
		}
		s := float32(0.25 * math.Exp(-7*t) * math.Sin(28*t))
		e.Transform.Scale = mathx.Vec3{1 + s, 1 - s, 1 + s}
	}
}
