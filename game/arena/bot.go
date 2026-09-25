package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// BotSkill tunes how well a bot plays.
type BotSkill struct {
	Reaction float32 // s from spotting an enemy to opening fire
	TurnRate float32 // 1/s: how quickly its aim closes on where it wants to look
	AimError float32 // m of aim offset on a fresh sighting, settling while it tracks
	Settle   float32 // 1/s: how quickly that error settles
	FOV      float32 // radians either side of its view in which it notices enemies
	Hearing  float32 // m within which it hears gunfire, launches and hammer blows
	Wobble   float32 // m of steady aim drift at 15 m, however long it tracks
}

// Bot skill levels. Normal is a fair opponent: it reacts in about a third of
// a second, its first shots on a new sighting go wide, and it never quite
// holds its aim still.
var (
	BotEasy   = BotSkill{Reaction: 0.6, TurnRate: 6, AimError: 1.6, Settle: 1.1, FOV: 0.85, Hearing: 35, Wobble: 0.6}
	BotNormal = BotSkill{Reaction: 0.35, TurnRate: 9, AimError: 1.1, Settle: 1.8, FOV: 1.05, Hearing: 45, Wobble: 0.35}
	BotHard   = BotSkill{Reaction: 0.2, TurnRate: 12, AimError: 0.7, Settle: 2.6, FOV: 1.25, Hearing: 60, Wobble: 0.15}
)

const (
	hammerRange = 4.5 // m: closer than this the bot charges with the hammer
	probeReach  = 1.3 // m ahead it checks for obstacles
)

// Bot plays a player through the same Input a person would give. It knows
// the site, but not where its enemy is: it only notices them in its field of
// view with nothing in the way, or by hearing them. Walls in its way get the
// hammer; enemies behind cover get a grenade lobbed at them.
type Bot struct {
	Skill   BotSkill
	Passive bool // never attacks (for testing)

	rng       *rand.Rand
	phase     float32 // offsets its aim drift from other bots'
	sees      bool
	seenFor   float32
	known     bool       // it has an idea where the enemy is
	lastKnown mathx.Vec3 // ... there
	lastInfo  float32    // arena time it last saw or heard them
	aimErr    mathx.Vec3

	strafe, strafeT float32
	jumpT           float32
	lobT            float32    // until it tries another speculative grenade
	prevAim         [2]float32 // the view angles it wanted last step
	tracking        bool       // ... and it was looking at the enemy then
	goal            mathx.Vec3
	goalT           float32
	smash           *Chunk // the piece in its way it's hammering
	smashT          float32
	avoid, avoidT   float32 // sidestepping something it can't break
	stuckT          float32
}

// NewBot makes a bot of normal skill; seed varies its choices.
func NewBot(seed uint64) *Bot {
	rng := rand.New(rand.NewPCG(seed, seed^0x2545f4914f6cdd1d))
	return &Bot{Skill: BotNormal, rng: rng, phase: rng.Float32() * 100, strafe: 1}
}

// Reset forgets everything it knew, for a new round.
func (b *Bot) Reset() {
	*b = Bot{Skill: b.Skill, Passive: b.Passive, rng: b.rng, phase: b.phase, strafe: 1}
}

