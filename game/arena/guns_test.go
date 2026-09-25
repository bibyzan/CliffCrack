package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

// aimAtTarget points shooter (dist away along Z) at the target's head or chest.
func aimAtTarget(shooter, target *Player, dist float32, head bool) {
	at := target.Chest()
	if head {
		at = target.Head()
	}
	shooter.Pitch = float32(math.Atan2(float64(at[1]-shooter.Eye(1)[1]), float64(dist)))
}

// shoot pulls the trigger once (holding aim) and lets the gun cycle.
func shoot(a *Arena, g *GunSpec) Events {
	ev := a.Step(frame, []Input{{Fire: true, FirePressed: true, Aim: true}})
	ev.Merge(run(a, g.Interval+0.02, Input{Aim: true}))
	return ev
}

// armedDuel is two players dist apart with the shooter holding k, sights up.
func armedDuel(k WeaponKind, dist float32) (*Arena, *Player, *Player) {
	a, shooter, target := duel(dist)
	arm(shooter, k)
	run(a, 0.4, Input{Aim: true})
	return a, shooter, target
}

func TestPistolPopsArmourThenAHeadshotKills(t *testing.T) {
	g := Guns[WeaponPistol]
	a, shooter, target := armedDuel(WeaponPistol, 15)
	aimAtTarget(shooter, target, 15, false)
	popped := false
	for range 3 {
		for _, h := range shoot(a, g).Hurts {
			popped = popped || h.Popped
		}
	}
	if !popped || !target.Popped() || target.Health != MaxHealth {
		t.Fatalf("three pistol shots to the body: popped %v, armour %v, health %v; want the armour gone and health untouched",
			popped, target.Shield, target.Health)
	}
	aimAtTarget(shooter, target, 15, true)
	ev := shoot(a, g)
	if !target.Dead || len(ev.Kills) != 1 || !ev.Kills[0].Head {
		t.Errorf("a headshot on a popped target should kill: dead %v, kills %+v", target.Dead, ev.Kills)
	}
}

func TestRifleHeadshotsDontKillThePopped(t *testing.T) {
	a, shooter, target := armedDuel(WeaponRifle, 8)
	target.Shield = 0
	aimAtTarget(shooter, target, 8, true)
	a.Step(frame, []Input{{Fire: true, FirePressed: true, Aim: true}})
	if target.Dead || target.Health >= MaxHealth {
		t.Errorf("a rifle headshot on a popped target should hurt, not kill: dead %v, health %v", target.Dead, target.Health)
	}
}

func TestSniperHeadshotAlwaysKills(t *testing.T) {
	a, shooter, target := armedDuel(WeaponSniper, 30)
	aimAtTarget(shooter, target, 30, true)
	shoot(a, Guns[WeaponSniper])
	if !target.Dead {
		t.Errorf("a sniper headshot through full armour should kill: armour %v, health %v", target.Shield, target.Health)
	}
	// Two to the body.
	a, shooter, target = armedDuel(WeaponSniper, 30)
	aimAtTarget(shooter, target, 30, false)
	shoot(a, Guns[WeaponSniper])
	if target.Dead || !target.Popped() || target.Health != MaxHealth {
		t.Fatalf("one body shot: dead %v, armour %v, health %v; want popped", target.Dead, target.Shield, target.Health)
	}
	shoot(a, Guns[WeaponSniper])
	if !target.Dead {
		t.Error("a second body shot should kill")
	}
}

func TestShotgunFallsOffWithRange(t *testing.T) {
	blast := func(dist float32) float32 {
		a, shooter, target := armedDuel(WeaponShotgun, dist)
		aimAtTarget(shooter, target, dist, false)
		shoot(a, Guns[WeaponShotgun])
		return MaxShield + MaxHealth - target.Durability()
	}
	close, far := blast(3), blast(20)
	if close < 100 || far > close/3 {
		t.Errorf("shotgun damage %v at 3 m and %v at 20 m: want a wreck up close and little far off", close, far)
	}
}

