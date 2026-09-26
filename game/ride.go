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
	// 36 m/s (130 km/h) at the top to ~76 m/s (274 km/h) far down.
	rideCruiseBase = 36.0
	rideCruiseGain = 40.0
	rideTuckBonus  = 0.3   // W: cruise this much faster...
	rideTuckBoost  = 0.8   // ... and push up to it this much harder
	rideBrakeCut   = 0.6   // S: cruise this much slower...
	rideBrakeDrag  = 0.012 // ... and scrub speed with quadratic drag
	rideBoost      = 12.0  // m/s^2 at most of push towards the cruise speed
	rideOverDrag   = 0.01  // drag on speed above cruise (per m/s over, per m/s), on the level
	rideAirHold    = 0.3   // share of it in the air
	// Down a steep face the ball is pulled along the slope harder than
	// rolling alone would (a rolling ball only gets ~70% of gravity's pull,
	// the rest spins it up): up to this share of gravity along the slope on
	// the Drop's face, less the gentler the slope, none on the flat.
	rideSteepPull = 0.6
	// Off the snow, extra gravity keeps the ball planted: at these speeds
	// every bump would otherwise throw it tens of metres, where it neither
	// steers nor builds speed.
	rideAirPull = 9.81 // m/s^2 down, on top of gravity
	// Grip: near the snow, speed away from it (off a mogul's crest, a roll)
	// is damped at this rate (1/s), so the ball hugs the surface at speed
	// instead of skipping. Not after a jump, nor on a crack's kicker.
	rideStick      = 10.0
	rideSteer      = 17.0 // m/s^2 of sideways push on the ground
	rideAirSteer   = 5.0  // ... and in the air
	rideSkipTime   = 0.3  // seconds after touching down that still count as on the ground (skipping over moguls)
	rideJumpCool   = 0.35 // seconds before another jump: the ball can still touch the snow for a step after one
	rideJump       = 7.2  // m/s off the snow, square to the slope (on the level; see the jump)
	rideJumpFlight = 1.5  // seconds after a jump with no extra air pull: the jump is the player's
	rideStallSpeed = 1.5  // slower than this for rideStallTime ends the run
	rideStallTime  = 2.5  // seconds
	rideCrackFall  = 4.0  // metres below the rim: fallen into a crack
	rideEdgeFall   = 14.0 // metres below the path: fallen off the ridge (or the mountain)
	rideOffPiste   = 95.0 // metres from the path's centre line: lost on the mountain

	// Up on the valley's banks the snow slides you back towards the path:
	// past rideBankFree metres beyond the channel's edge, a push grows with
	// distance up to rideBankMax m/s^2 (and snowballs start rolling at you).
	rideBankFree = 3.0
	rideBankPush = 0.7 // m/s^2 per metre further out
	rideBankMax  = 16.0

	// Snowballs roll down the banks at a ball that stays up there.
	snowballAfter = 0.4  // seconds up on a bank before the first one
	snowballEvery = 0.6  // seconds between them
	snowballMax   = 7    // alive at once
	snowballSpeed = 16.0 // m/s down the bank
	// In the narrows they tumble off the gorge walls at a ball that climbs
	// one: sooner and more often, the walls being quick to climb.
	snowballWall      = 1.5  // metres up a wall before they come
	snowballWallAfter = 0.1  // seconds up it before the first
	snowballWallEvery = 0.25 // seconds between them
	snowballLife      = 14.0 // seconds before one melts away
	// The ride starts already at the Drop's terminal speed, where the drag
	// above cruise balances gravity down its face.
	rideStartSpeed  = 52.0
	rideBodiesAhead = 2 // chunks of obstacle colliders kept ahead of the ball
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
	picked  []course.PowerKind // power-ups collected
	smashed []int              // chunks an obstacle was smashed out of (with the shield up)
}

// obstacleTag marks obstacle bodies (Body.UserData).
type obstacleTag struct {
	kind course.ObstacleKind
	id   obstacleID
}

