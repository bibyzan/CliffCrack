package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// The firing range: a long platform over the chasm with a firing line at
// one end and practice dummies down range, a few strafing, and some walls
// and a bunker to put paint (and holes) in. Player 0 shoots from the line;
// the dummies are players 1 and up, driven by the match.
const (
	rangeHalfZ   = 14
	rangeFar     = -100 // x of the far end
	rangeNear    = 14   // x of the wall behind the firing line
	firingLine   = 8    // x the player starts at, facing -X
	dummyRevive  = 1.5  // s a downed dummy lies there
	playerRevive = 2.0  // s before you're back on the line if you fall
)

// Dummy is a target on the range: where it stands and whether it strafes.
type Dummy struct {
	Spawn  Spawn
	Strafe float32 // m/s side to side (0 stands still)
	phase  float32
}

// input is the dummy's movement at time t: sidestepping back and forth.
func (d Dummy) input(t float32) Input {
	if d.Strafe == 0 {
		return Input{}
	}
	s := float32(math.Sin(float64(t*0.9 + d.phase)))
	return Input{Move: [2]float32{float32(math.Copysign(float64(min(d.Strafe/walkSpeed, 1)), float64(s))), 0}}
}

// Distance is how far down range the dummy stands from the firing line.
func (d Dummy) Distance() float32 { return firingLine - d.Spawn.At[0] }

// rangeDummies stand at these distances (m) and lanes (z), some strafing.
var rangeDummies = []struct {
	dist, z, strafe float32
}{
	{10, -4, 0}, {18, 4, 3}, {30, -6, 0}, {45, 3, 4}, {60, -2, 0}, {85, 5, 0},
}

// GenerateRange builds the firing range.
func GenerateRange() *Site {
	s := &Site{Bounds: [2]float32{-rangeFar, rangeHalfZ}}
	add := func(k BlockKind, lo, hi mathx.Vec3) {
		s.add(k, lo.Add(hi).Scale(0.5), hi.Sub(lo).Scale(0.5))
	}
	v := func(x, y, z float32) mathx.Vec3 { return mathx.Vec3{x, y, z} }
	add(Floor, v(rangeFar, -1, -rangeHalfZ), v(rangeNear, 0, rangeHalfZ))
	add(Wall, v(rangeNear, 0, -rangeHalfZ), v(rangeNear+1, 4, rangeHalfZ)) // behind the line
	for _, sz := range []float32{-1, 1} {
		add(Wall, v(rangeFar, 0, sz*rangeHalfZ-0.5), v(rangeNear, 1.1, sz*rangeHalfZ+0.5)) // the parapets
		// A post every 10 m down range, to judge distance by.
		for d := float32(10); firingLine-d > rangeFar; d += 10 {
			x := firingLine - d
			add(Wall, v(x-0.2, 1.1, sz*(rangeHalfZ-0.3)-0.2), v(x+0.2, 2.6, sz*(rangeHalfZ-0.3)+0.2))
		}
	}
	add(Wall, v(rangeFar-1, 0, -rangeHalfZ), v(rangeFar, 6, rangeHalfZ)) // the backstop
	// A board straight down the line at 12 m, to read your grouping and
	// bloom off the paint.
	add(Wall, v(firingLine-12.1, 0.5, -0.8), v(firingLine-12, 2.1, 0.8))

	s.Spawns = []Spawn{{At: v(firingLine, PlayerRadius+0.02, 0), Yaw: -math.Pi / 2}}
	for _, d := range rangeDummies {
		s.Spawns = append(s.Spawns, Spawn{At: v(firingLine-d.dist, PlayerRadius+0.02, d.z), Yaw: math.Pi / 2})
	}

	// Things to shoot through: walls of each material, and a bunker.
	rng := rand.New(rand.NewPCG(5, 6))
	for i, m := range []Material{Wood, Brick, Concrete} {
		b := newBuilder(Materials[m].Name+" wall", v(firingLine-14-float32(i)*12, 0, 9), 1)
		b.wall(-2.5, 0, 2.5, 0, 0, 2.6, 0.3, m)
		s.Structures = append(s.Structures, b.finish())
	}
	s.Structures = append(s.Structures, Bunker(rng, v(firingLine-40, 0, -9), 0), Glasshouse(rng, v(firingLine-55, 0, 9), 0))

	// A movement course down the right-hand side, to practise on: a
	// waist-high barrier to vault, a 2 m ledge to climb (and drop off), and
	// a low roof to slide under.
	const cz0, cz1 = -13, -9.5
	add(Bay, v(1.6, 0, cz0), v(2, 1.0, cz1))       // the barrier
	add(Bay, v(-7.5, 0, cz0), v(-4, 2.0, cz1))     // the ledge
	add(Bay, v(-17, 1.35, cz0), v(-12, 1.55, cz1)) // the roof ...
	add(Bay, v(-17, 0, cz0), v(-16.7, 1.35, cz0+0.3))
	add(Bay, v(-12.3, 0, cz0), v(-12, 1.35, cz0+0.3)) // ... on two posts at the back

	// The weapon table, to the left of the line: every gun, crates of both
	// grenades and both gadgets, none of which run out.
	add(Bay, v(tableX0, 0, tableZ-0.6), v(tableX1, tableH, tableZ+0.6))
	items := []Pickup{
		weaponPickup(WeaponRifle, mathx.Vec3{}), weaponPickup(WeaponPistol, mathx.Vec3{}),
		weaponPickup(WeaponShotgun, mathx.Vec3{}), weaponPickup(WeaponSniper, mathx.Vec3{}),
		weaponPickup(WeaponSMG, mathx.Vec3{}), weaponPickup(WeaponRevolver, mathx.Vec3{}),
		weaponPickup(WeaponLauncher, mathx.Vec3{}),
		grenadePickup(Frag, MaxGrenades, mathx.Vec3{}), grenadePickup(Sticky, MaxGrenades, mathx.Vec3{}),
		gadgetPickup(GadgetHammer), gadgetPickup(GadgetGrapple),
	}
	for i, p := range items {
		p.At = v(tableX1-0.5-float32(i)*(tableX1-tableX0-1)/float32(len(items)-1), tableH, tableZ)
		p.Yaw, p.Table = math.Pi/2, true
		s.Spots = append(s.Spots, PickupSpot{Pickup: p})
	}
	return s
}

