package game

import (
	"fmt"
	"math"

	"vkgame/engine/geom"
	"vkgame/engine/mathx"
	"vkgame/engine/physics"
	"vkgame/engine/render"
	"vkgame/engine/scene"
)

// physLink ties a scene entity to a physics body.
//
// Dynamic bodies drive their entity's transform. Kinematic bodies follow their
// entity (which scripts and behaviours may animate) and push balls around.
type physLink struct {
	entity scene.ID
	body   *physics.Body
	size   mathx.Vec3 // unscaled half extents (box) or radius in [0] (sphere)

	// Kinematic only: the entity's position last frame. Velocity must come from
	// the entity's motion, not from the body, which the solver also advances.
	prevPos mathx.Vec3
	hasPrev bool
}

const fallLimit = -10 // balls below this height are removed

// setupPhysics creates the physics world with the ground as a static box.
func (g *Game) setupPhysics(groundHalfSize float32) error {
	g.phys = physics.NewWorld()
	ground := physics.NewBox(mathx.Vec3{groundHalfSize, 0.5, groundHalfSize}, physics.Static)
	ground.Position = mathx.Vec3{0, -0.5, 0} // top face at y = 0
	if err := g.phys.Add(ground); err != nil {
		return err
	}
	sphere, err := render.CreateMesh(geom.Sphere(1, 24, 12))
	if err != nil {
		return err
	}
	g.ballMesh = sphere
	return nil
}

// addWalls rings the ground with low static walls so balls stay in play.
func (g *Game) addWalls(mesh render.Mesh, tex render.Texture, groundHalfSize float32) error {
	const height, thick = 0.4, 0.3
	edge := groundHalfSize - thick/2
	walls := []struct{ pos, half mathx.Vec3 }{
		{mathx.Vec3{edge, height / 2, 0}, mathx.Vec3{thick / 2, height / 2, groundHalfSize}},
		{mathx.Vec3{-edge, height / 2, 0}, mathx.Vec3{thick / 2, height / 2, groundHalfSize}},
		{mathx.Vec3{0, height / 2, edge}, mathx.Vec3{groundHalfSize, height / 2, thick / 2}},
		{mathx.Vec3{0, height / 2, -edge}, mathx.Vec3{groundHalfSize, height / 2, thick / 2}},
	}
	for i, w := range walls {
		e := g.world.Spawn(fmt.Sprintf("wall%d", i), scene.ID{})
		e.Transform.Position = w.pos
		e.Transform.Scale = w.half.Scale(2) // the cube mesh is 1 unit across
		e.Renderable = &scene.Renderable{Mesh: mesh, Texture: tex, Color: mathx.Hex(0x9aa3ad)}

		b := physics.NewBox(w.half, physics.Static)
		b.Position = w.pos
		if err := g.phys.Add(b); err != nil {
			return err
		}
	}
	return nil
}

// attachKinematic makes an entity a collider that follows the entity's motion.
func (g *Game) attachKinematic(e *scene.Entity, shape physics.Shape, size mathx.Vec3) {
	var b *physics.Body
	if shape == physics.Box {
		b = physics.NewBox(size, physics.Kinematic)
	} else {
		b = &physics.Body{Kind: physics.Kinematic, Shape: physics.Sphere, Radius: size[0],
			Restitution: 0.5, Friction: 0.4}
	}
	b.UserData = e.ID()
	if err := g.phys.Add(b); err != nil {
		panic(err) // kinematic bodies are always valid
	}
	g.links = append(g.links, physLink{entity: e.ID(), body: b, size: size})
}

