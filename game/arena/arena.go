package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Movement tuning.
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

	DroneRadius  = 0.45
	DroneHealth  = 3
	respawnDelay = 3.0 // s
	KillScore    = 100
	ChunkScore   = 10 // per piece of structure destroyed
	droneDebris  = 10 // pieces a drone bursts into
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
	Select      int  // 1..3 picks a weapon slot this frame, 0 none
	Cycle       int  // -1 / +1 steps through the weapons
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

// Debris is a broken piece: rubble from a structure or a bit of drone. It's
// a dynamic sphere in the physics but drawn as a box of Half extents.
type Debris struct {
	Body *physics.Body
	Half mathx.Vec3
	Mat  Material
	Age  float32
	Life float32 // seconds before it's cleared away
}

// Shot is one bullet: from the eye to where it stopped.
type Shot struct {
	From, To mathx.Vec3
	Normal   mathx.Vec3 // surface normal where it stopped (zero if it hit nothing)
	Drone    *Drone     // the drone it hit, if any
	Chunk    *Chunk     // the structure piece it hit, if any
}

// Break is a chunk destroyed (by damage or by collapsing).
type Break struct {
	At, Half  mathx.Vec3
	Mat       Material
	Collapsed bool // fell because nothing held it up, rather than broken
}

// Smash is a sledgehammer blow landing.
type Smash struct {
	At, Normal mathx.Vec3
	Mat        Material // what it hit; Scrap for a drone, -1 for the level
}

// Events are what happened during a step, for sounds and effects.
type Events struct {
	Shots      []Shot
	Kills      []*Drone
	Breaks     []Break
	Smashes    []Smash
	Explosions []mathx.Vec3
	Swung      bool // a hammer swing started
	Launched   bool // a grenade left the launcher
	Switched   bool // the weapon changed
	Jumped     bool
	Landed     float32 // impact speed of a landing, 0 if none
	Reloaded   bool    // a reload started
	Empty      bool    // the trigger was pulled on an empty magazine
}

// Arena is the whole match state: a level (fixed, or a generated site with
// destructible structures), the player, their weapons and the drones.
type Arena struct {
	Phys       *physics.World
	Level      []Block      // indestructible
	Structures []*Structure // destructible
	Bounds     float32      // the playable square is -Bounds..Bounds
	Player     Player
	Weapons
	Drones   []*Drone
	Debris   []*Debris
	Grenades []*Grenade

	Time         float32
	Score        int
	Kills        int
	Destroyed    int // structure chunks destroyed
	ShotsFired   int
	ShotsHit     int
	InfiniteAmmo bool // debug

	droneFloor float32 // lowest patrol height
	rng        *rand.Rand
}

// Spawn is where the player starts in the fixed arena, looking north (-Z) at the platform.
var Spawn = mathx.Vec3{0, PlayerRadius + 0.02, 20}

// New builds the fixed arena with eight drones and the rifle out. seed picks
// the drones' patrols.
func New(seed uint64) *Arena {
	a := newArena(seed, Level(), nil, HalfSize, Spawn)
	a.droneFloor = 3.5
	a.addDrones(8)
	a.Current = WeaponRifle
	return a
}

// NewSite builds a generated demolition site: random structures to smash,
// four drones high overhead and the sledgehammer out.
func NewSite(seed uint64) *Arena {
	site := GenerateSite(seed)
	a := newArena(seed, site.Blocks, site.Structures, site.HalfSize, site.Spawn)
	a.droneFloor = 12 // above the rooftops
	a.addDrones(4)
	a.Current = WeaponHammer
	return a
}

