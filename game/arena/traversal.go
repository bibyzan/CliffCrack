package arena

import (
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Traversal, after THE FINALS: fast, and never in the way.
//
//   - Crouch (held): the eye and head drop, you move slower, and you can
//     get under things. You stay down under a low ceiling.
//   - Slide: crouch while running fast and you drop into a slide, a burst
//     of speed that carries you, steering a little, faster still downhill.
//     Jump out of it and you keep the speed (see hopGrace).
//   - Vault: run into anything up to waist height, sprinting or with a
//     jump, and you're over it in a quarter of a second at the same speed.
//   - Climb: jump at a ledge up to about head height above your feet and
//     you pull yourself up onto it; in the air, reaching a ledge catches it
//     (so a jump climbs well above your head).
//
// A vault or climb is a short scripted move: no shooting or aiming meanwhile.
const (
	CrouchDrop  = 0.55 // m the eye and head drop crouched
	crouchSpeed = 3.6  // m/s
	crouchEase  = 14   // 1/s: how quickly you get down or up

	slideMinSpeed = 7.4  // m/s: crouching this fast (or faster) slides
	slideStart    = 11.5 // m/s at least as a slide begins...
	slideBoost    = 2.5  // ... or this much faster than you were going
	slideCap      = 14   // m/s: a slide's boost stops here (a pad's speed is kept)
	slideFriction = 5.5  // m/s^2 on the level
	slideTurn     = 1.6  // rad/s of steering
	slideMax      = 1.3  // s
	slideCooldown = 0.5  // s after one ends before the next

	vaultMax    = 1.3  // m above the feet: vaulted
	climbMax    = 2.3  // m above the feet: climbed
	mantleReach = 0.45 // m past the body's edge that a ledge is reached for
	vaultTime   = 0.26 // s
	climbTime   = 0.3  // s, and ...
	climbPerM   = 0.12 // ... s more per m climbed
)

// MantleState is a vault or climb under way.
type MantleState struct {
	On        bool
	Vault     bool       // over something low (else a climb)
	T, Dur    float32    // s in, and s long
	From, Top mathx.Vec3 // where it started (the body's centre), and the ledge's top edge
	Dir       mathx.Vec3 // which way, level
	Exit      float32    // m/s to carry on at
}

// Mantling is how far through a vault or climb p is (0..1), or -1 if not.
func (p *Player) Mantling() float32 {
	if !p.Mantle.On {
		return -1
	}
	return clamp(p.Mantle.T/p.Mantle.Dur, 0, 1)
}

// eyeHeight is the eye above the body's centre, crouching included.
func (p *Player) eyeHeight() float32 { return EyeHeight - CrouchDrop*p.Crouch }

// traverse runs crouching, slides and mantles for a step, before the
// movement. It reports whether a vault or climb has p (then there's no
// ordinary movement, and no shooting: in is changed to say so).
func (a *Arena) traverse(p *Player, dt float32, in *Input, ev *Events) bool {
	p.slideCool = max(p.slideCool-dt, 0)
	if p.Mantle.On {
		a.stepMantle(p, dt, ev)
		mantleOnly(in)
		return true
	}

	// Crouch: held, but you can't stand up into a ceiling; jumping stands
	// you up (the slide ends there too: see movePlayer).
	press := in.Crouch && !p.wasCrouch
	p.wasCrouch = in.Crouch
	want := in.Crouch && !(in.Jump && p.onGround)
	if !want && !a.roomToStand(p) {
		want = true // under something low: ducked, and kept down until there's room
	}
	p.crouched = want
	speed := flat(p.Body.Velocity).Len()
	if press && p.onGround && !p.Sliding && p.slideCool == 0 && speed >= slideMinSpeed {
		p.Sliding, p.slideTime = true, 0
		mag := min(max(speed+slideBoost, slideStart), max(speed, slideCap))
		v := flat(p.Body.Velocity).Scale(mag / speed)
		p.Body.Velocity = mathx.Vec3{v[0], p.Body.Velocity[1], v[2]}
		ev.act(p, ActSlide, mag)
	}
	down := float32(0)
	if p.crouched || p.Sliding {
		down = 1
	}
	p.Crouch += (down - p.Crouch) * (1 - float32(math.Exp(-crouchEase*float64(dt))))

	if a.tryMantle(p, *in, ev) {
		mantleOnly(in)
		return true
	}
	return false
}

// mantleOnly is what's left of an input during a vault or climb: looking,
// and the presses that don't need hands.
func mantleOnly(in *Input) {
	in.Fire, in.FirePressed, in.Aim, in.Jump = false, false, false, false
	in.Melee, in.Throw, in.Gadget, in.Interact = false, false, false, false
}

// roomToStand reports whether there's head room to stand up.
func (a *Arena) roomToStand(p *Player) bool {
	_, blocked := a.Phys.Raycast(p.Body.Position, mathx.Vec3{0, 1, 0}, EyeHeight+0.2, a.ignoreForAim)
	return !blocked
}

// tryMantle starts a vault or climb if p is pushing into something they
// can get over or onto, and reports whether it did.
func (a *Arena) tryMantle(p *Player, in Input, ev *Events) bool {
	if p.Dead || p.boosted || p.Grapple.On || p.Sliding || in.Move[1] < 0.3 {
		return false
	}
	forward := camera.Direction(p.Yaw, 0)
	right := forward.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	wish := forward.Scale(in.Move[1]).Add(right.Scale(in.Move[0]))
	if wish.Len() < 0.3 {
		return false
	}
	dir := wish.Normalize()
	up, downDir := mathx.Vec3{0, 1, 0}, mathx.Vec3{0, -1, 0}
	pos := p.Body.Position
	feet := pos[1] - PlayerRadius
	// Something in the way, at the shin or the waist.
	var wallDist float32
	found := false
	for _, h := range []float32{0.25, 0.8, 1.3} {
		hit, ok := a.Phys.Raycast(mathx.Vec3{pos[0], feet + h, pos[2]}, dir, PlayerRadius+mantleReach, a.ignoreForAim)
		if ok && hit.Normal[1] < 0.3 {
			wallDist, found = hit.Distance, true
			break
		}
	}
	if !found {
		return false
	}
	// Its top: straight down from overhead, just past the face (close in
	// first, for a thin wall's top), then further in.
	var top physics.RayHit
	var rise float32
	found = false
	for _, in := range []float32{0.06, 0.2} {
		probe := mathx.Vec3{pos[0], feet + climbMax + 0.3, pos[2]}.Add(dir.Scale(wallDist + in))
		hit, ok := a.Phys.Raycast(probe, downDir, climbMax+0.3, a.ignoreForAim)
		if !ok || hit.Normal[1] < 0.7 || hit.Distance < 0.05 {
			continue // too high to reach the top of, or no top there
		}
		if r := hit.Point[1] - feet; r > stepHeight+0.05 && r <= climbMax {
			top, rise, found = hit, r, true
			break
		}
	}
	if !found {
		return false
	}
	// Room to get up there, and to be there (crouched, at least).
	if _, blocked := a.Phys.Raycast(pos, up, rise+PlayerRadius+0.1, a.ignoreForAim); blocked {
		return false
	}
	if _, blocked := a.Phys.Raycast(top.Point.Add(mathx.Vec3{0, 0.05, 0}), up, EyeHeight-CrouchDrop+0.3, a.ignoreForAim); blocked {
		return false
	}
	speed := flat(p.Body.Velocity).Len()
	vault := rise <= vaultMax
	switch {
	case vault && (in.Jump || p.jumpQueue > 0 || (sprinting(in) && speed > 5) || !p.onGround):
	case !vault && (in.Jump || p.jumpQueue > 0 || (!p.onGround && p.airborne > 0.08)):
	default:
		return false
	}
	m := MantleState{On: true, Vault: vault, From: pos, Top: top.Point, Dir: dir}
	if vault {
		m.Dur, m.Exit = vaultTime, max(speed, walkSpeed)
	} else {
		m.Dur, m.Exit = climbTime+climbPerM*rise, walkSpeed*0.75
	}
	p.Mantle = m
	p.Sliding, p.jumpQueue, p.onGround = false, 0, false
	kind := ActClimb
	if vault {
		kind = ActVault
	}
	ev.act(p, kind, rise)
	return true
}

// stepMantle moves p along a vault or climb: up the face first, then over
// the top, and off at the end with its speed.
func (a *Arena) stepMantle(p *Player, dt float32, ev *Events) {
	m := &p.Mantle
	m.T += dt
	k := clamp(m.T/m.Dur, 0, 1)
	rise := smoothstep(clamp(k/0.6, 0, 1))
	over := smoothstep(clamp((k-0.5)/0.5, 0, 1))
	topY := m.Top[1] + PlayerRadius + 0.05
	end := m.Top.Add(m.Dir.Scale(PlayerRadius * 0.7))
	target := mathx.Vec3{
		m.From[0] + (end[0]-m.From[0])*over,
		m.From[1] + (topY-m.From[1])*rise,
		m.From[2] + (end[2]-m.From[2])*over,
	}
	p.Body.Velocity = target.Sub(p.Body.Position).Scale(1 / dt)
	p.onGround = false
	if k < 1 {
		return
	}
	m.On = false
	hop := float32(0.5)
	if m.Vault {
		hop = 1.8 // up and over
	}
	out := m.Dir.Scale(m.Exit)
	p.Body.Velocity = mathx.Vec3{out[0], hop, out[2]}
	p.sinceJump, p.sinceLand = jumpCooldown, 0 // land running: no hop grace spent
}
