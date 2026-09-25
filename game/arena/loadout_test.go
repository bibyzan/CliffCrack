package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func TestFragBouncesAndGoesOffOnItsFuse(t *testing.T) {
	a, thrower, target := duel(10)
	thrower.Pitch = -0.15
	ev := a.Step(frame, []Input{{Throw: true}})
	if !ev.Did(thrower, ActThrow) || thrower.Grenades[Frag] != 1 || len(a.Grenades) != 1 {
		t.Fatalf("throw: did %v, frags left %d, in flight %d", ev.Did(thrower, ActThrow), thrower.Grenades[Frag], len(a.Grenades))
	}
	ev = run(a, grenadeSpecs[Frag].Fuse-0.2)
	if len(ev.Explosions) != 0 {
		t.Fatal("a frag went off before its fuse")
	}
	ev = run(a, 0.4)
	if len(ev.Explosions) != 1 || ev.Explosions[0].Kind != Frag {
		t.Fatalf("explosions %+v, want the frag on its fuse", ev.Explosions)
	}
	if lost := MaxShield + MaxHealth - target.Durability(); lost <= 0 {
		t.Errorf("a frag landing near the target did no damage (went off at %v, target at %v)", ev.Explosions[0].At, target.Body.Position)
	}
}

func TestStickySticksToAPlayerAndKills(t *testing.T) {
	a, thrower, target := duel(6)
	thrower.GrenadeKind = Sticky
	thrower.Pitch = float32(math.Atan2(float64(target.Chest()[1]-thrower.Eye(1)[1]), 6)) - 0.02
	var ev Events
	for i := 0; i < 60 && len(ev.Stuck) == 0; i++ {
		ev.Merge(a.Step(frame, []Input{{Throw: i == 0}}))
	}
	if len(ev.Stuck) != 1 || ev.Stuck[0].On != target {
		t.Fatalf("stuck %+v: want it on the target", ev.Stuck)
	}
	// It goes where they go, and then off.
	ev.Merge(run(a, grenadeSpecs[Sticky].Fuse+0.1, Input{}, Input{Move: [2]float32{1, 0}}))
	if len(ev.Explosions) != 1 || !target.Dead {
		t.Errorf("explosions %d, target dead %v: a sticky on you should finish you", len(ev.Explosions), target.Dead)
	}
	if len(ev.Kills) != 1 || ev.Kills[0].Weapon != WeaponSticky || ev.Kills[0].By != thrower {
		t.Errorf("kill %+v: want it credited to the thrower, as a sticky", ev.Kills)
	}
}

func TestStickySticksToWalls(t *testing.T) {
	a, p, _ := testWall(Concrete)
	p.GrenadeKind = Sticky
	var ev Events
	for i := 0; i < 60 && len(ev.Stuck) == 0; i++ {
		ev.Merge(a.Step(frame, []Input{{Throw: i == 0}}))
	}
	if len(ev.Stuck) != 1 || ev.Stuck[0].On != nil {
		t.Fatalf("stuck %+v: want it on the wall", ev.Stuck)
	}
	at := a.Grenades[0].Position()
	run(a, 0.5)
	if len(a.Grenades) != 1 || a.Grenades[0].Position() != at {
		t.Error("a stuck sticky should stay put until it goes off")
	}
}

// onTable puts the range's player at the weapon table, by pickup of kind k.
func onTable(t *testing.T, m *Match, k WeaponKind) *Pickup {
	t.Helper()
	a, p := m.Arena, m.Arena.Players[0]
	for _, q := range a.Pickups {
		if q.Weapon == k && q.Table {
			p.Body.Position = q.At.Add(mathx.Vec3{0, 0, -1.2})
			p.Body.Position[1] = PlayerRadius + 0.02
			p.Body.Teleported()
			return q
		}
	}
	t.Fatalf("no %v on the table", k)
	return nil
}

func TestPickingUpSwapsTheWeaponInHand(t *testing.T) {
	m := NewRange()
	a, p := m.Arena, m.Arena.Players[0]
	onTable(t, m, WeaponSniper)
	m.Step(frame, nil)
	if q := a.NearestPickup(p); q == nil || q.Weapon != WeaponSniper {
		t.Fatalf("nearest pickup %+v, want the sniper", q)
	}
	pickups := len(a.Pickups)
	ev := m.Step(frame, []Input{{Interact: true}})
	if !ev.Did(p, ActPickup) || p.Current != WeaponSniper || p.Slots != [2]WeaponKind{WeaponSniper, WeaponPistol} {
		t.Fatalf("after picking up: slots %v, current %v", p.Slots, p.Current)
	}
	// The rifle's on the floor now; the table's sniper is still there.
	if len(a.Pickups) != pickups+1 {
		t.Errorf("%d pickups, want one more (the dropped rifle)", len(a.Pickups))
	}
	dropped := a.Pickups[len(a.Pickups)-1]
	if dropped.Weapon != WeaponRifle || dropped.Table {
		t.Errorf("dropped %+v, want the rifle", dropped)
	}
	// Take the rifle back, dropping the sniper.
	m.Step(frame, []Input{{Interact: true}})
	if p.Current != WeaponRifle && p.Current != WeaponSniper {
		t.Errorf("current %v", p.Current)
	}
}

