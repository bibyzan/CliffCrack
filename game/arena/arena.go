package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Movement and weapon tuning.
const (
	PlayerRadius = 0.4
	EyeHeight    = 1.25 // eye above the body's centre (~1.65 m above the floor)
	walkSpeed    = 6.5  // m/s
	sprintSpeed  = 9.5
	groundAccel  = 14.0 // 1/s: how quickly velocity reaches the target on the ground
	airAccel     = 2.5  // ... and in the air
	jumpSpeed    = 6.2  // m/s; with gravity 15 the apex is ~1.3 m
	jumpCooldown = 0.25 // s: the feet still touch the floor for a step after take-off
	gravity      = 15.0 // snappier than 9.81 for a shooter

	MagSize      = 30
	fireInterval = 0.1 // s between shots (600 rpm)
	ReloadTime   = 1.5 // s
	maxRange     = 200
	baseSpread   = 0.002 // radians of cone half-angle standing still
	moveSpread   = 0.012 // extra while moving
	bloomPerShot = 0.004 // extra per shot in a burst, decays quickly
	recoilKick   = 0.012 // radians of pitch per shot
	recoilReturn = 4.0   // 1/s: sustained fire climbs ~2 degrees, then settles

	DroneRadius   = 0.45
	DroneHealth   = 3
	respawnDelay  = 3.0 // s
	KillScore     = 100
	debrisPerKill = 10
	debrisLife    = 4.0 // s
)

// Input is one frame of player intent, already mapped from keys/pad.
type Input struct {
	Move        [2]float32 // x: strafe right, y: forward; length <= 1
	Look        [2]float32 // radians this frame: yaw right, pitch up
	Jump        bool       // pressed this frame
	Sprint      bool
	Fire        bool // trigger held
	FirePressed bool // trigger went down this frame (for the empty click)
	Reload      bool // pressed this frame
}

// Player is the first-person character: a fixed-rotation sphere at the feet
// and a view on top.
type Player struct {
	Body       *physics.Body
	Yaw, Pitch float32
	recoil     float32 // pitch added by recoil, recovering over time
	onGround   bool
	sinceJump  float32
}

// Eye is the camera position. alpha interpolates between physics steps.
func (p *Player) Eye(alpha float32) mathx.Vec3 {
	pos, _ := p.Body.Interpolated(alpha)
	return pos.Add(mathx.Vec3{0, EyeHeight, 0})
}

// ViewPitch includes the recoil kick.
func (p *Player) ViewPitch() float32 {
	return clamp(p.Pitch+p.recoil, -camera.MaxPitch, camera.MaxPitch)
}

// Forward is the view direction.
func (p *Player) Forward() mathx.Vec3 { return camera.Direction(p.Yaw, p.ViewPitch()) }

// OnGround reports whether the player is standing on something.
func (p *Player) OnGround() bool { return p.onGround }

// Weapon is the rifle.
type Weapon struct {
	Ammo      int
	Reloading float32 // seconds left, 0 when ready
	cooldown  float32
	bloom     float32
	Kick      float32 // 0..1 visual recoil for the gun model, decays fast
}

// Patrol is a drone's flight path: a circle (or ellipse) with a vertical bob.
type Patrol struct {
	Centre        mathx.Vec3
	RadiusX       float32
	RadiusZ       float32
	Speed         float32 // radians per second around the loop
	Bob, BobSpeed float32
	Phase         float32
}

// At is the position on the path at time t, and its velocity.
func (p Patrol) At(t float32) (pos, vel mathx.Vec3) {
	a := float64(p.Phase + p.Speed*t)
	b := float64(p.Phase*1.7 + p.BobSpeed*t)
	pos = p.Centre.Add(mathx.Vec3{
		p.RadiusX * float32(math.Cos(a)),
		p.Bob * float32(math.Sin(b)),
		p.RadiusZ * float32(math.Sin(a)),
	})
	vel = mathx.Vec3{
		-p.RadiusX * p.Speed * float32(math.Sin(a)),
		p.Bob * p.BobSpeed * float32(math.Cos(b)),
		p.RadiusZ * p.Speed * float32(math.Cos(a)),
	}
	return pos, vel
}

