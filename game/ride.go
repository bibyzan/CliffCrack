package game

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/game/course"
)

// Run-mode tuning. Speeds are m/s.
const (
	rideBallRadius = 0.5
	// Cruise speed: what the ride builds towards on the ground, rising from
	// 20 m/s (72 km/h) at the top to ~50 m/s (180 km/h) far down.
	rideCruiseBase  = 20.0
	rideCruiseGain  = 30.0
	rideTuckBonus   = 0.15  // W: cruise this much faster
	rideBrakeCut    = 0.6   // S: cruise this much slower...
	rideBrakeDrag   = 0.012 // ... and scrub speed with quadratic drag
	rideBoost       = 6.0   // m/s^2 at most of push towards the cruise speed
	rideOverDrag    = 0.02  // drag on speed above cruise (per m/s over, per m/s)
	rideSteer       = 17.0  // m/s^2 of sideways push on the ground
	rideAirSteer    = 5.0   // ... and in the air
	rideSkipTime    = 0.3   // seconds after touching down that still count as on the ground (skipping over moguls)
	rideJumpCool    = 0.35  // seconds before another jump: the ball can still touch the snow for a step after one
	rideJump        = 6.5   // m/s straight up
	rideStallSpeed  = 1.5   // slower than this for rideStallTime ends the run
	rideStallTime   = 2.5   // seconds
	rideCrackFall   = 4.0   // metres below the rim: fallen into a crack
	rideOffPiste    = 95.0  // metres from the centre line: lost on the mountain
	rideStartSpeed  = 7.0   // push off the cliff top
	rideBodiesAhead = 2     // chunks of obstacle colliders kept ahead of the ball
)

// rideInput is one frame of control. Steer is -1 (left) .. 1 (right) relative
// to the direction of travel; throttle is -1 (brake) .. 1 (tuck).
type rideInput struct {
	steer, throttle float32
	jump            bool
}

// rideEvents reports what happened during a step, for sounds and the camera.
type rideEvents struct {
	jumped  bool
	landed  float32 // impact speed of a landing, 0 if none
	crashed bool
}

// obstacleTag marks obstacle bodies (Body.UserData).
type obstacleTag struct{ kind course.ObstacleKind }

// ride is the Run mode's simulation: the ball on the generated course, the
// obstacle colliders near it, and the rules that end a run. It has no
// graphics, so tests can play whole runs headless.
type ride struct {
	course  *course.Course
	phys    *physics.World
	ball    *physics.Body
	bodies  map[int][]*physics.Body // obstacle colliders by chunk index
	chunkFn func(int) []course.Obstacle

	heading   mathx.Vec3 // smoothed horizontal direction of travel
	distance  float32    // furthest s reached
	time      float32
	stalled   float32 // seconds spent below rideStallSpeed
	airborne  float32 // seconds since the ball last touched the ground
	sinceJump float32 // seconds since the last jump
	landed    bool    // the ball has touched down after the drop
	crashed   bool
	cause     string
	crashPos  mathx.Vec3
	debris    []*physics.Body // the ball's pieces after a crash
	lane      float32         // autopilot: chosen offset from the centre line
}

// newRide starts a run on c. obstacles returns a chunk's obstacles (cached
// by the caller, which usually also builds the chunk's meshes).
func newRide(c *course.Course, obstacles func(index int) []course.Obstacle) *ride {
	r := &ride{
		course:    c,
		phys:      physics.NewWorld(),
		bodies:    map[int][]*physics.Body{},
		chunkFn:   obstacles,
		heading:   mathx.Vec3{0, 0, -1},
		sinceJump: rideJumpCool,
	}
	r.phys.AngularDamping = 0.15 // snow is fast: little rolling resistance
	r.phys.LinearDamping = 0     // air drag is applied by the ride itself
	terrain := physics.NewHeightfield(c.Height)
	terrain.Friction = 0.7
	terrain.Restitution = 0.05
	r.phys.Add(terrain)

	r.ball = physics.NewSphere(rideBallRadius, 2)
	r.ball.Friction = 0.9
	r.ball.Restitution = 0.15
	r.ball.Position = c.StartPosition()
	r.ball.Velocity = mathx.Vec3{0, 0, -rideStartSpeed}
	r.ball.AngularVelocity = mathx.Vec3{-rideStartSpeed / rideBallRadius, 0, 0}
	r.phys.Add(r.ball)
	r.syncBodies()
	return r
}

// Distance travelled down the course (s of the ball, never negative).
func (r *ride) s() float32 { return -r.ball.Position[2] }

func (r *ride) speed() float32 { return r.ball.Velocity.Len() }

// cruise is the speed the ride builds towards at the ball's distance.
func (r *ride) cruise() float32 {
	return rideCruiseBase + rideCruiseGain*course.Difficulty(r.s())
}

// pose is where to draw a body this frame (interpolated between physics steps).
func (r *ride) pose(b *physics.Body) (mathx.Vec3, mathx.Quat) {
	return b.Interpolated(r.phys.Alpha())
}

