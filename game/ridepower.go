package game

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/game/course"
)

// Power-ups: the ball picks one up by rolling through it.
const (
	powerReach = 1.7 // metres from a pickup's centre to the ball's that collects it

	// Boost: a kick, then a higher cruise pushed hard, on the ground or not.
	rideBoostTime   = 3.5  // seconds
	rideBoostKick   = 10.0 // m/s added at once, along the direction of travel
	rideBoostCruise = 1.45 // cruise speed multiplier while it lasts
	rideBoostPush   = 30.0 // m/s^2 at most towards that cruise

	// Shield: rocks and trees shatter instead of ending the run.
	rideShieldTime = 8.0  // seconds
	rideSmashKeep  = 0.92 // share of its speed the ball keeps through a smash
	smashShards    = 7    // pieces a smashed obstacle breaks into
	shardLife      = 2.5  // seconds before they're cleared away
)

// obstacleID names one obstacle: its chunk and its place in the chunk's list.
type obstacleID struct{ chunk, i int }

// powerID names one power-up the same way.
type powerID struct{ chunk, i int }

// powers is a ride's power-up state.
type powers struct {
	boost, shield float32 // seconds of each left
	taken         map[powerID]bool
	pickups       map[int][]course.PowerUp // by chunk, generated once
	smashed       map[obstacleID]bool
	shards        []*shard
}

// shard is a piece of a smashed obstacle.
type shard struct {
	body *physics.Body
	kind course.ObstacleKind
	age  float32
}

// powerUps returns chunk index's pickups (taken ones included: see taken),
// generating them once, and forgetting chunks left well behind.
func (r *ride) powerUps(index int) []course.PowerUp {
	if p, ok := r.pickups[index]; ok {
		return p
	}
	if r.pickups == nil {
		r.pickups = map[int][]course.PowerUp{}
	}
	cur := course.ChunkAt(r.s())
	for i := range r.pickups {
		if i < cur-2 {
			delete(r.pickups, i)
		}
	}
	p := r.course.PowerUps(index)
	r.pickups[index] = p
	return p
}

// collectPowerUps takes any pickup the ball has rolled into.
func (r *ride) collectPowerUps(ev *rideEvents) {
	cur := course.ChunkAt(r.s())
	for index := cur - 1; index <= cur+1; index++ {
		for i, p := range r.powerUps(index) {
			id := powerID{index, i}
			if r.taken[id] || p.Pos.Sub(r.ball.Position).Len() > powerReach {
				continue
			}
			if r.taken == nil {
				r.taken = map[powerID]bool{}
			}
			r.taken[id] = true
			ev.picked = append(ev.picked, p.Kind)
			switch p.Kind {
			case course.Boost:
				r.boost = rideBoostTime
				r.ball.Velocity = r.ball.Velocity.Add(r.ball.Velocity.Normalize().Scale(rideBoostKick))
			case course.Shield:
				r.shield = rideShieldTime
			}
		}
	}
}

// smash shatters obstacle body o, hit with the shield up: it's gone, in
// pieces flying on, and the ball carries on at most of the speed it had.
func (r *ride) smash(o *physics.Body, tag obstacleTag, before mathx.Vec3, ev *rideEvents) {
	if r.smashed == nil {
		r.smashed = map[obstacleID]bool{}
	}
	r.smashed[tag.id] = true
	r.phys.Remove(o)
	bodies := r.bodies[tag.id.chunk]
	for i, b := range bodies {
		if b == o {
			r.bodies[tag.id.chunk] = append(bodies[:i:i], bodies[i+1:]...)
			break
		}
	}
	r.ball.Velocity = before.Scale(rideSmashKeep)
	ev.smashed = append(ev.smashed, tag.id.chunk)

	if r.rng == nil {
		r.rng = rand.New(rand.NewPCG(r.course.Seed, 0x5a0b))
	}
	for range smashShards {
		a := r.rng.Float64() * 2 * math.Pi
		out := mathx.Vec3{float32(math.Cos(a)), 0.6 + r.rng.Float32(), float32(math.Sin(a))}.Scale(4 + 6*r.rng.Float32())
		size := o.Radius * (0.2 + 0.2*r.rng.Float32())
		body := physics.NewSphere(size, 5*size)
		body.Position = o.Position.Add(out.Normalize().Scale(o.Radius * 0.5))
		body.Velocity = before.Scale(0.6).Add(out)
		body.AngularVelocity = mathx.Vec3{r.rng.Float32()*20 - 10, r.rng.Float32()*20 - 10, r.rng.Float32()*20 - 10}
		body.Ignore = r.ball // they fly off; they don't knock the ball about
		r.phys.Add(body)
		r.shards = append(r.shards, &shard{body: body, kind: tag.kind})
	}
}

// updatePowers runs the timers down and clears away old shards.
func (r *ride) updatePowers(dt float32) {
	r.boost = max(r.boost-dt, 0)
	r.shield = max(r.shield-dt, 0)
	alive := r.shards[:0]
	for _, s := range r.shards {
		if s.age += dt; s.age > shardLife {
			r.phys.Remove(s.body)
			continue
		}
		alive = append(alive, s)
	}
	r.shards = alive
}