// The weapon table's extent.
const (
	tableX0, tableX1 = 4, 11
	tableZ, tableH   = 3.4, 0.9
)

// NewRange starts a practice match on the firing range: no rounds, no
// clock, and downed dummies (or you) get back up.
func NewRange() *Match {
	a := newArena(1, GenerateRange())
	for _, sp := range a.Spawns {
		a.AddPlayer(sp)
	}
	m := &Match{Arena: a, Players: len(a.Players), Wins: make([]int, len(a.Players)), Round: 1,
		Phase: PhaseFight, RoundWinner: -1, Winner: -1, Practice: true}
	a.Live, a.FreeAmmo = true, true
	a.Players[0].Grenades = [GrenadeKinds]int{Frag: MaxGrenades, Sticky: MaxGrenades}
	for i, d := range rangeDummies {
		m.Dummies = append(m.Dummies, Dummy{Spawn: a.Spawns[i+1], Strafe: d.strafe, phase: float32(i) * 1.7})
	}
	return m
}

// stepRange advances a practice match: the dummies strafe, and whoever is
// down gets back up after a moment.
func (m *Match) stepRange(dt float32, inputs []Input) Events {
	a := m.Arena
	full := make([]Input, len(a.Players))
	copy(full, inputs)
	for i, d := range m.Dummies {
		full[i+1] = d.input(a.Time)
	}
	ev := a.Step(dt, full)
	for i, p := range a.Players {
		wait := float32(dummyRevive)
		if i == 0 {
			wait = playerRevive
		}
		if p.Dead && a.Time-p.DiedAt > wait {
			a.Revive(p, a.Spawns[i])
		}
	}
	return ev
}

// Revive puts a downed player back on their feet at sp, armour and health
// full and every weapon loaded, still carrying what they had.
func (a *Arena) Revive(p *Player, sp Spawn) {
	slots, active, grenades := p.Slots, p.Active, p.Grenades
	p.Dead, p.Shield, p.Health, p.Flash = false, MaxShield, MaxHealth, 0
	p.Yaw, p.Pitch, p.recoil = sp.Yaw, 0, 0
	p.Weapons = newWeapons()
	p.Slots, p.Active, p.Current, p.Grenades = slots, active, slots[active], grenades
	p.Body.Position, p.Body.Velocity = sp.At, mathx.Vec3{}
	p.Body.Teleported()
	p.lastHitBy, p.sinceHurt = nil, 0
	if err := a.Phys.Add(p.Body); err != nil {
		panic(err)
	}
}