// Drone is a hovering target. It's a kinematic sphere while alive.
type Drone struct {
	Body    *physics.Body
	Patrol  Patrol
	Health  int
	Dead    bool
	respawn float32 // seconds until it comes back
	Flash   float32 // 0..1 hit flash, decays fast
}

// Debris is a piece of a destroyed drone.
type Debris struct {
	Body *physics.Body
	Age  float32
}

// Shot is one bullet: from the eye to where it stopped.
type Shot struct {
	From, To mathx.Vec3
	Normal   mathx.Vec3 // surface normal where it stopped (zero if it hit nothing)
	Drone    *Drone     // the drone it hit, if any
}

// Events are what happened during a step, for sounds and effects.
type Events struct {
	Shots    []Shot
	Kills    []*Drone
	Jumped   bool
	Landed   float32 // impact speed of a landing, 0 if none
	Reloaded bool    // a reload started
	Empty    bool    // the trigger was pulled on an empty magazine
}

// Arena is the whole match state.
type Arena struct {
	Phys   *physics.World
	Level  []Block
	Player Player
	Weapon Weapon
	Drones []*Drone
	Debris []*Debris

	Time         float32
	Score        int
	Kills        int
	ShotsFired   int
	ShotsHit     int
	InfiniteAmmo bool // debug

	rng *rand.Rand
}

// Spawn is where the player starts, looking north (-Z) at the platform.
var Spawn = mathx.Vec3{0, PlayerRadius + 0.02, 20}

// New builds the arena with its drones. seed picks the drones' patrols.
func New(seed uint64) *Arena {
	a := &Arena{
		Phys:  physics.NewWorld(),
		Level: Level(),
		rng:   rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}
	a.Phys.Gravity = mathx.Vec3{0, -gravity, 0}
	addLevel(a.Phys, a.Level)

	b := physics.NewSphere(PlayerRadius, 80)
	b.FixedRotation = true
	b.Friction = 1
	b.Restitution = 0
	b.Position = Spawn
	b.UserData = &a.Player
	if err := a.Phys.Add(b); err != nil {
		panic(err)
	}
	a.Player = Player{Body: b}
	a.Weapon = Weapon{Ammo: MagSize}

	for range 8 {
		d := &Drone{Health: DroneHealth, Patrol: a.randomPatrol()}
		d.Body = &physics.Body{Kind: physics.Kinematic, Shape: physics.Sphere, Radius: DroneRadius,
			Rotation: mathx.QuatIdentity(), Restitution: 0.6, Friction: 0.2, UserData: d}
		d.place(0)
		a.Phys.Add(d.Body)
		a.Drones = append(a.Drones, d)
	}
	return a
}

// randomPatrol picks a loop inside the arena, clear of the walls and above head height.
func (a *Arena) randomPatrol() Patrol {
	r := a.rng
	rx := 2 + r.Float32()*6
	rz := 2 + r.Float32()*6
	cx := (r.Float32()*2 - 1) * (HalfSize - 2 - rx)
	cz := (r.Float32()*2 - 1) * (HalfSize - 2 - rz)
	speed := (0.25 + r.Float32()*0.4) * float32(1-2*r.IntN(2))
	return Patrol{
		Centre:   mathx.Vec3{cx, 3.5 + r.Float32()*3, cz},
		RadiusX:  rx,
		RadiusZ:  rz,
		Speed:    speed,
		Bob:      0.3 + r.Float32()*0.6,
		BobSpeed: 1 + r.Float32()*1.5,
		Phase:    r.Float32() * 2 * math.Pi,
	}
}

func (d *Drone) place(t float32) {
	pos, vel := d.Patrol.At(t)
	d.Body.Position, d.Body.Velocity = pos, vel
}