// syncBodies keeps obstacle colliders for the chunks around the ball only, so
// the physics never looks at the thousands of obstacles further down.
func (r *ride) syncBodies() {
	cur := course.ChunkAt(r.s())
	for index, bodies := range r.bodies {
		if index < cur-1 || index > cur+rideBodiesAhead {
			for _, b := range bodies {
				r.phys.Remove(b)
			}
			delete(r.bodies, index)
		}
	}
	for index := cur - 1; index <= cur+rideBodiesAhead; index++ {
		if _, ok := r.bodies[index]; ok {
			continue
		}
		var bodies []*physics.Body
		for _, o := range r.chunkFn(index) {
			b := &physics.Body{Kind: physics.Static, Shape: physics.Sphere, Radius: o.Radius,
				Position: o.Centre, Restitution: 0.4, Friction: 0.5, UserData: obstacleTag{o.Kind}}
			r.phys.Add(b)
			bodies = append(bodies, b)
		}
		r.bodies[index] = bodies
	}
}

// step advances the run by dt.
func (r *ride) step(dt float32, in rideInput) rideEvents {
	var ev rideEvents
	if dt <= 0 {
		return ev
	}
	r.time += dt
	if r.crashed {
		r.phys.Update(dt) // let the debris settle
		return ev
	}

	b := r.ball
	up := mathx.Vec3{0, 1, 0}
	v := b.Velocity
	flat := mathx.Vec3{v[0], 0, v[2]}
	if l := flat.Len(); l > 2 {
		k := float32(1 - math.Exp(-6*float64(dt)))
		r.heading = r.heading.Add(flat.Scale(1 / l).Sub(r.heading).Scale(k)).Normalize()
	}
	right := r.heading.Cross(up).Normalize()

	steer := clampf(in.steer, -1, 1)
	throttle := clampf(in.throttle, -1, 1)
	// Skipping across moguls leaves the ground for a few frames at a time;
	// that still counts as riding, not flying.
	onGround := r.landed && r.airborne < rideSkipTime
	push := float32(rideAirSteer)
	if onGround {
		push = rideSteer
	}
	b.Velocity = b.Velocity.Add(right.Scale(steer * push * dt))
	// Spin with the push so the ball visibly carves instead of sliding.
	b.AngularVelocity = b.AngularVelocity.Add(up.Cross(right).Scale(steer * push * dt / rideBallRadius))

	// Speed builds towards a cruise speed that rises the further you get:
	// on the ground the ride pushes you up to it, and drag only bites above it.
	cruise := r.cruise()
	drag := float32(0)
	switch {
	case throttle > 0:
		cruise *= 1 + rideTuckBonus*throttle
	case throttle < 0:
		cruise *= 1 - rideBrakeCut*-throttle
		drag = rideBrakeDrag * -throttle
	}
	speed := b.Velocity.Len()
	if onGround && speed < cruise {
		b.Velocity = b.Velocity.Add(r.heading.Scale(min(rideBoost, (cruise-speed)*0.8) * dt))
	}
	if speed > cruise {
		drag += rideOverDrag * (speed - cruise) / speed
	}
	b.Velocity = b.Velocity.Scale(1 / (1 + drag*speed*dt))

	r.sinceJump += dt
	if in.jump && onGround && r.sinceJump > rideJumpCool {
		r.sinceJump = 0
		b.Velocity[1] = max(b.Velocity[1], 0) + rideJump
		r.airborne = rideSkipTime // a jump leaves the ground: no second jump off the same contact
		ev.jumped = true
	}

	r.phys.Update(dt)
	r.syncBodies()
	r.distance = max(r.distance, r.s())

	for _, hit := range r.phys.Impacts() {
		if hit.A != b && hit.B != b {
			continue
		}
		other := hit.A
		if other == b {
			other = hit.B
		}
		if tag, ok := other.UserData.(obstacleTag); ok {
			what := "a rock"
			if tag.kind == course.Tree {
				what = "a tree"
			}
			r.crash("hit " + what)
			ev.crashed = true
			return ev
		}
		if hit.Speed > 3 {
			ev.landed = max(ev.landed, hit.Speed)
		}
	}
	if b.Grounded {
		r.landed = true
		r.airborne = 0
	} else {
		r.airborne += dt
	}

	p := b.Position
	switch {
	case p[1] < r.course.Rim(p[0], p[2])-rideCrackFall:
		r.crash("fell into a crack")
	case abs32(p[0]-r.course.Centre(r.s())) > rideOffPiste:
		r.crash("lost on the mountain")
	}
	if r.landed && b.Velocity.Len() < rideStallSpeed {
		r.stalled += dt
		if r.stalled > rideStallTime {
			r.crash("ran out of steam")
		}
	} else {
		r.stalled = 0
	}
	ev.crashed = r.crashed
	return ev
}

// crash ends the run: the ball shatters into pieces that tumble on.
func (r *ride) crash(cause string) {
	if r.crashed {
		return
	}
	r.crashed = true
	r.cause = cause
	b := r.ball
	r.crashPos = b.Position
	r.phys.Remove(b)
	rng := rand.New(rand.NewPCG(uint64(r.time*1000), 7))
	for i := 0; i < 14; i++ {
		dir := mathx.Vec3{rng.Float32()*2 - 1, rng.Float32() * 1.2, rng.Float32()*2 - 1}.Normalize()
		p := physics.NewSphere(0.1+0.08*rng.Float32(), 0.05)
		p.Position = b.Position.Add(dir.Scale(rideBallRadius * 0.6))
		p.Velocity = b.Velocity.Scale(0.35).Add(dir.Scale(3 + 4*rng.Float32()))
		p.AngularVelocity = mathx.Vec3{rng.Float32()*20 - 10, rng.Float32()*20 - 10, rng.Float32()*20 - 10}
		p.Restitution = 0.3
		r.phys.Add(p)
		r.debris = append(r.debris, p)
	}
}

func clampf(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
