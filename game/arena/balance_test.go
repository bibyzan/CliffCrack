package arena

import (
	"testing"

	"CliffCrack/engine/mathx"
)

// hitsToKill counts the shots (each landing every pellet) that gun k takes
// to kill a fresh player, all to the body or all to the head, fired as fast
// as the gun allows (so armour never recharges).
func hitsToKill(t *testing.T, k WeaponKind, head bool) int {
	t.Helper()
	a, shooter, target := duel(10)
	g := Guns[k]
	for n := 1; n <= 200; n++ {
		var ev Events
		for range g.Pellets {
			a.hurtPlayer(target, shooter, g.Damage, head, k, mathx.Vec3{}, mathx.Vec3{}, &ev)
		}
		if target.Dead {
			return n
		}
	}
	t.Fatalf("%v never killed", k)
	return 0
}

// The balance is tuned around the rifle, after Halo: it takes most of a
// magazine; the pistol kills sooner, if you find the head; the shotgun
// kills in one up close; the sniper in two to the body, one to the head.
func TestTimeToKill(t *testing.T) {
	for _, c := range []struct {
		k          WeaponKind
		body, head int
	}{
		{WeaponRifle, 24, 19},
		{WeaponShotgun, 1, 1},
		{WeaponSniper, 2, 1},
	} {
		if got := hitsToKill(t, c.k, false); got != c.body {
			t.Errorf("%v: %d body hits to kill, want %d", WeaponNames[c.k], got, c.body)
		}
		if got := hitsToKill(t, c.k, true); got != c.head {
			t.Errorf("%v: %d headshots to kill, want %d", WeaponNames[c.k], got, c.head)
		}
	}
	// The SMG: as quick as the rifle to kill up close, faster firing, and
	// steadier from the hip; weaker past 12 m.
	smg := Guns[WeaponSMG]
	if got := hitsToKill(t, WeaponSMG, false); got != 32 {
		t.Errorf("SMG: %d body hits to kill, want 32", got)
	}
	if smgTTK, rifleTTK := float32(31)*smg.Interval, float32(23)*Guns[WeaponRifle].Interval; smgTTK > rifleTTK*1.05 {
		t.Errorf("SMG kills in %.2f s up close, rifle %.2f: want about as quick", smgTTK, rifleTTK)
	}
	if smg.Interval >= Guns[WeaponRifle].Interval || smg.HipSpread >= Guns[WeaponRifle].HipSpread || smg.Range >= Guns[WeaponRifle].Range {
		t.Error("the SMG should fire faster, spread less from the hip and reach less far than the rifle")
	}
	rifle := Guns[WeaponRifle]
	if frac := float32(24) / float32(rifle.Mag); frac < 0.6 || frac > 0.8 {
		t.Errorf("the rifle kills in %.0f%% of a magazine of perfect hits; want most of one, with room to miss", frac*100)
	}

	// The pistol: three to the body pop armour, then one to the head.
	a, shooter, target := duel(10)
	var ev Events
	pistol := Guns[WeaponPistol]
	for range 3 {
		a.hurtPlayer(target, shooter, pistol.Damage, false, WeaponPistol, mathx.Vec3{}, mathx.Vec3{}, &ev)
	}
	if !target.Popped() || target.Dead {
		t.Fatalf("three pistol shots: popped %v, dead %v; want popped", target.Popped(), target.Dead)
	}
	a.hurtPlayer(target, shooter, pistol.Damage, true, WeaponPistol, mathx.Vec3{}, mathx.Vec3{}, &ev)
	if !target.Dead {
		t.Error("a pistol headshot on a popped player should kill")
	}
	if got := hitsToKill(t, WeaponPistol, false); got != 5 {
		t.Errorf("pistol: %d body shots to kill, want 5", got)
	}
	// ... and with the headshot it beats the rifle's best.
	pistolTTK := 3 * pistol.Interval
	rifleTTK := float32(19-1) * rifle.Interval
	if pistolTTK >= rifleTTK {
		t.Errorf("pistol kill (with the headshot) %.2f s, rifle's best %.2f s: the pistol should be quicker for a steady hand", pistolTTK, rifleTTK)
	}
}

// The sniper's bolt is worked after each shot, and the scope drops for it.
func TestSniperBoltDropsScope(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	p.Weapons.Slots[0], p.Weapons.Active, p.Weapons.Current = WeaponSniper, 0, WeaponSniper
	g := Guns[WeaponSniper]
	run(a, g.ADSTime+0.2, Input{Aim: true})
	if p.ADS < 1 {
		t.Fatalf("ADS %v before firing, want the scope up", p.ADS)
	}
	run(a, 1.0/60, Input{Aim: true, Fire: true, FirePressed: true})
	run(a, 0.3, Input{Aim: true})
	if p.ADS > 0 {
		t.Errorf("ADS %v 0.3 s after the shot, want it down while the bolt's worked", p.ADS)
	}
	run(a, g.BoltTime, Input{Aim: true})
	if p.ADS < 1 {
		t.Errorf("ADS %v once the bolt's home, want the scope back up", p.ADS)
	}
}