// Step advances the match by dt.
func (a *Arena) Step(dt float32, in Input) Events {
	var ev Events
	a.Time += dt
	p := &a.Player

	// Look (per frame, not per physics step) and recoil recovery.
	p.Yaw += in.Look[0]
	p.Pitch = clamp(p.Pitch+in.Look[1], -camera.MaxPitch, camera.MaxPitch)
	p.recoil *= float32(math.Exp(-recoilReturn * float64(dt)))

	a.moveDrones(dt)
	a.movePlayer(dt, in, &ev)
	a.updateWeapon(dt, in, &ev)

	wasGround := p.onGround
	fallSpeed := -p.Body.Velocity[1]
	a.Phys.Update(dt)
	p.onGround = p.Body.Grounded
	if p.onGround && !wasGround && fallSpeed > 2 {
		ev.Landed = fallSpeed
	}

	a.ageDebris(dt)
	return ev
}

func (a *Arena) movePlayer(dt float32, in Input, ev *Events) {
	p := &a.Player
	forward := camera.Direction(p.Yaw, 0)
	right := forward.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	wish := forward.Scale(in.Move[1]).Add(right.Scale(in.Move[0]))
	if l := wish.Len(); l > 1 {
		wish = wish.Scale(1 / l)
	}
	speed := float32(walkSpeed)
	if in.Sprint && in.Move[1] > 0.3 {
		speed = sprintSpeed
	}
	rate := float32(airAccel)
	if p.onGround {
		rate = groundAccel
	}
	v := p.Body.Velocity
	flat := mathx.Vec3{v[0], 0, v[2]}
	flat = flat.Add(wish.Scale(speed).Sub(flat).Scale(1 - float32(math.Exp(-float64(rate*dt)))))
	p.Body.Velocity = mathx.Vec3{flat[0], v[1], flat[2]}

	p.sinceJump += dt
	if in.Jump && p.onGround && p.sinceJump >= jumpCooldown {
		p.Body.Velocity[1] = jumpSpeed
		p.onGround = false
		p.sinceJump = 0
		ev.Jumped = true
	}
}

func (a *Arena) updateWeapon(dt float32, in Input, ev *Events) {
	w := &a.Weapon
	p := &a.Player
	// The cooldown may go negative while the trigger is held, so leftover time
	// carries into the next shot and the fire rate doesn't depend on frame rate.
	w.cooldown -= dt
	if !in.Fire {
		w.cooldown = max(w.cooldown, 0)
	}
	w.bloom *= float32(math.Exp(-6 * float64(dt)))
	w.Kick *= float32(math.Exp(-18 * float64(dt)))

	if w.Reloading > 0 {
		w.Reloading = max(w.Reloading-dt, 0)
		if w.Reloading == 0 {
			w.Ammo = MagSize
		}
		w.cooldown = max(w.cooldown, 0)
		return
	}
	if in.Reload && w.Ammo < MagSize {
		a.startReload(ev)
		return
	}
	if !in.Fire || w.cooldown > 0 {
		return
	}
	if w.Ammo == 0 {
		if in.FirePressed {
			ev.Empty = true
			a.startReload(ev)
		}
		return
	}

	w.cooldown += fireInterval
	if !a.InfiniteAmmo {
		w.Ammo--
	}
	a.ShotsFired++

	moving := mathx.Vec3{p.Body.Velocity[0], 0, p.Body.Velocity[2]}.Len() / walkSpeed
	spread := baseSpread + moveSpread*min(moving, 1) + w.bloom
	if !p.onGround {
		spread += moveSpread
	}
	dir := a.jitter(p.Forward(), spread)
	shot := a.fire(p.Eye(1), dir)
	ev.Shots = append(ev.Shots, shot)

	w.bloom = min(w.bloom+bloomPerShot, 0.03)
	w.Kick = 1
	p.recoil += recoilKick

	if d := shot.Drone; d != nil {
		a.ShotsHit++
		d.Health--
		d.Flash = 1
		if d.Health <= 0 {
			a.kill(d, dir)
			ev.Kills = append(ev.Kills, d)
		}
	}
	if w.Ammo == 0 && !a.InfiniteAmmo {
		a.startReload(ev) // auto-reload after the last round
	}
}