func TestADSTightensZoomsAndSlows(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	arm(p, WeaponPistol)
	run(a, 0.5)
	hip := p.Spread()
	run(a, 0.5, Input{Aim: true})
	if p.ADS != 1 || p.Spread() >= hip/3 || p.Zoom() < Guns[WeaponPistol].Zoom-1e-3 {
		t.Errorf("sights up: ADS %v, spread %v (hip %v), zoom %v", p.ADS, p.Spread(), hip, p.Zoom())
	}
	run(a, 1, Input{Aim: true, Move: [2]float32{0, 1}})
	if s := flat(p.Body.Velocity).Len(); s > walkSpeed*Guns[WeaponPistol].ADSMove+0.2 {
		t.Errorf("walking with the sights up at %.1f m/s, want slower", s)
	}
	// Sprinting drops them.
	run(a, 0.5, Input{Aim: true, Sprint: true, Move: [2]float32{0, 1}})
	if p.ADS != 0 {
		t.Errorf("ADS %v while sprinting", p.ADS)
	}
}

func TestHipfireBlooms(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	p.Pitch = 0.5
	run(a, 0.3)
	rest := p.Spread()
	run(a, 0.6, Input{Fire: true})
	if bloomed := p.Spread(); bloomed < rest*2.5 {
		t.Errorf("rifle spread %v after a burst from %v at rest: want it to bloom", bloomed, rest)
	}
	run(a, 1.5)
	if p.Spread() > rest+0.002 {
		t.Errorf("bloom should settle: %v", p.Spread())
	}
}

func TestArmourRecharges(t *testing.T) {
	a, _, target := duel(10)
	var ev Events
	a.hurtPlayer(target, nil, 70, false, WeaponRifle, mathx.Vec3{}, mathx.Vec3{}, &ev)
	run(a, shieldDelay-0.5)
	if target.Shield != MaxShield-70 {
		t.Fatalf("armour recharging too soon: %v", target.Shield)
	}
	run(a, 2.5)
	if target.Shield != MaxShield {
		t.Errorf("armour %v a while after the last hit, want it back to full", target.Shield)
	}
}

func TestGettingHitKnocksYouOutOfTheScope(t *testing.T) {
	a, _, target := armedDuel(WeaponRifle, 10)
	arm(target, WeaponSniper)
	run(a, 0.5, Input{}, Input{Aim: true})
	if target.ADS != 1 {
		t.Fatalf("target's scope not up: %v", target.ADS)
	}
	var ev Events
	a.hurtPlayer(target, nil, 5, false, WeaponRifle, mathx.Vec3{}, mathx.Vec3{}, &ev)
	run(a, 0.2, Input{}, Input{Aim: true})
	if target.ADS > 0.2 {
		t.Errorf("still scoped (%v) just after being hit", target.ADS)
	}
}

func TestRangeDummiesStrafeAndGetBackUp(t *testing.T) {
	m := NewRange()
	a := m.Arena
	if len(a.Players) != 1+len(rangeDummies) {
		t.Fatalf("%d players on the range", len(a.Players))
	}
	strafer := a.Players[2] // the 18 m dummy strafes
	start := strafer.Body.Position
	for range 120 {
		m.Step(frame, nil)
	}
	if strafer.Body.Position.Sub(start).Len() < 1 {
		t.Error("the strafing dummy stood still")
	}
	still := a.Players[1]
	var ev Events
	a.hurtPlayer(still, a.Players[0], 1e6, false, WeaponLauncher, mathx.Vec3{}, mathx.Vec3{}, &ev)
	for range 60 {
		m.Step(frame, nil)
	}
	if !still.Dead {
		t.Fatal("the dummy got up straight away")
	}
	for range 90 {
		m.Step(frame, nil)
	}
	if still.Dead || still.Durability() != MaxShield+MaxHealth || still.Body.Position.Sub(a.Spawns[1].At).Len() > 0.2 {
		t.Errorf("the dummy should be back up at its spot: dead %v, %v left, at %v", still.Dead, still.Durability(), still.Body.Position)
	}
	if m.Phase != PhaseFight {
		t.Errorf("practice has no rounds: phase %v", m.Phase)
	}
}