// snowball is a big ball of snow rolling down a bank. It knocks the player
// about but, unlike a rock, doesn't end the run.
type snowball struct {
	body *physics.Body
	age  float32
	hit  bool // it has struck the ball
}

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

	snowballs []*snowball
	onBank    float32 // seconds the ball has been up on a bank
	nextBall  float32 // seconds until the next snowball may roll
	rng       *rand.Rand

	powers
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
	r.phys.SupportY = 0.25       // the Drop's face (~70 degrees) is ground to ride, not a wall
	terrain := physics.NewHeightfield(c.Height)
	terrain.Friction = 0.7
	terrain.Restitution = 0.05
	r.phys.Add(terrain)

	r.ball = physics.NewSphere(rideBallRadius, 2)
	r.ball.Friction = 0.9
	r.ball.Restitution = 0.15
	r.ball.Position = c.StartPosition()
	// Down the face, not off it.
	g := course.GradeAt(1)
	r.ball.Velocity = mathx.Vec3{0, -g, -1}.Normalize().Scale(rideStartSpeed)
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
		for i, o := range r.chunkFn(index) {
			id := obstacleID{index, i}
			if r.smashed[id] {
				continue
			}
			b := &physics.Body{Kind: physics.Static, Shape: physics.Sphere, Radius: o.Radius,
				Position: o.Centre, Restitution: 0.4, Friction: 0.5, UserData: obstacleTag{o.Kind, id}}
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
	if r.boost > 0 {
		cruise *= rideBoostCruise
	}
	drag := float32(0)
	switch {
	case throttle > 0:
		cruise *= 1 + rideTuckBonus*throttle
	case throttle < 0:
		cruise *= 1 - rideBrakeCut*-throttle
		drag = rideBrakeDrag * -throttle
	}
	// On (or skimming just off) the snow, steepness pulls the ball on.
	at := b.Position
	n := physics.TerrainNormal(r.course.Height, at[0], at[2], 0.5)
	onSnow := (at[1]-r.course.Height(at[0], at[2]))*n[1]-rideBallRadius < 1
	if onSnow {
		g := mathx.Vec3{0, -9.81, 0}
		down := g.Sub(n.Scale(g.Dot(n))) // gravity along the slope
		steep := clampf((1-n[1])/0.66, 0, 1)
		b.Velocity = b.Velocity.Add(down.Scale(rideSteepPull * steep * dt))
	} else {
		if r.sinceJump > rideJumpFlight {
			b.Velocity[1] -= rideAirPull * dt
		}
	}
	speed := b.Velocity.Len()
	if (onGround || r.boost > 0) && speed < cruise {
		boost := float32(rideBoost) * (1 + rideTuckBoost*max(throttle, 0))
		if r.boost > 0 {
			boost = rideBoostPush
		}
		b.Velocity = b.Velocity.Add(r.heading.Scale(min(boost, (cruise-speed)*0.8) * dt))
	}
	if speed > cruise {
		// Above cruise the snow holds the ball back, less the steeper it is
		// (on a sheer face gravity wins: the Drop keeps getting faster), and
		// the air only a little.
		// (Down a steep face the ball skims, just off the snow as often as
		// on it: near enough counts.)
		hold := float32(rideAirHold)
		if onSnow {
			hold = n[1] * n[1]
		}
		drag += rideOverDrag * hold * (speed - cruise) / speed
	}
	b.Velocity = b.Velocity.Scale(1 / (1 + drag*speed*dt))

	r.sinceJump += dt
	if onGround {
		r.bankPush(dt)
	}

	if in.jump && onGround && r.sinceJump > rideJumpCool {
		r.sinceJump = 0
		// Off the slope, not straight up: the ball keeps its speed down the
		// hill (a jump used to cancel it, which down a steep face was a leap
		// far out over the snow) and pops clear of the surface it's on: a
		// few metres in the valley, a bit less down the Drop. The push
		// shrinks with the slope's steepness as gravity's pull back onto it
		// does (down a steep face the hang is longer).
		n := physics.TerrainNormal(r.course.Height, b.Position[0], b.Position[2], 0.5)
		if into := b.Velocity.Dot(n); into < 0 {
			b.Velocity = b.Velocity.Sub(n.Scale(into))
		}
		b.Velocity = b.Velocity.Add(n.Scale(rideJump * float32(math.Sqrt(float64(n[1])))))
		r.airborne = rideSkipTime // a jump leaves the ground: no second jump off the same contact
		ev.jumped = true
	}

	before := b.Velocity // (a smash puts most of it back)
	r.phys.Update(dt)
	r.syncBodies()
	r.distance = max(r.distance, r.s())
	r.updatePowers(dt)

	for _, hit := range r.phys.Impacts() {
		if hit.A != b && hit.B != b {
			continue
		}
		other := hit.A
		if other == b {
			other = hit.B
		}
		if tag, ok := other.UserData.(obstacleTag); ok && r.shield > 0 {
			r.smash(other, tag, before, &ev)
			continue
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
		if sb, ok := other.UserData.(*snowball); ok {
			sb.hit = true
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
	s := r.s()
	pathX := r.course.PathCentre(s)
	switch {
	case p[1] < r.course.Rim(p[0], p[2])-rideCrackFall:
		r.crash("fell into a crack")
	case p[1] < r.course.Rim(pathX, p[2])-rideEdgeFall:
		if k, ok := r.course.SectionAt(s); ok && k.Kind == course.Ridge {
			r.crash("fell off the ridge")
		} else {
			r.crash("fell off the mountain")
		}
	case abs32(p[0]-pathX) > rideOffPiste:
		r.crash("lost on the mountain")
	}
	if !r.crashed {
		r.updateSnowballs(dt)
		r.collectPowerUps(&ev)
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

// placeAt puts the ball on the path at distance s (u across from its centre
// line), already rolling down it at speed: for starting partway down.
func (r *ride) placeAt(s, u, speed float32) {
	x := r.course.PathCentre(s) + u
	r.ball.Position = mathx.Vec3{x, r.course.Height(x, -s) + rideBallRadius + 0.05, -s}
	grade := r.course.Height(x, -s) - r.course.Height(x, -s-1)
	r.ball.Velocity = mathx.Vec3{0, -grade, -1}.Normalize().Scale(speed) // down the slope, not off it
	r.ball.AngularVelocity = mathx.Vec3{-speed / rideBallRadius, 0, 0}
	r.ball.Teleported()
	r.landed = true
	r.distance = s
	r.syncBodies()
}

// bankOut is how far past the valley channel's edge the ball is (metres,
// positive up on a bank) and which side it's on (+1 for +x).
func (r *ride) bankOut() (out, side float32) {
	s := r.s()
	u := r.ball.Position[0] - r.course.Centre(s)
	side = 1
	if u < 0 {
		side = -1
	}
	return abs32(u) - r.course.HalfWidth(s), side
}

// bankPush slides a ball that's up on the valley's banks back towards the
// path, harder the further out it is, so riding the banks can't last. In
// sections, whose own walls and edges shape the ride, it fades out.
func (r *ride) bankPush(dt float32) {
	out, side := r.bankOut()
	weight := r.course.ValleyWeight(r.s())
	if out <= rideBankFree || weight <= 0 {
		return
	}
	push := min(rideBankMax, (out-rideBankFree)*rideBankPush+3) * weight
	r.ball.Velocity[0] -= side * push * dt
}

// updateSnowballs rolls snowballs down the bank at a ball that stays up
// there, ages them and clears away old ones.
func (r *ride) updateSnowballs(dt float32) {
	out, side := r.bankOut()
	up := out > rideBankFree+3 && r.course.ValleyWeight(r.s()) > 0.99
	wall, wallSide, narrows := r.wallOut()
	if narrows && wall > snowballWall {
		up, out, side = true, wall, wallSide
	}
	if up {
		r.onBank += dt
	} else {
		r.onBank = 0
	}
	r.nextBall -= dt
	after, every := float32(snowballAfter), float32(snowballEvery)
	if narrows {
		after, every = snowballWallAfter, snowballWallEvery
	}
	if r.onBank > after && r.nextBall <= 0 && len(r.snowballs) < snowballMax {
		r.spawnSnowball(side, narrows, out)
		r.nextBall = every
	}
	alive := r.snowballs[:0]
	for _, sb := range r.snowballs {
		sb.age += dt
		p := sb.body.Position
		behind := p[2] - r.ball.Position[2] // metres behind the ball (the run heads to -z)
		if sb.age > snowballLife || behind > 60 || p[1] < r.course.Rim(p[0], p[2])-10 {
			r.phys.Remove(sb.body)
			continue
		}
		alive = append(alive, sb)
	}
	r.snowballs = alive
}

// wallOut is, in the narrows, how far the ball has climbed past the gorge
// floor's edge (metres, positive up a wall) and which wall (+1 for +x).
func (r *ride) wallOut() (out, side float32, narrows bool) {
	s := r.s()
	if k, ok := r.course.SectionAt(s); !ok || k.Kind != course.Narrows {
		return 0, 0, false
	}
	u := r.ball.Position[0] - r.course.PathCentre(s)
	side = 1
	if u < 0 {
		side = -1
	}
	return abs32(u) - r.course.PathHalfWidth(s), side, true
}

// spawnSnowball starts a snowball further up the bank (or, in the narrows,
// the gorge wall) the ball is on, out metres past the floor's edge, and a
// little ahead, already rolling down across the ball's line.
func (r *ride) spawnSnowball(side float32, narrows bool, out float32) {
	if r.rng == nil {
		r.rng = rand.New(rand.NewPCG(r.course.Seed, 0x5a0b))
	}
	// Timed to cross the ball's line about 1 s later: it starts ~16 m up
	// the bank, rolling down at snowballSpeed while drifting downhill at half
	// the ball's speed, so it starts half a second of the ball's speed ahead.
	b := r.ball
	ahead := 6 + 0.5*b.Velocity.Len() + 6*r.rng.Float32()
	s := r.s() + ahead
	x := b.Position[0] + side*(13+5*r.rng.Float32())
	radius := 1.6 + 0.7*r.rng.Float32()
	vel := mathx.Vec3{-side * snowballSpeed * (0.85 + 0.3*r.rng.Float32()), 0, b.Velocity[2] * 0.5}
	if narrows {
		// A gorge wall is too steep (sheer in places) to roll one down from
		// far off: it would drop off a ledge and bounce clear. It tumbles
		// off the wall just above the ball instead, keeping pace with it,
		// and falls in on it, shoving it back onto the floor. Smaller, so
		// the floor isn't blocked once it's down.
		radius = 1.1 + 0.5*r.rng.Float32()
		z := b.Position[2] - 0.5*r.rng.Float32()
		vel = b.Velocity.Add(mathx.Vec3{-side * snowballSpeed * 0.6, -3, 0})
		// Beside and a little above the ball, stepped in off the wall until
		// clear of it.
		y := b.Position[1] + 0.6*radius + 0.3
		for off := radius + 1.2; off > 0; off -= 0.25 {
			x = b.Position[0] + side*off
			if r.course.Height(x, z)+radius+0.2 < y {
				break
			}
		}
		body := physics.NewSphere(radius, 40*radius)
		body.Position, body.Velocity = mathx.Vec3{x, y, z}, vel
		r.addSnowball(body)
		return
	}
	body := physics.NewSphere(radius, 40*radius) // heavy: it shoves the ball
	// Clear of the ground: on a steep face a sphere touching it sits much
	// higher over the point below its centre than its radius.
	grade := abs32(r.course.Height(x+0.5, -s)-r.course.Height(x-0.5, -s)) + abs32(r.course.Height(x, -s-0.5)-r.course.Height(x, -s+0.5))
	lift := radius*float32(math.Sqrt(float64(1+grade*grade))) + 0.3
	body.Position = mathx.Vec3{x, r.course.Height(x, -s) + lift, -s}
	body.Velocity = vel
	r.addSnowball(body)
}

func (r *ride) addSnowball(body *physics.Body) {
	body.Restitution = 0.2
	body.Friction = 0.8
	sb := &snowball{body: body}
	body.UserData = sb
	r.phys.Add(body)
	r.snowballs = append(r.snowballs, sb)
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