func TestWalkingOverAmmoAndGrenades(t *testing.T) {
	a, p, _ := testWall(Wood)
	p.States[WeaponRifle].Reserve = 0
	p.Grenades[Frag] = 0
	here := p.Body.Position.Sub(mathx.Vec3{0, PlayerRadius, 0})
	a.Pickups = append(a.Pickups, &Pickup{Weapon: WeaponRifle, Ammo: 48, Reserve: 20, At: here, spot: -1},
		&Pickup{Weapon: NoWeapon, Grenade: Frag, Count: 2, At: here, spot: -1})
	run(a, frame)
	if p.States[WeaponRifle].Reserve != 68 || p.Grenades[Frag] != 2 || len(a.Pickups) != 0 {
		t.Errorf("reserve %d, frags %d, pickups left %d: want the ammo and grenades taken",
			p.States[WeaponRifle].Reserve, p.Grenades[Frag], len(a.Pickups))
	}
}

func TestOutOfReserveYouCantReload(t *testing.T) {
	a, p, _ := testWall(Wood)
	p.Pitch = 0.5
	s := &p.States[WeaponRifle]
	s.Reserve = 10
	run(a, 8, Input{Fire: true})
	if s.Ammo != 0 || s.Reserve != 0 || s.Reloading != 0 {
		t.Errorf("after emptying the lot: ammo %d reserve %d reloading %v", s.Ammo, s.Reserve, s.Reloading)
	}
}

func TestArenaPickupsSpawnMirroredAndComeBack(t *testing.T) {
	a := New(3, 1, 0)
	if len(a.Spots) == 0 || len(a.Spots)%2 != 0 {
		t.Fatalf("%d pickup spots, want mirrored pairs", len(a.Spots))
	}
	for i := 0; i < len(a.Spots); i += 2 {
		s, n := a.Spots[i].Pickup, a.Spots[i+1].Pickup
		if s.Weapon != n.Weapon || s.At.Sub(mathx.Vec3{-n.At[0], n.At[1], -n.At[2]}).Len() > 1e-3 {
			t.Errorf("spot pair %d: %v at %v and %v at %v", i/2, s.Name(), s.At, n.Name(), n.At)
		}
	}
	run(a, 0.5)
	for _, p := range a.Pickups {
		if !groundBelow(a, p.At.Add(mathx.Vec3{0, 0.3, 0}), 0.4) {
			t.Errorf("%s at %v isn't resting on anything", p.Name(), p.At)
		}
	}
	// Take one and it comes back.
	p := a.Players[0]
	var target *Pickup
	for _, q := range a.Pickups {
		if q.Weapon != NoWeapon {
			target = q
			break
		}
	}
	p.Body.Position = target.At.Add(mathx.Vec3{0, PlayerRadius + 0.02, 0.5})
	p.Body.Teleported()
	run(a, 0.1)
	a.Step(frame, []Input{{Interact: true}})
	if !p.Holds(target.Weapon) {
		t.Fatalf("didn't pick up the %s", target.Name())
	}
	count := func(k WeaponKind) (n int) {
		for _, q := range a.Pickups {
			if q.Weapon == k && q.spot >= 0 {
				n++
			}
		}
		return
	}
	before := count(target.Weapon)
	p.Body.Position = mathx.Vec3{0, PlayerRadius + 0.02, 20} // away, or they'd take its ammo straight off
	p.Body.Teleported()
	a.Time += respawnFor(*target) + 1
	a.Step(frame, nil)
	if count(target.Weapon) != before+1 {
		t.Errorf("the %s didn't come back", target.Name())
	}
}

func TestScopeSwaysUnlessYouHoldYourBreath(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	arm(p, WeaponSniper)
	run(a, 1.5, Input{Aim: true})
	if s := abs(p.sway[0]) + abs(p.sway[1]); s < 1e-4 {
		t.Fatalf("scoped in, the aim should drift: sway %v", p.sway)
	}
	run(a, 0.8, Input{Aim: true, Sprint: true, Move: [2]float32{0, 0}})
	if s := abs(p.sway[0]) + abs(p.sway[1]); s > 3e-4 || p.ADS < 1 || p.Breath >= maxBreath {
		t.Errorf("holding breath: sway %v, ADS %v, breath %v; want it steady, still scoped, and breath used", p.sway, p.ADS, p.Breath)
	}
}