// Think decides self's input for this step.
func (b *Bot) Think(a *Arena, self *Player, dt float32) Input {
	var in Input
	enemy := nearestEnemy(a, self)
	if self.Dead || enemy == nil {
		return in
	}
	b.perceive(a, self, enemy, dt)
	b.strafeT -= dt
	b.jumpT -= dt
	b.lobT -= dt
	b.goalT -= dt
	b.avoidT -= dt

	eye, feet := self.Eye(1), self.Body.Position
	want := WeaponRifle
	var look mathx.Vec3          // where it wants to aim
	var move mathx.Vec3          // which way it wants to go (world, flat)
	cone := float32(0.08)        // how close the aim must be to fire
	fire, sprint := false, false // fire: it would like to, once on target

	toward := func(p mathx.Vec3) mathx.Vec3 { return flat(p.Sub(feet)).Normalize() }
	switch {
	case b.sees:
		dist := flat(enemy.Body.Position.Sub(feet)).Len()
		t := float64(a.Time) + float64(b.phase)
		drift := mathx.Vec3{float32(math.Sin(t * 1.7)), float32(math.Sin(t*2.3 + 1)), float32(math.Cos(t * 1.9))}
		drift = drift.Scale(b.Skill.Wobble * min(dist/15, 1.5))
		look = enemy.Chest().Add(enemy.Body.Velocity.Scale(0.06)).Add(b.aimErr).Add(drift)
		fire = b.seenFor >= b.Skill.Reaction
		reloading := self.Rifle.Reloading > 0
		if dist < hammerRange || (reloading && dist < 9) {
			// Close in and swing.
			want, move, sprint = WeaponHammer, toward(enemy.Body.Position), true
			look = enemy.Chest()
			cone = 0.3
			fire = fire && dist < hammerReach-0.2
			break
		}
		cone = max(float32(math.Atan(float64(bodyRadius/dist))), 0.015)
		// Strafe, and keep to a comfortable range.
		if b.strafeT <= 0 {
			b.strafe = float32(1 - 2*b.rng.IntN(2))
			b.strafeT = 0.4 + b.rng.Float32()*0.9
		}
		to := toward(enemy.Body.Position)
		side := mathx.Vec3{-to[2], 0, to[0]}
		move = side.Scale(b.strafe)
		switch {
		case dist > 22:
			move = move.Add(to.Scale(0.8))
		case dist < 8:
			move = move.Add(to.Scale(-0.6))
		}
		if b.jumpT <= 0 {
			in.Jump = b.rng.IntN(3) == 0
			b.jumpT = 1.5 + b.rng.Float32()*2.5
		}

	case b.known:
		// Head for where they were; now and then lob a grenade at them.
		target := b.lastKnown
		dist := flat(target.Sub(feet)).Len()
		look = target.Add(mathx.Vec3{0, 0.5, 0})
		move, sprint = toward(target), true
		since := a.Time - b.lastInfo
		if since > 0.6 && since < 6 && dist > 7 && dist < 28 && b.lobT <= 0 &&
			(self.Launcher.Ammo > 0 || self.Launcher.Reloading > 0) {
			want = WeaponLauncher
			muzzle := eye.Add(self.Forward().Scale(0.7))
			look = eye.Add(lobDirection(target.Sub(muzzle)).Scale(10))
			cone, fire = 0.04, true
			move = mathx.Vec3{}
		}
		if dist < 1.5 {
			b.known = false // not here any more
		}

	default:
		// Roam the site looking for them.
		if b.goalT <= 0 || flat(b.goal.Sub(feet)).Len() < 2 {
			for range 8 { // somewhere with floor left under it
				b.goal = mathx.Vec3{(b.rng.Float32()*2 - 1) * (a.Bounds[0] - 4), 0, (b.rng.Float32()*2 - 1) * (a.Bounds[1] - 4)}
				if groundBelow(a, b.goal.Add(mathx.Vec3{0, 3, 0}), 4) {
					break
				}
			}
			b.goalT = 8 + b.rng.Float32()*6
		}
		move = toward(b.goal)
		look = eye.Add(move.Scale(10))
	}

	// Anything in the way: hop the low stuff, hammer through structures,
	// sidestep the boundary.
	if move != (mathx.Vec3{}) && want != WeaponLauncher && !(b.sees && want == WeaponHammer) {
		if b.avoidT > 0 {
			move = mathx.Vec3{-move[2], 0, move[0]}.Scale(b.avoid).Add(move.Scale(0.3)).Normalize()
		}
		dir := move.Normalize()
		knee := feet.Add(mathx.Vec3{0, -0.05, 0})
		chest := feet.Add(mathx.Vec3{0, 0.55, 0})
		walkable := func(h physics.RayHit) bool { return h.Normal[1] > 0.5 } // a ramp, not a wall
		if hit, ok := a.Phys.Raycast(chest, dir, probeReach, a.ignoreForAim); ok && !walkable(hit) {
			if c, isChunk := hit.Body.UserData.(*Chunk); isChunk && !b.sees {
				if c != b.smash {
					b.smash, b.smashT = c, 0
				}
			} else if b.avoidT <= 0 {
				b.avoid, b.avoidT = float32(1-2*b.rng.IntN(2)), 0.8
			}
		} else if hit, ok := a.Phys.Raycast(knee, dir, probeReach*0.7, a.ignoreForAim); ok && !walkable(hit) {
			in.Jump = true
		}
	}
	if b.smash != nil {
		b.smashT += dt
		if !b.smash.Alive || b.smashT > 4 || b.sees {
			if b.smash.Alive && b.smashT > 4 {
				b.avoid, b.avoidT = float32(1-2*b.rng.IntN(2)), 1.2 // too tough: go round
			}
			b.smash = nil
		} else {
			want, fire, cone = WeaponHammer, true, 0.25
			look = b.smash.Centre
			for k := range 3 { // the face nearest the eye
				look[k] = clamp(eye[k], b.smash.Centre[k]-b.smash.Half[k]*0.8, b.smash.Centre[k]+b.smash.Half[k]*0.8)
			}
			move = mathx.Vec3{}
		}
	}

	// Never walk (or jump) off an edge or into a hole: turn along it.
	if move != (mathx.Vec3{}) && self.OnGround() {
		dir := move.Normalize()
		if !groundAhead(a, feet, dir) {
			in.Jump = false
			move = mathx.Vec3{}
			for _, turn := range []float32{0.8, -0.8, 1.6, -1.6, 2.4, -2.4} {
				t := turn * b.strafe
				c, s := float32(math.Cos(float64(t))), float32(math.Sin(float64(t)))
				try := mathx.Vec3{dir[0]*c - dir[2]*s, 0, dir[0]*s + dir[2]*c}
				if groundAhead(a, feet, try) {
					move = try
					break
				}
			}
		}
	}

	// Stuck against something: jump and sidestep.
	speed := flat(self.Body.Velocity).Len()
	if move != (mathx.Vec3{}) && speed < 0.8 && self.OnGround() {
		b.stuckT += dt
		if b.stuckT > 0.7 {
			in.Jump = true
			b.avoid, b.avoidT = float32(1-2*b.rng.IntN(2)), 1
			b.stuckT = 0
		}
	} else {
		b.stuckT = 0
	}

	if self.Current != want {
		in.Select = int(want) + 1
	}
	if !b.sees && self.Current == WeaponRifle && self.Rifle.Ammo < MagSize/2 && self.Rifle.Reloading == 0 {
		in.Reload = true
	}
	var onTarget bool
	in.Look, onTarget = aimAt(self, look, cone, b.Skill.TurnRate, dt)
	// While tracking, follow the target's motion across the view as well as
	// closing the gap, as a player does; otherwise the aim trails behind
	// anything moving.
	want2 := viewAngles(look.Sub(eye))
	if b.sees && b.tracking {
		for k := range 2 {
			d := want2[k] - b.prevAim[k]
			if k == 0 {
				d = wrap(d)
			}
			in.Look[k] += clamp(d, -0.05, 0.05)
		}
	}
	b.prevAim, b.tracking = want2, b.sees
	if fire && onTarget && !b.Passive && self.Current == want {
		in.Fire, in.FirePressed = true, true
	}

	fwd := camera.Direction(self.Yaw, 0)
	right := fwd.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	if l := move.Len(); l > 1 {
		move = move.Scale(1 / l)
	}
	in.Move = [2]float32{move.Dot(right), move.Dot(fwd)}
	in.Sprint = sprint && in.Move[1] > 0.5
	return in
}

// perceive updates what the bot knows: whether it can see the enemy (in its
// field of view, or very close, with nothing solid in between).
func (b *Bot) perceive(a *Arena, self, enemy *Player, dt float32) {
	eye := self.Eye(1)
	to := enemy.Head().Sub(eye)
	dist := to.Len()
	inView := dist < 4 || angleBetween(self.Forward(), to) < b.Skill.FOV
	sees := inView && (a.CanSee(eye, enemy.Head()) || a.CanSee(eye, enemy.Chest()))
	if sees {
		if !b.sees {
			// A fresh sighting: the aim starts off, then settles.
			b.seenFor = 0
			e := mathx.Vec3{b.rng.Float32()*2 - 1, b.rng.Float32()*2 - 1, b.rng.Float32()*2 - 1}
			b.aimErr = e.Scale(b.Skill.AimError * (0.5 + b.rng.Float32()) * min(dist/15, 1.5))
		}
		b.seenFor += dt
		b.aimErr = b.aimErr.Scale(float32(math.Exp(-float64(b.Skill.Settle * dt))))
		b.known, b.lastKnown, b.lastInfo = true, enemy.Body.Position, a.Time
	}
	b.sees = sees
}

// Hear lets the bot react to the step's events: gunfire, launches and blows
// it can hear give away where the enemy is, and so does getting hurt.
func (b *Bot) Hear(a *Arena, self *Player, ev *Events) {
	if self.Dead {
		return
	}
	near := func(p *Player) bool {
		return p != nil && p != self && p.Body.Position.Sub(self.Body.Position).Len() < b.Skill.Hearing
	}
	for _, s := range ev.Shots {
		if near(s.By) {
			b.heard(a, s.By)
		}
	}
	for _, s := range ev.Smashes {
		if near(s.By) {
			b.heard(a, s.By)
		}
	}
	for _, act := range ev.Actions {
		switch {
		case act.By == self && act.Kind == ActLaunch:
			b.lobT = 2.5 + b.rng.Float32()*2
		case act.Kind == ActLaunch && near(act.By):
			b.heard(a, act.By)
		}
	}
	for _, h := range ev.Hurts {
		if h.Victim == self && h.By != nil && h.By != self {
			b.heard(a, h.By)
		}
	}
}

func (b *Bot) heard(a *Arena, p *Player) {
	if b.sees {
		return
	}
	b.known, b.lastKnown, b.lastInfo = true, p.Body.Position, a.Time
}

// nearestEnemy is the closest other player still alive.
func nearestEnemy(a *Arena, self *Player) *Player {
	var best *Player
	bestDist := float32(math.MaxFloat32)
	for _, p := range a.Players {
		if p == self || p.Dead {
			continue
		}
		if d := p.Body.Position.Sub(self.Body.Position).Len(); d < bestDist {
			best, bestDist = p, d
		}
	}
	return best
}

// lobDirection is the direction to launch a grenade so it lands at offset
// (from the muzzle), using the flatter of the two ballistic solutions. Out of
// range it aims at 45 degrees.
func lobDirection(offset mathx.Vec3) mathx.Vec3 {
	f := flat(offset)
	x := f.Len()
	if x < 1e-3 {
		return offset.Normalize()
	}
	const v, g = grenadeSpeed, gravity
	y := offset[1]
	disc := v*v*v*v - g*(g*x*x+2*y*v*v)
	angle := float32(math.Pi / 4)
	if disc >= 0 {
		angle = float32(math.Atan((v*v - math.Sqrt(float64(disc))) / (g * float64(x))))
	}
	h := f.Scale(1 / x)
	c, s := math.Cos(float64(angle)), math.Sin(float64(angle))
	return mathx.Vec3{h[0] * float32(c), float32(s), h[2] * float32(c)}
}

// aimAt eases p's view towards point at rate (1/s) and reports whether it's
// within cone radians of it.
func aimAt(p *Player, point mathx.Vec3, cone, rate, dt float32) ([2]float32, bool) {
	want := viewAngles(point.Sub(p.Eye(1)))
	dYaw := wrap(want[0] - p.Yaw)
	dPitch := want[1] - p.ViewPitch()
	k := 1 - float32(math.Exp(-float64(rate*dt)))
	return [2]float32{dYaw * k, dPitch * k}, float32(math.Hypot(float64(dYaw), float64(dPitch))) < cone
}

// viewAngles are the yaw and pitch that look along dir.
func viewAngles(dir mathx.Vec3) [2]float32 {
	dir = dir.Normalize()
	return [2]float32{
		float32(math.Atan2(float64(dir[0]), float64(-dir[2]))),
		float32(math.Asin(float64(clamp(dir[1], -1, 1)))),
	}
}

func flat(v mathx.Vec3) mathx.Vec3 { return mathx.Vec3{v[0], 0, v[2]} }

func angleBetween(a, b mathx.Vec3) float32 {
	c := a.Dot(b) / (a.Len() * b.Len())
	return float32(math.Acos(float64(clamp(c, -1, 1))))
}

// groundAhead reports whether there's something to stand on a step or two
// along dir from feet: a floor, or a drop onto something solid, not the pit.
func groundAhead(a *Arena, feet, dir mathx.Vec3) bool {
	for _, d := range []float32{0.9, 1.8} {
		if !groundBelow(a, feet.Add(dir.Scale(d)).Add(mathx.Vec3{0, 0.4, 0}), 3.5) {
			return false
		}
	}
	return true
}

// groundBelow reports whether a floor (not just a girder) lies within reach
// below from.
func groundBelow(a *Arena, from mathx.Vec3, reach float32) bool {
	hit, ok := a.Phys.Raycast(from, mathx.Vec3{0, -1, 0}, reach, a.ignoreForAim)
	if !ok {
		return false
	}
	kind, isBlock := hit.Body.UserData.(BlockKind)
	return !isBlock || kind != Girder
}
