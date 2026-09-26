package arena

import (
	"math"

	"CliffCrack/engine/mathx"
)

// GadgetKind is the one gadget a player carries, chosen before each round
// (see Match): taking one means giving up the others.
type GadgetKind int

const (
	// GadgetHammer: the sledgehammer, brought out in place of the gun. It
	// hits as hard as ever (two blows down a player, a crater in a wall),
	// but while it's out there's no shooting.
	GadgetHammer GadgetKind = iota
	// GadgetGrapple: a hook fired along the view that reels you in fast,
	// keeping your momentum (let go to fling on), then has to recharge.
	GadgetGrapple
	GadgetKinds
)

// GadgetNames are the HUD labels.
var GadgetNames = [GadgetKinds]string{"HAMMER", "GRAPPLE"}

// Melee and gadget tuning.
const (
	// The elbow: quick, short and light; the melee everyone has.
	ElbowTime        = 0.45 // s per strike
	ElbowHitAt       = 0.1  // s in, when it lands
	elbowReach       = 1.9  // m
	ElbowDamage      = 30   // to a player: it gets through armour, but takes five or so
	elbowChunkDamage = 30   // to structures (a crack, a pane of glass)
	elbowRadius      = 0.35 // m

	hammerDraw = 0.25 // s to bring the hammer out (or the gun back)

	GrappleReach    = 38   // m the hook flies
	grapplePull     = 42   // m/s^2 along the rope
	grappleTop      = 30   // m/s: it pulls no faster than this along the rope
	grappleLift     = 0.55 // share of gravity it holds up while reeling
	grappleHop      = 4.5  // m/s up when it catches with you on the ground, to get off it
	GrappleTime     = 2.2  // s it holds before letting go by itself
	grappleArrive   = 2.2  // m from the anchor it lets go: you're there
	GrappleCooldown = 4.0  // s to recharge after a pull
	grappleMissWait = 1.0  // s after a miss
	GrappleFly      = 0.12 // s the hook takes to fly out (drawn; it catches at once)
)

// ElbowState is an elbow strike in progress.
type ElbowState struct {
	Swing  float32 // s into it, -1 when idle
	struck bool
}

// Progress is how far through the strike the elbow is (0..1), or -1 idle.
func (e ElbowState) Progress() float32 {
	if e.Swing < 0 {
		return -1
	}
	return e.Swing / ElbowTime
}

// GrappleState is a player's grapple: the hook out on its rope, or
// recharging.
type GrappleState struct {
	On       bool       // hooked on and reeling in
	To       mathx.Vec3 // where the hook is: the anchor, or where a miss ran out
	Miss     bool       // the last shot caught nothing
	Shot     float32    // s since the hook was fired (for drawing it fly out)
	Time     float32    // s it's been reeling
	Cooldown float32    // s until it can be fired again

	chunk  *Chunk  // what it's hooked into, if a structure (it lets go if that breaks)
	victim *Player // ... or a player (it follows them)
}

// Ready reports whether the grapple can be fired (0..1 charged when not).
func (g GrappleState) Ready() (float32, bool) {
	if g.Cooldown <= 0 && !g.On {
		return 1, true
	}
	return 1 - g.Cooldown/GrappleCooldown, false
}

// Swinging reports whether a melee strike is in progress, the hammer's or
// the elbow's (the gun is put aside).
func (w *Weapons) Swinging() bool { return w.Hammer.Swing >= 0 || w.Elbow.Swing >= 0 }

// Holding is what's in the hand to draw: the hammer while it's out (or
// swinging), else the weapon in hand.
func (w *Weapons) Holding() WeaponKind {
	if w.HammerOut || w.Hammer.Swing >= 0 {
		return WeaponHammer
	}
	return w.Current
}

// putHammerAway puts the hammer back and brings the gun up.
func (w *Weapons) putHammerAway() {
	if w.HammerOut && w.Hammer.Swing < 0 {
		w.HammerOut = false
		w.Switching = SwitchTime
	}
}

// useGadget is the gadget button: the hammer out (or away), or the grapple
// fired (or let go).
func (a *Arena) useGadget(p *Player, ev *Events) {
	w := &p.Weapons
	switch w.Gadget {
	case GadgetHammer:
		switch {
		case w.HammerOut:
			w.putHammerAway()
			ev.act(p, ActSwitch, 0)
		case !w.Swinging():
			w.HammerOut, w.ADS, w.Switching = true, 0, hammerDraw
			ev.act(p, ActGadget, float32(GadgetHammer))
		}
	case GadgetGrapple:
		g := &w.Grapple
		switch {
		case g.On:
			a.letGo(p, ev)
		case g.Cooldown <= 0:
			a.fireGrapple(p, ev)
		}
	}
}

// fireGrapple shoots the hook along the view: it catches on the first thing
// in reach (a wall, the ground, another player) and starts reeling in.
func (a *Arena) fireGrapple(p *Player, ev *Events) {
	g := &p.Grapple
	eye, fwd := p.Eye(1), p.Forward()
	shot := a.trace(p, eye, fwd, GrappleReach)
	g.Shot, g.To = 0, shot.To
	if shot.Normal == (mathx.Vec3{}) {
		g.Miss, g.Cooldown = true, grappleMissWait
		ev.act(p, ActGrapple, 0)
		return
	}
	*g = GrappleState{On: true, To: shot.To, chunk: shot.Chunk, victim: shot.Victim}
	if p.onGround {
		p.Body.Velocity[1] = max(p.Body.Velocity[1], grappleHop) // off the ground, where it can pull
	}
	p.boosted = false
	ev.act(p, ActGrapple, 1)
}

// letGo releases the grapple and starts it recharging.
func (a *Arena) letGo(p *Player, ev *Events) {
	g := &p.Grapple
	if !g.On {
		return
	}
	g.On, g.chunk, g.victim = false, nil, nil
	g.Cooldown = GrappleCooldown
	if ev != nil {
		ev.act(p, ActGrapple, 2)
	}
}

// updateGrapple runs the grapple for a step: the timers, and while it's
// hooked on, the pull. It lets go when you arrive, when it's held long
// enough, when what it caught is gone, or when you jump.
func (a *Arena) updateGrapple(p *Player, dt float32, in Input, ev *Events) {
	g := &p.Grapple
	g.Shot += dt
	if !g.On {
		g.Cooldown = max(g.Cooldown-dt, 0)
		return
	}
	switch {
	case g.victim != nil && g.victim.Dead, g.chunk != nil && !g.chunk.Alive:
		a.letGo(p, ev)
		return
	case g.victim != nil:
		g.To = g.victim.Chest()
	}
	if in.Jump || !a.pullGrapple(p, dt) {
		a.letGo(p, ev)
	}
}

// pullGrapple reels p in towards the hook for a step (it's also how a guest
// predicts itself being pulled), and reports whether it holds on.
func (a *Arena) pullGrapple(p *Player, dt float32) bool {
	g := &p.Grapple
	g.Time += dt
	to := g.To.Sub(p.Body.Position)
	d := to.Len()
	if d < grappleArrive || g.Time > GrappleTime {
		return false
	}
	dir := to.Scale(1 / d)
	v := p.Body.Velocity
	if v.Dot(dir) < grappleTop {
		v = v.Add(dir.Scale(grapplePull * dt))
	}
	v[1] -= a.Phys.Gravity[1] * grappleLift * dt // (gravity is negative: this holds some of it up)
	p.Body.Velocity = v
	return true
}

// updateElbow carries a strike through: it lands ElbowHitAt in.
func (a *Arena) updateElbow(p *Player, dt float32, ev *Events) {
	e := &p.Elbow
	e.Swing += dt
	if !e.struck && e.Swing >= ElbowHitAt {
		e.struck = true
		a.elbowStrike(p, ev)
	}
	if e.Swing >= ElbowTime {
		e.Swing = -1
	}
}

// elbowStrike lands an elbow: whatever's right in front takes a light blow.
func (a *Arena) elbowStrike(p *Player, ev *Events) {
	eye, fwd := p.Eye(1), p.Forward()
	right, up := basis(fwd)
	var best Shot
	bestDist := float32(math.MaxFloat32)
	for _, off := range [][2]float32{{0, 0}, {0.18, 0}, {-0.18, 0}, {0, -0.15}} {
		dir := fwd.Add(right.Scale(off[0])).Add(up.Scale(off[1])).Normalize()
		s := a.trace(p, eye, dir, elbowReach)
		if s.Normal == (mathx.Vec3{}) {
			continue
		}
		d := s.To.Sub(eye).Len()
		if s.Victim != nil {
			d -= elbowReach // a player anywhere in the arc before a wall
		}
		if d < bestDist {
			best, bestDist = s, d
		}
	}
	if bestDist == math.MaxFloat32 {
		return
	}
	smash := Smash{By: p, At: best.To, Normal: best.Normal, Mat: -1, Victim: best.Victim, Light: true}
	if best.Victim != nil {
		push := fwd.Scale(3).Add(mathx.Vec3{0, 1, 0})
		a.hurtPlayer(best.Victim, p, ElbowDamage, false, WeaponElbow, eye, push, ev)
	} else {
		if best.Chunk != nil {
			smash.Mat = best.Chunk.Mat
		}
		a.blast(best.To, elbowRadius, elbowChunkDamage, 0, 2, fwd.Scale(1), p, WeaponElbow, ev)
	}
	ev.Smashes = append(ev.Smashes, smash)
	p.recoil += 0.015
}