// dropBall spawns a randomly sized and coloured ball above the scene.
func (g *Game) dropBall() {
	radius := 0.2 + g.rng.Float32()*0.25
	angle := g.rng.Float64() * 2 * math.Pi
	dist := 1 + g.rng.Float64()*4.5

	e := g.world.Spawn(fmt.Sprintf("ball%d", len(g.balls)), scene.ID{})
	e.Transform.Scale = mathx.Vec3{radius, radius, radius}
	e.Renderable = &scene.Renderable{
		Mesh:  g.ballMesh,
		Color: mathx.SRGB(0.3+0.7*g.rng.Float32(), 0.3+0.7*g.rng.Float32(), 0.3+0.7*g.rng.Float32(), 1),
	}

	b := physics.NewSphere(radius, 4*radius*radius*radius) // mass ~ volume
	b.Position = mathx.Vec3{float32(math.Cos(angle) * dist), 3 + 2*g.rng.Float32(), float32(math.Sin(angle) * dist)}
	b.Restitution = 0.25 + 0.3*g.rng.Float32()
	b.UserData = e.ID()
	if err := g.phys.Add(b); err != nil {
		panic(err)
	}
	e.Transform.Position = b.Position
	g.links = append(g.links, physLink{entity: e.ID(), body: b, size: mathx.Vec3{radius}})
	g.balls = append(g.balls, e.ID())
}

// clearBalls removes every dropped ball.
func (g *Game) clearBalls() {
	for _, id := range g.balls {
		g.world.Destroy(id)
	}
	g.balls = g.balls[:0]
	g.pruneLinks()
}

// stepPhysics syncs kinematic colliders from the scene, advances the
// simulation, writes dynamic bodies back and plays impact sounds.
func (g *Game) stepPhysics(dt float32) {
	for i := range g.links {
		l := &g.links[i]
		if l.body.Kind != physics.Kinematic {
			continue
		}
		e := g.world.Get(l.entity)
		if e == nil {
			continue
		}
		pos, rot, scale := mathx.Decompose(e.WorldMatrix())
		l.body.Velocity = mathx.Vec3{}
		if l.hasPrev && dt > 0 {
			l.body.Velocity = pos.Sub(l.prevPos).Scale(1 / dt)
		}
		l.prevPos, l.hasPrev = pos, true
		l.body.Position, l.body.Rotation = pos, rot
		if l.body.Shape == physics.Box {
			l.body.HalfExtents = mathx.Vec3{l.size[0] * scale[0], l.size[1] * scale[1], l.size[2] * scale[2]}
		} else {
			l.body.Radius = l.size[0] * max(scale[0], scale[1], scale[2])
		}
	}

	g.phys.Update(dt)

	fallen := false
	for _, l := range g.links {
		if l.body.Kind != physics.Dynamic {
			continue
		}
		if e := g.world.Get(l.entity); e != nil {
			e.Transform.Position = l.body.Position
			e.Transform.Rotation = l.body.Rotation
			if l.body.Position[1] < fallLimit && l.entity != g.player.entity { // the player respawns instead
				g.world.Destroy(l.entity)
				fallen = true
			}
		}
	}
	if fallen {
		g.pruneLinks()
	}
	g.world.UpdateTransforms()

	played := 0
	for _, hit := range g.phys.Impacts() {
		if cube, ok := g.bumpedCube(hit); ok && hit.Speed >= bumpSpeed {
			g.bump(cube)
			g.playAtVolume(g.bonkSound, hit.Point, min(1, 0.3+hit.Speed/6))
			played++
			continue
		}
		if hit.Speed < 1.5 || played >= 4 { // ignore taps; cap per frame
			continue
		}
		g.playAtVolume(g.bounceSound, hit.Point, min(1, hit.Speed/8))
		played++
	}
}

// bumpedCube reports which ring cube (if any) took part in an impact.
func (g *Game) bumpedCube(hit physics.Impact) (scene.ID, bool) {
	for _, b := range [2]*physics.Body{hit.A, hit.B} {
		if id, ok := b.UserData.(scene.ID); ok && g.cubes[id] {
			return id, true
		}
	}
	return scene.ID{}, false
}

// pruneLinks drops physics bodies whose entities no longer exist.
func (g *Game) pruneLinks() {
	alive := g.links[:0]
	for _, l := range g.links {
		if g.world.Get(l.entity) != nil {
			alive = append(alive, l)
		} else {
			g.phys.Remove(l.body)
		}
	}
	g.links = alive
	balls := g.balls[:0]
	for _, id := range g.balls {
		if g.world.Get(id) != nil {
			balls = append(balls, id)
		}
	}
	g.balls = balls
}