func newArena(seed uint64, level []Block, structures []*Structure, bounds float32, spawn mathx.Vec3) *Arena {
	a := &Arena{
		Phys:       physics.NewWorld(),
		Level:      level,
		Structures: structures,
		Bounds:     bounds,
		rng:        rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}
	a.Phys.Gravity = mathx.Vec3{0, -gravity, 0}
	addLevel(a.Phys, a.Level)
	for _, s := range structures {
		for _, c := range s.Chunks {
			c.Body = physics.NewBox(c.Half, physics.Static)
			c.Body.Position = c.Centre
			c.Body.Friction = 1
			c.Body.UserData = c
			a.Phys.Add(c.Body)
		}
	}

	b := physics.NewSphere(PlayerRadius, 80)
	b.FixedRotation = true
	b.Friction = 1
	b.Restitution = 0
	b.Position = spawn
	b.UserData = &a.Player
	if err := a.Phys.Add(b); err != nil {
		panic(err)
	}
	a.Player = Player{Body: b}
	a.Weapons = newWeapons()
	return a
}

func (a *Arena) addDrones(n int) {
	for range n {
		d := &Drone{Health: DroneHealth, Patrol: a.randomPatrol()}
		d.Body = &physics.Body{Kind: physics.Kinematic, Shape: physics.Sphere, Radius: DroneRadius,
			Rotation: mathx.QuatIdentity(), Restitution: 0.6, Friction: 0.2, UserData: d}
		d.place(0)
		a.Phys.Add(d.Body)
		a.Drones = append(a.Drones, d)
	}
}

// randomPatrol picks a loop inside the bounds, clear of the walls and above head height.
func (a *Arena) randomPatrol() Patrol {
	r := a.rng
	rx := 2 + r.Float32()*6
	rz := 2 + r.Float32()*6
	cx := (r.Float32()*2 - 1) * (a.Bounds - 2 - rx)
	cz := (r.Float32()*2 - 1) * (a.Bounds - 2 - rz)
	speed := (0.25 + r.Float32()*0.4) * float32(1-2*r.IntN(2))
	return Patrol{
		Centre:   mathx.Vec3{cx, a.droneFloor + r.Float32()*3, cz},
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
	a.updateWeapons(dt, in, &ev)

	wasGround := p.onGround
	fallSpeed := -p.Body.Velocity[1]
	a.Phys.Update(dt)
	p.onGround = p.Body.Grounded
	if p.onGround && !wasGround && fallSpeed > 2 {
		ev.Landed = fallSpeed
	}

	a.updateGrenades(dt, &ev)
	a.settle(&ev)
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

// hurtDrone applies damage and destroys the drone at zero.
func (a *Arena) hurtDrone(d *Drone, damage int, push mathx.Vec3, ev *Events) {
	if d.Dead {
		return
	}
	d.Health -= damage
	d.Flash = 1
	if d.Health > 0 {
		return
	}
	d.Dead = true
	d.respawn = respawnDelay
	a.Kills++
	a.Score += KillScore
	a.Phys.Remove(d.Body)
	ev.Kills = append(ev.Kills, d)

	centre := d.Body.Position
	for range droneDebris {
		out := mathx.Vec3{a.rng.Float32()*2 - 1, a.rng.Float32()*2 - 0.5, a.rng.Float32()*2 - 1}.Normalize()
		r := 0.07 + a.rng.Float32()*0.09
		a.addDebris(centre.Add(out.Scale(DroneRadius*0.6)), mathx.Vec3{r, r, r}, Scrap,
			out.Scale(3+a.rng.Float32()*4).Add(push).Add(d.Body.Velocity))
	}
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

// Standing is the fraction of all structure chunks still standing (1 with no structures).
func (a *Arena) Standing() float32 {
	total, alive := 0, 0
	for _, s := range a.Structures {
		total += len(s.Chunks)
		alive += s.alive
	}
	if total == 0 {
		return 1
	}
	return float32(alive) / float32(total)
}

// ignoreForAim skips the player, debris and grenades in traces.
func (a *Arena) ignoreForAim(b *physics.Body) bool {
	if b == a.Player.Body {
		return true
	}
	switch b.UserData.(type) {
	case *Debris, *Grenade:
		return true
	}
	return false
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }
