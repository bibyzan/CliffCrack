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
	airAccel     = 12   // m/s^2 of steering in the air
	jumpSpeed    = 6.2  // m/s; with gravity 15 the apex is ~1.3 m
	jumpCooldown = 0.25 // s: the feet still touch the floor for a step after take-off
	gravity      = 15.0 // snappier than 9.81 for a shooter

	MaxHealth  = 150
	selfDamage = 0.5 // your own grenades hurt you this much less
	fallDeath  = -10 // below this height you're out
)

// Input is one step of a player's intent, already mapped from keys, pad or a
// bot. It is the only thing that drives a player, so it's also what a client
// would send a server.
type Input struct {
	Move        [2]float32 // x: strafe right, y: forward; length <= 1
	Look        [2]float32 // radians this step: yaw right, pitch up
	Jump        bool       // pressed this step
	Sprint      bool
	Fire        bool // trigger held
	FirePressed bool // trigger went down this step (for the empty click)
	Reload      bool // pressed this step
	Select      int  // 1..3 picks a weapon slot this step, 0 none
	Cycle       int  // -1 / +1 steps through the weapons
}

// LookOnly keeps just the aiming and weapon choice: players can look around
// (and pick a weapon) but not move or fire, as during the countdown.
func (in Input) LookOnly() Input {
	return Input{Look: in.Look, Select: in.Select, Cycle: in.Cycle}
}

// Stats are a player's numbers for the round.
type Stats struct {
	Kills      int
	Destroyed  int // structure chunks destroyed
	ShotsFired int
	ShotsHit   int // rifle rounds that hit a player
	Headshots  int
	Damage     float32 // dealt to other players
}

// Player is a first-person fighter: a fixed-rotation sphere at the feet, a
// view on top, a loadout and health.
type Player struct {
	ID         int // index in Arena.Players
	Body       *physics.Body
	Yaw, Pitch float32
	Weapons
	Health float32
	Dead   bool
	DiedAt float32 // arena time of death
	Flash  float32 // 0..1 hurt flash, decays fast
	Stats

	recoil    float32 // pitch added by recoil, recovering over time
	onGround  bool
	sinceJump float32
	sincePad  float32 // since a launch pad threw them
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

// Accuracy is rifle hits / shots (0 before the first shot).
func (p *Player) Accuracy() float32 {
	if p.ShotsFired == 0 {
		return 0
	}
	return float32(p.ShotsHit) / float32(p.ShotsFired)
}

// Debris is a broken piece of a structure. It's a dynamic sphere in the
// physics but drawn as a box of Half extents.
type Debris struct {
	Body *physics.Body
	Half mathx.Vec3
	Mat  Material
	Age  float32
	Life float32 // seconds before it's cleared away
}

// Shot is one bullet (or hammer reach): from the eye to where it stopped.
type Shot struct {
	By       *Player
	From, To mathx.Vec3
	Normal   mathx.Vec3 // surface normal where it stopped (zero if it hit nothing)
	Victim   *Player    // the player it hit, if any
	Head     bool       // ... in the head
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
	By         *Player
	At, Normal mathx.Vec3
	Mat        Material // what it hit; -1 for the level or a player
	Victim     *Player  // the player it hit, if any
}

// Explosion is a grenade going off.
type Explosion struct {
	At mathx.Vec3
	By *Player
}

// Hurt is a player taking damage.
type Hurt struct {
	Victim, By *Player // By is nil for the world
	Damage     float32
	Head       bool
	From       mathx.Vec3 // where the damage came from
}

// Kill is a player going down.
type Kill struct {
	Victim, By *Player // By is nil (or the victim) for a suicide
	Weapon     WeaponKind
	Head       bool
}

// ActionKind is a player doing something with a sound but no other effect.
type ActionKind int

const (
	ActJump ActionKind = iota
	ActLand
	ActSwing
	ActLaunch
	ActSwitch
	ActReload
	ActEmpty // the trigger was pulled on an empty magazine
	ActBoost // a launch pad threw them
)

// Action is one player action; Value is the impact speed for ActLand.
type Action struct {
	By    *Player
	Kind  ActionKind
	Value float32
}

// Events are what happened during a step, for sounds, effects and the HUD.
type Events struct {
	Shots      []Shot
	Hurts      []Hurt
	Kills      []Kill
	Breaks     []Break
	Smashes    []Smash
	Explosions []Explosion
	Actions    []Action
}

func (ev *Events) act(p *Player, k ActionKind, v float32) {
	ev.Actions = append(ev.Actions, Action{By: p, Kind: k, Value: v})
}

// Did reports whether p did k this step.
func (ev *Events) Did(p *Player, k ActionKind) bool {
	for _, a := range ev.Actions {
		if a.By == p && a.Kind == k {
			return true
		}
	}
	return false
}

// Landed is p's landing speed this step (0 if it didn't land).
func (ev *Events) Landed(p *Player) float32 {
	for _, a := range ev.Actions {
		if a.By == p && a.Kind == ActLand {
			return a.Value
		}
	}
	return 0
}

// Merge appends o's events to ev.
func (ev *Events) Merge(o Events) {
	ev.Shots = append(ev.Shots, o.Shots...)
	ev.Hurts = append(ev.Hurts, o.Hurts...)
	ev.Kills = append(ev.Kills, o.Kills...)
	ev.Breaks = append(ev.Breaks, o.Breaks...)
	ev.Smashes = append(ev.Smashes, o.Smashes...)
	ev.Explosions = append(ev.Explosions, o.Explosions...)
	ev.Actions = append(ev.Actions, o.Actions...)
}

// Arena is one round's state: the site (the indestructible shell,
// destructible structures and launch pads), the players, and the debris and
// grenades flying about.
type Arena struct {
	Phys       *physics.World
	Level      []Block      // indestructible
	Structures []*Structure // destructible
	Pads       []Pad
	Bounds     [2]float32 // the playable floor is -Bounds..Bounds on X and Z
	Spawns     []Spawn
	Players    []*Player
	Debris     []*Debris
	Grenades   []*Grenade

	Time         float32
	Live         bool // the round has started: the launch bays' pads fire
	InfiniteAmmo bool // debug

	rng *rand.Rand
}

// New generates the site for seed with players at the spawns: player i
// starts at spawn i + side (so a match can swap sides between rounds). The
// same seed always gives the same site.
func New(seed uint64, players, side int) *Arena {
	a := newArena(seed, GenerateSite(seed))
	for i := range players {
		a.AddPlayer(a.Spawns[(i+side)%len(a.Spawns)])
	}
	return a
}

func newArena(seed uint64, site *Site) *Arena {
	a := &Arena{
		Phys:       physics.NewWorld(),
		Level:      site.Blocks,
		Structures: site.Structures,
		Pads:       site.Pads,
		Bounds:     site.Bounds,
		Spawns:     site.Spawns,
		rng:        rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}
	a.Phys.Gravity = mathx.Vec3{0, -gravity, 0}
	addLevel(a.Phys, a.Level)
	for _, s := range a.Structures {
		for _, c := range s.Chunks {
			c.Body = physics.NewBox(c.Half, physics.Static)
			c.Body.Position = c.Centre
			c.Body.Friction = 1
			c.Body.UserData = c
			a.Phys.Add(c.Body)
		}
	}
	return a
}

// AddPlayer puts a new player at sp with full health and the rifle out.
func (a *Arena) AddPlayer(sp Spawn) *Player {
	b := physics.NewSphere(PlayerRadius, 80)
	b.FixedRotation = true
	b.Friction = 1
	b.Restitution = 0
	b.Position = sp.At
	p := &Player{ID: len(a.Players), Body: b, Yaw: sp.Yaw, Health: MaxHealth, Weapons: newWeapons(), sincePad: padCooldown}
	p.Current = WeaponRifle
	b.UserData = p
	if err := a.Phys.Add(b); err != nil {
		panic(err)
	}
	a.Players = append(a.Players, p)
	return p
}

// Step advances the round by dt. inputs[i] drives Players[i]; missing
// entries count as no input.
func (a *Arena) Step(dt float32, inputs []Input) Events {
	var ev Events
	a.Time += dt
	fall := make([]float32, len(a.Players))
	for i, p := range a.Players {
		if p.Dead {
			continue
		}
		var in Input
		if i < len(inputs) {
			in = inputs[i]
		}
		// Look (per step, not per physics step) and recoil recovery.
		p.Yaw = wrap(p.Yaw + in.Look[0])
		p.Pitch = clamp(p.Pitch+in.Look[1], -camera.MaxPitch, camera.MaxPitch)
		p.recoil *= float32(math.Exp(-recoilReturn * float64(dt)))
		p.Flash *= float32(math.Exp(-8 * float64(dt)))
		a.movePlayer(p, dt, in, &ev)
		a.updateWeapons(p, dt, in, &ev)
		fall[i] = -p.Body.Velocity[1]
	}

	a.Phys.Update(dt)

	for i, p := range a.Players {
		if p.Dead {
			continue
		}
		was := p.onGround
		p.onGround = p.Body.Grounded && p.sincePad > padGrace // just launched: still leaving the pad
		if p.onGround && !was && fall[i] > 2 {
			ev.act(p, ActLand, fall[i])
		}
		if p.Body.Position[1] < fallDeath {
			a.hurtPlayer(p, nil, p.Health, false, WeaponRifle, p.Body.Position, mathx.Vec3{}, &ev)
		}
		a.usePads(p, &ev)
	}
	a.updateGrenades(dt, &ev)
	a.settle(&ev)
	a.ageDebris(dt)
	return ev
}

func (a *Arena) movePlayer(p *Player, dt float32, in Input, ev *Events) {
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
	v := p.Body.Velocity
	flat := mathx.Vec3{v[0], 0, v[2]}
	if p.onGround {
		flat = flat.Add(wish.Scale(speed).Sub(flat).Scale(1 - float32(math.Exp(-groundAccel*float64(dt)))))
	} else {
		// In the air you can steer, but nothing slows you down: a launch keeps
		// its speed unless you push against it.
		top := max(flat.Len(), speed)
		flat = flat.Add(wish.Scale(airAccel * dt))
		if l := flat.Len(); l > top {
			flat = flat.Scale(top / l)
		}
	}
	p.Body.Velocity = mathx.Vec3{flat[0], v[1], flat[2]}

	p.sinceJump += dt
	p.sincePad += dt
	if in.Jump && p.onGround && p.sinceJump >= jumpCooldown {
		p.Body.Velocity[1] = jumpSpeed
		p.onGround = false
		p.sinceJump = 0
		ev.act(p, ActJump, 0)
	}
}

// hurtPlayer applies damage (halved if it's your own) and a shove, and kills
// the player at zero health. by is nil for the world.
func (a *Arena) hurtPlayer(p, by *Player, damage float32, head bool, weapon WeaponKind, from, push mathx.Vec3, ev *Events) {
	if p.Dead || damage <= 0 {
		return
	}
	if by == p {
		damage *= selfDamage
	}
	damage = min(damage, p.Health)
	p.Health -= damage
	p.Flash = 1
	p.Body.Velocity = p.Body.Velocity.Add(push)
	if by != nil && by != p {
		by.Damage += damage
	}
	ev.Hurts = append(ev.Hurts, Hurt{Victim: p, By: by, Damage: damage, Head: head, From: from})
	if p.Health > 0 {
		return
	}
	p.Dead = true
	p.DiedAt = a.Time
	a.Phys.Remove(p.Body)
	if by != nil && by != p {
		by.Kills++
	}
	ev.Kills = append(ev.Kills, Kill{Victim: p, By: by, Weapon: weapon, Head: head})
}

// Alive counts the players still standing.
func (a *Arena) Alive() int {
	n := 0
	for _, p := range a.Players {
		if !p.Dead {
			n++
		}
	}
	return n
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

// ignoreForAim skips players (their hitboxes are tested separately), debris
// and grenades in traces.
func (a *Arena) ignoreForAim(b *physics.Body) bool {
	switch b.UserData.(type) {
	case *Player, *Debris, *Grenade:
		return true
	}
	return false
}

// CanSee reports whether nothing solid lies between from and to (players,
// debris and grenades don't block).
func (a *Arena) CanSee(from, to mathx.Vec3) bool {
	off := to.Sub(from)
	dist := off.Len()
	if dist < 1e-4 {
		return true
	}
	hit, ok := a.Phys.Raycast(from, off, dist, a.ignoreForAim)
	return !ok || hit.Distance >= dist-0.05
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

// wrap brings an angle into [-pi, pi).
func wrap(a float32) float32 {
	return float32(math.Mod(math.Mod(float64(a)+math.Pi, 2*math.Pi)+2*math.Pi, 2*math.Pi) - math.Pi)
}
