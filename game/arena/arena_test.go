package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

const frame = 1.0 / 60

// run steps the arena for seconds with the same inputs (one per player)
// every frame and merges the events.
func run(a *Arena, seconds float32, inputs ...Input) Events {
	var all Events
	for t := float32(0); t < seconds; t += frame {
		all.Merge(a.Step(frame, inputs))
	}
	return all
}

// flatArena is the site's ground and boundary with no structures, and
// players at the given spots.
func flatArena(structures []*Structure, spots ...Spawn) *Arena {
	site := GenerateSite(1)
	a := newArena(1, site.Blocks, structures, site.HalfSize, site.Spawns)
	for _, sp := range spots {
		a.AddPlayer(sp)
	}
	return a
}

// at is a spawn spot on the ground at x, z facing yaw (0 faces north, -Z).
func at(x, z, yaw float32) Spawn {
	return Spawn{At: mathx.Vec3{x, PlayerRadius + 0.02, z}, Yaw: yaw}
}

// duel is two players on open ground, dist apart on the Z axis, facing each other.
func duel(dist float32) (*Arena, *Player, *Player) {
	a := flatArena(nil, at(0, dist/2, 0), at(0, -dist/2, math.Pi))
	run(a, 0.3) // settle
	return a, a.Players[0], a.Players[1]
}

func TestWalkForwardOnTheFloor(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	run(a, 0.3) // settle
	start := p.Body.Position
	run(a, 1, Input{Move: [2]float32{0, 1}})
	pos := p.Body.Position
	if moved := start[2] - pos[2]; moved < 4.5 || moved > 7 {
		t.Errorf("walked %.2f m north in 1 s, want ~6", moved)
	}
	if math.Abs(float64(pos[0]-start[0])) > 0.05 {
		t.Errorf("drifted sideways to x = %v", pos[0])
	}
	if math.Abs(float64(pos[1]-PlayerRadius)) > 0.05 || !p.OnGround() {
		t.Errorf("should stay on the floor: y = %v, grounded %v", pos[1], p.OnGround())
	}

	// Letting go stops quickly.
	run(a, 0.5)
	if v := p.Body.Velocity.Len(); v > 0.2 {
		t.Errorf("still moving at %v m/s 0.5 s after letting go", v)
	}
}

func TestWallsStopThePlayer(t *testing.T) {
	a := flatArena(nil, at(0, siteHalf-6, math.Pi)) // facing south, towards the nearest wall
	run(a, 4, Input{Move: [2]float32{0, 1}, Sprint: true})
	if z := a.Players[0].Body.Position[2]; z > siteHalf-PlayerRadius+0.02 {
		t.Errorf("walked through the south wall: z = %v", z)
	}
}

func TestJumpAndLand(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	run(a, 0.3)
	ev := a.Step(frame, []Input{{Jump: true}})
	if !ev.Did(p, ActJump) {
		t.Fatal("jump from the ground should work")
	}
	peak := float32(0)
	var landed float32
	for range 90 {
		e := a.Step(frame, []Input{{Jump: true}}) // holding jump mustn't double-jump in the air
		peak = max(peak, p.Body.Position[1])
		landed = max(landed, e.Landed(p))
		if e.Did(p, ActJump) && landed == 0 {
			t.Fatal("jumped again in mid-air")
		}
	}
	if rise := peak - PlayerRadius; rise < 1.0 || rise > 1.6 {
		t.Errorf("jump height %.2f m, want ~1.3", rise)
	}
	if landed == 0 {
		t.Error("landing should be reported")
	}
}

func TestMagazineAndReload(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	p.Pitch = 0.5 // shoot at the sky
	ev := run(a, float32(MagSize)*fireInterval+0.05, Input{Fire: true})
	if len(ev.Shots) != MagSize {
		t.Errorf("fired %d shots from a full magazine, want %d", len(ev.Shots), MagSize)
	}
	if !ev.Did(p, ActReload) || p.Rifle.Reloading == 0 {
		t.Fatal("an empty magazine should start a reload")
	}
	if ev := run(a, 0.5, Input{Fire: true}); len(ev.Shots) != 0 {
		t.Error("can't fire while reloading")
	}
	run(a, ReloadTime)
	if p.Rifle.Ammo != MagSize || p.Rifle.Reloading != 0 {
		t.Errorf("after reloading: ammo %d, reloading %v", p.Rifle.Ammo, p.Rifle.Reloading)
	}

	// Manual reload of a partial magazine.
	run(a, 0.35, Input{Fire: true})
	if ev := a.Step(frame, []Input{{Reload: true}}); !ev.Did(p, ActReload) {
		t.Error("R with a partial magazine should reload")
	}
}