func (a *Arena) startReload(ev *Events) {
	a.Weapon.Reloading = ReloadTime
	ev.Reloaded = true
}

// fire traces one bullet. It ignores the player and debris.
func (a *Arena) fire(from, dir mathx.Vec3) Shot {
	skip := func(b *physics.Body) bool {
		if b == a.Player.Body {
			return true
		}
		_, isDebris := b.UserData.(*Debris)
		return isDebris
	}
	hit, ok := a.Phys.Raycast(from, dir, maxRange, skip)
	if !ok {
		return Shot{From: from, To: from.Add(dir.Scale(maxRange))}
	}
	shot := Shot{From: from, To: hit.Point, Normal: hit.Normal}
	if d, isDrone := hit.Body.UserData.(*Drone); isDrone {
		shot.Drone = d
	}
	return shot
}

// jitter tilts dir by a random angle up to spread radians.
func (a *Arena) jitter(dir mathx.Vec3, spread float32) mathx.Vec3 {
	if spread <= 0 {
		return dir
	}
	up := mathx.Vec3{0, 1, 0}
	if math.Abs(float64(dir.Dot(up))) > 0.99 {
		up = mathx.Vec3{1, 0, 0}
	}
	right := dir.Cross(up).Normalize()
	up = right.Cross(dir)
	r := spread * float32(math.Sqrt(a.rng.Float64())) // uniform over the disc
	th := a.rng.Float64() * 2 * math.Pi
	return dir.Add(right.Scale(r * float32(math.Cos(th)))).Add(up.Scale(r * float32(math.Sin(th)))).Normalize()
}

// kill removes a drone, bursts it into debris and schedules its return.
func (a *Arena) kill(d *Drone, dir mathx.Vec3) {
	d.Dead = true
	d.respawn = respawnDelay
	a.Kills++
	a.Score += KillScore
	a.Phys.Remove(d.Body)

	centre := d.Body.Position
	for range debrisPerKill {
		out := mathx.Vec3{a.rng.Float32()*2 - 1, a.rng.Float32()*2 - 0.5, a.rng.Float32()*2 - 1}.Normalize()
		r := 0.07 + a.rng.Float32()*0.09
		b := physics.NewSphere(r, 0.2)
		b.Position = centre.Add(out.Scale(DroneRadius * 0.6))
		b.Velocity = out.Scale(3 + a.rng.Float32()*4).Add(dir.Scale(4)).Add(d.Body.Velocity)
		b.AngularVelocity = mathx.Vec3{a.rng.Float32()*20 - 10, a.rng.Float32()*20 - 10, a.rng.Float32()*20 - 10}
		b.Restitution = 0.4
		deb := &Debris{Body: b}
		b.UserData = deb
		a.Phys.Add(b)
		a.Debris = append(a.Debris, deb)
	}
}

func (a *Arena) moveDrones(dt float32) {
	for _, d := range a.Drones {
		d.Flash *= float32(math.Exp(-12 * float64(dt)))
		if d.Dead {
			d.respawn -= dt
			if d.respawn <= 0 {
				d.Dead, d.Health = false, DroneHealth
				d.Patrol = a.randomPatrol()
				d.place(a.Time)
				a.Phys.Add(d.Body)
			}
			continue
		}
		d.place(a.Time)
	}
}

func (a *Arena) ageDebris(dt float32) {
	alive := a.Debris[:0]
	for _, d := range a.Debris {
		d.Age += dt
		if d.Age > debrisLife || d.Body.Position[1] < -20 {
			a.Phys.Remove(d.Body)
			continue
		}
		alive = append(alive, d)
	}
	clear(a.Debris[len(alive):])
	a.Debris = alive
}

// Alive counts the drones currently flying.
func (a *Arena) Alive() int {
	n := 0
	for _, d := range a.Drones {
		if !d.Dead {
			n++
		}
	}
	return n
}

// Accuracy is hits / shots (0 before the first shot).
func (a *Arena) Accuracy() float32 {
	if a.ShotsFired == 0 {
		return 0
	}
	return float32(a.ShotsHit) / float32(a.ShotsFired)
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }
