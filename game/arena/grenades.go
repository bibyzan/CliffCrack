package arena

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// GrenadeKind is what's in flight: a thrown frag or sticky, or a round from
// the grenade launcher.
type GrenadeKind int

const (
	Frag   GrenadeKind = iota // bounces, and goes off on its fuse
	Sticky                    // sticks to the first thing it touches (players too), then goes off
	GrenadeKinds

	GrenadeRound = GrenadeKinds // from the launcher: bursts on impact
)

// GrenadeNames are the HUD labels, by kind (thrown ones).
var GrenadeNames = [GrenadeKinds]string{"FRAG", "STICKY"}

// GrenadeSpec is how a kind of grenade behaves and hurts.
type GrenadeSpec struct {
	Fuse         float32 // s: from the throw, or for a sticky from when it sticks
	Bounce       float32 // restitution
	Radius       float32 // m of blast
	PlayerDamage float32 // at the centre, through armour
	ChunkDamage  float32 // at the centre
	Push         float32 // m/s at the centre
	Cause        WeaponKind
}

var grenadeSpecs = [GrenadeKinds + 1]GrenadeSpec{
	Frag:         {Fuse: 2.2, Bounce: 0.45, Radius: 5, PlayerDamage: 130, ChunkDamage: 300, Push: 14, Cause: WeaponFrag},
	Sticky:       {Fuse: 1.6, Bounce: 0, Radius: 4, PlayerDamage: 200, ChunkDamage: 380, Push: 12, Cause: WeaponSticky},
	GrenadeRound: {Fuse: grenadeFuse, Bounce: 0.3, Radius: BlastRadius, PlayerDamage: BlastPlayerDamage, ChunkDamage: blastDamage, Push: blastPush, Cause: WeaponLauncher},
}

// Throwing.
const (
	throwSpeed    = 17  // m/s
	throwLift     = 3.5 // m/s up on top of the aim, for an arc
	throwInterval = 0.8 // s between throws
	MaxGrenades   = 4   // of each kind
	unstuckFuse   = 4   // s: a sticky that never finds anything still goes off
	fragDrag      = 4   // 1/s: a frag's slowing as it rolls
)

// Grenade is a grenade in flight, or stuck to something.
type Grenade struct {
	ID    int // unique in the arena
	Body  *physics.Body
	Owner *Player
	Kind  GrenadeKind
	Age   float32

	Stuck   bool       // a sticky that's found something
	StuckTo *Player    // ... a player, carrying it round with them
	offset  mathx.Vec3 // from them (or where it stuck, if not a player)
	stuckAt float32    // Age when it stuck
}

// Position is where the grenade is (stuck ones move with whoever they're on).
func (g *Grenade) Position() mathx.Vec3 {
	if g.StuckTo != nil {
		return g.StuckTo.Body.Position.Add(g.offset)
	}
	return g.Body.Position
}

// Fuse is the seconds left before it goes off.
func (g *Grenade) Fuse() float32 {
	spec := grenadeSpecs[g.Kind]
	switch {
	case g.Kind == Sticky && g.Stuck:
		return spec.Fuse - (g.Age - g.stuckAt)
	case g.Kind == Sticky:
		return unstuckFuse - g.Age
	}
	return spec.Fuse - g.Age
}

// throwGrenade throws one of the kind selected, if there's one to throw.
func (a *Arena) throwGrenade(p *Player, ev *Events) {
	w := &p.Weapons
	if w.throwWait > 0 || w.Grenades[w.GrenadeKind] == 0 {
		return
	}
	w.throwWait = throwInterval
	if !a.InfiniteAmmo {
		w.Grenades[w.GrenadeKind]--
	}
	fwd := p.Forward()
	right, _ := basis(fwd)
	at := p.Eye(1).Add(fwd.Scale(0.5)).Add(right.Scale(-0.2)) // from the off hand
	a.launch(p, w.GrenadeKind, at, fwd.Scale(throwSpeed).Add(mathx.Vec3{0, throwLift, 0}))
	ev.act(p, ActThrow, float32(w.GrenadeKind))
}

// updateGrenades sets off grenades: launcher rounds on any impact or on
// reaching someone, frags on their fuse, stickies a moment after sticking
// to whatever (or whoever) they touch first.
func (a *Arena) updateGrenades(dt float32, ev *Events) {
	if len(a.Grenades) == 0 {
		return
	}
	hit := map[*physics.Body]*physics.Body{}
	for _, im := range a.Phys.Impacts() {
		hit[im.A], hit[im.B] = im.B, im.A
	}
	live := a.Grenades[:0]
	for _, g := range a.Grenades {
		g.Age += dt
		if g.Kind == Frag && g.Body.Grounded {
			// Rolling along the ground, it scrubs off speed: it settles
			// near where it landed rather than rolling away.
			g.Body.Velocity = g.Body.Velocity.Scale(float32(math.Exp(-fragDrag * float64(dt))))
		}
		other, touched := hit[g.Body]
		boom := false
		switch g.Kind {
		case GrenadeRound:
			boom = touched || a.touching(g) != nil
		case Sticky:
			if !g.Stuck {
				switch p := a.touching(g); {
				case p != nil:
					a.stick(g, p, ev)
				case touched && !isGrenade(other):
					a.stick(g, nil, ev)
				}
			}
			if g.StuckTo != nil && g.StuckTo.Dead {
				// Whoever it was on is down: it stays where they fell.
				g.Body.Position, g.StuckTo = g.Position(), nil
			}
		}
		pos := g.Position()
		if g.Fuse() <= 0 || boom || pos[1] < fallDeath {
			if !g.Stuck {
				a.Phys.Remove(g.Body)
			}
			if pos[1] >= fallDeath {
				a.explode(g, pos, ev)
			}
			continue
		}
		live = append(live, g)
	}
	clear(a.Grenades[len(live):])
	a.Grenades = live
}

// stick fixes a sticky where it is, or to player p.
func (a *Arena) stick(g *Grenade, p *Player, ev *Events) {
	a.Phys.Remove(g.Body)
	g.Stuck, g.stuckAt = true, g.Age
	if p != nil {
		g.StuckTo = p
		g.offset = g.Body.Position.Sub(p.Body.Position)
	}
	ev.Stuck = append(ev.Stuck, Stick{Grenade: g, On: p})
}

func isGrenade(b *physics.Body) bool {
	_, ok := b.UserData.(*Grenade)
	return ok
}

// touching is the other player whose hitbox a grenade has reached, if any.
func (a *Arena) touching(g *Grenade) *Player {
	for _, p := range a.Players {
		if p != g.Owner && !p.Dead && p.hitboxDist(g.Body.Position) <= grenadeRadius {
			return p
		}
	}
	return nil
}

// explode sets a grenade off at pos: damage and a shove in its radius,
// including players (rocket jumps work, and cost some health).
func (a *Arena) explode(g *Grenade, pos mathx.Vec3, ev *Events) {
	spec := grenadeSpecs[g.Kind]
	ev.Explosions = append(ev.Explosions, Explosion{At: pos, By: g.Owner, Kind: g.Kind})
	a.blast(pos, spec.Radius, spec.ChunkDamage, spec.PlayerDamage, spec.Push, mathx.Vec3{}, g.Owner, spec.Cause, ev)
}