func TestRecoilClimbsAndRecovers(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	p.Pitch = 0.3
	run(a, 0.5, Input{Fire: true})
	if kick := p.ViewPitch() - p.Pitch; kick < 0.02 {
		t.Errorf("recoil after a burst = %v rad, want a noticeable climb", kick)
	}
	run(a, 1)
	if kick := p.ViewPitch() - p.Pitch; kick > 0.002 {
		t.Errorf("recoil should settle back, still %v", kick)
	}
}

func TestRayCapsule(t *testing.T) {
	a, b := mathx.Vec3{0, 0, 0}, mathx.Vec3{0, 1, 0}
	cases := []struct {
		name   string
		o, dir mathx.Vec3
		want   float32 // -1: miss
	}{
		{"side of the cylinder", mathx.Vec3{-5, 0.5, 0}, mathx.Vec3{1, 0, 0}, 4.6},
		{"top cap from above", mathx.Vec3{0, 5, 0}, mathx.Vec3{0, -1, 0}, 3.6},
		{"bottom cap edge", mathx.Vec3{-5, -0.2, 0}, mathx.Vec3{1, 0, 0}, 5 - float32(math.Sqrt(0.16-0.04))},
		{"passes beside", mathx.Vec3{-5, 0.5, 0.5}, mathx.Vec3{1, 0, 0}, -1},
		{"points away", mathx.Vec3{-5, 0.5, 0}, mathx.Vec3{-1, 0, 0}, -1},
	}
	for _, c := range cases {
		got, ok := rayCapsule(c.o, c.dir, a, b, 0.4)
		switch {
		case c.want < 0 && ok:
			t.Errorf("%s: hit at %v, want a miss", c.name, got)
		case c.want >= 0 && (!ok || math.Abs(float64(got-c.want)) > 1e-3):
			t.Errorf("%s: got %v (hit %v), want %v", c.name, got, ok, c.want)
		}
	}
}

func TestRifleKillsAPlayer(t *testing.T) {
	a, shooter, target := duel(12)
	// Level aim at the chest.
	shooter.Pitch = float32(math.Atan2(float64(target.Chest()[1]-shooter.Eye(1)[1]), 12))
	ev := run(a, 0.35, Input{Fire: true, FirePressed: true})
	hits := 0
	for _, s := range ev.Shots {
		if s.Victim == target {
			hits++
			if s.Head {
				t.Error("a chest shot counted as a headshot")
			}
		}
	}
	if hits == 0 || target.Health != MaxHealth-float32(hits)*RifleDamage {
		t.Fatalf("%d hits left the target at %v health", hits, target.Health)
	}
	if target.Flash == 0 {
		t.Error("a hit should flash the target")
	}

	ev.Merge(run(a, 2, Input{Fire: true}))
	if !target.Dead || len(ev.Kills) != 1 || ev.Kills[0].By != shooter || ev.Kills[0].Weapon != WeaponRifle {
		t.Fatalf("sustained fire should kill: dead %v, kills %+v, health %v", target.Dead, ev.Kills, target.Health)
	}
	if shooter.Kills != 1 || shooter.Damage != MaxHealth {
		t.Errorf("shooter credited with %d kills, %v damage", shooter.Kills, shooter.Damage)
	}
	// The dead can't be shot and don't move.
	pos := target.Body.Position
	ev = run(a, 0.3, Input{Fire: true}, Input{Move: [2]float32{0, 1}, Fire: true})
	for _, s := range ev.Shots {
		if s.Victim != nil || s.By == target {
			t.Fatalf("shot %+v after the target died", s)
		}
	}
	if target.Body.Position != pos {
		t.Error("a dead player moved")
	}
}

func TestHeadshotsHitHarder(t *testing.T) {
	a, shooter, target := duel(8)
	shooter.Pitch = float32(math.Atan2(float64(target.Head()[1]-shooter.Eye(1)[1]), 8))
	ev := a.Step(frame, []Input{{Fire: true, FirePressed: true}})
	if len(ev.Hurts) != 1 || !ev.Hurts[0].Head {
		t.Fatalf("a shot at the head: hurts %+v", ev.Hurts)
	}
	if want := float32(RifleDamage * headMult); math.Abs(float64(ev.Hurts[0].Damage-want)) > 1e-3 {
		t.Errorf("headshot damage %v, want %v", ev.Hurts[0].Damage, want)
	}
	if shooter.Headshots != 1 {
		t.Errorf("headshots %d, want 1", shooter.Headshots)
	}
}

func TestCoverBlocksShots(t *testing.T) {
	b := newBuilder("cover", mathx.Vec3{0, 0, 0}, 0)
	b.wall(-2.5, 0, 2.5, 0, 0, 3, 0.3, Metal) // metal: the rifle won't get through it
	a := flatArena([]*Structure{b.finish()}, at(0, 6, 0), at(0, -6, math.Pi))
	run(a, 0.3)
	ev := run(a, 1, Input{Fire: true})
	if len(ev.Shots) == 0 {
		t.Fatal("no shots fired")
	}
	if h := a.Players[1].Health; h != MaxHealth {
		t.Errorf("the player behind the wall took damage: health %v", h)
	}
	if a.CanSee(a.Players[0].Eye(1), a.Players[1].Head()) {
		t.Error("CanSee through a wall")
	}
}

func TestHammerTwoBlowsDownAPlayer(t *testing.T) {
	a, attacker, target := duel(2.2)
	attacker.Current = WeaponHammer
	attacker.Pitch = -0.1
	before := target.Body.Position
	ev := run(a, frame, Input{Fire: true})
	ev.Merge(run(a, HammerSwing))
	if len(ev.Smashes) != 1 || ev.Smashes[0].Victim != target {
		t.Fatalf("the blow should land on the player: smashes %+v", ev.Smashes)
	}
	if target.Health != MaxHealth-HammerPlayerDamage {
		t.Errorf("health after one blow %v", target.Health)
	}
	if pushed := before[2] - target.Body.Position[2]; pushed < 0.3 {
		t.Error("a hammer blow should knock the target back")
	}
	// Close in again and swing.
	target.Body.Position, target.Body.Velocity = attacker.Body.Position.Add(mathx.Vec3{0, 0, -2.2}), mathx.Vec3{}
	target.Body.Teleported()
	run(a, frame, Input{Fire: true})
	run(a, HammerSwing)
	if !target.Dead {
		t.Errorf("two blows should kill: health %v", target.Health)
	}
}

func TestGrenadesHurtAndSelfDamageIsHalved(t *testing.T) {
	a, shooter, target := duel(12)
	shooter.Current = WeaponLauncher
	// Lob it onto the target.
	muzzle := shooter.Eye(1).Add(shooter.Forward().Scale(0.7))
	dir := lobDirection(target.Chest().Sub(muzzle))
	shooter.Pitch = float32(math.Asin(float64(dir[1])))
	var ev Events
	for i := 0; i < 120 && len(ev.Explosions) == 0; i++ {
		ev.Merge(a.Step(frame, []Input{{Fire: i == 0, FirePressed: i == 0}}))
	}
	if len(ev.Explosions) != 1 {
		t.Fatalf("explosions %d", len(ev.Explosions))
	}
	if lost := MaxHealth - target.Health; lost < BlastPlayerDamage*0.6 {
		t.Errorf("a grenade on the target took only %v health (exploded at %v, target at %v)",
			lost, ev.Explosions[0].At, target.Body.Position)
	}

	// Your own grenade at your feet: a rocket jump, for half damage.
	a2 := flatArena(nil, at(0, 10, 0))
	p := a2.Players[0]
	run(a2, 0.3)
	p.Current, p.Pitch = WeaponLauncher, -1.4
	run(a2, frame, Input{Fire: true, FirePressed: true})
	peak := float32(0)
	for range 60 {
		a2.Step(frame, nil)
		peak = max(peak, p.Body.Position[1])
	}
	if peak < 1.8 {
		t.Errorf("a grenade at your feet should launch you: peak y %.2f", peak)
	}
	if lost := MaxHealth - p.Health; lost <= 0 || lost > BlastPlayerDamage*selfDamage+1e-3 {
		t.Errorf("rocket jump cost %v health, want some but at most %v", lost, BlastPlayerDamage*selfDamage)
	}
}
