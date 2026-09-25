package arena

import (
	"math/rand/v2"
	"testing"
	"time"

	"CliffCrack/engine/mathx"
)

// testWall builds an arena (the fixed level, no drones) with one straight
// wall of the given material 2 m in front of the spawn, facing the player.
func testWall(m Material) (*Arena, *Structure) {
	b := newBuilder("test wall", mathx.Vec3{0, 0, Spawn[2] - 2.4}, 0)
	b.wall(-2.5, 0, 2.5, 0, 0, 3, 0.3, m) // five panels: the middle one is straight ahead
	s := b.finish()
	a := newArena(1, Level(), []*Structure{s}, HalfSize, Spawn)
	run(a, 0.2, Input{}) // settle
	return a, s
}

// swing presses fire for one hammer swing and lets it finish.
func swing(a *Arena) Events {
	ev := run(a, frame, Input{Fire: true})
	more := run(a, HammerSwing, Input{})
	ev.Breaks = append(ev.Breaks, more.Breaks...)
	ev.Smashes = append(ev.Smashes, more.Smashes...)
	return ev
}

func TestHammerSmashesAHole(t *testing.T) {
	a, s := testWall(Wood)
	a.Current = WeaponHammer
	before := s.Alive()
	ev := swing(a)
	if !ev.Swung || len(ev.Smashes) != 1 || ev.Smashes[0].Mat != Wood {
		t.Fatalf("swing: swung %v, smashes %+v", ev.Swung, ev.Smashes)
	}
	broken := before - s.Alive()
	if broken < 2 || broken > before/3 {
		t.Errorf("one blow broke %d of %d wooden panels, want a hole, not the whole wall", broken, before)
	}
	if len(a.Debris) == 0 {
		t.Error("broken panels should leave rubble")
	}
	if a.Destroyed != broken || a.Score != broken*ChunkScore {
		t.Errorf("destroyed %d score %d for %d broken", a.Destroyed, a.Score, broken)
	}
}

func TestConcreteTakesMoreBlowsThanWood(t *testing.T) {
	blowsToBreak := func(m Material) int {
		a, s := testWall(m)
		a.Current = WeaponHammer
		before := s.Alive()
		for n := 1; n <= 10; n++ {
			swing(a)
			if s.Alive() < before {
				return n
			}
		}
		return 99
	}
	wood, concrete := blowsToBreak(Wood), blowsToBreak(Concrete)
	if wood != 1 {
		t.Errorf("wood took %d blows to break, want 1", wood)
	}
	if concrete <= wood || concrete > 4 {
		t.Errorf("concrete took %d blows (wood %d), want a couple more but not forever", concrete, wood)
	}
}

func TestRifleChipsAndGlassShatters(t *testing.T) {
	a, s := testWall(Wood)
	a.Current = WeaponRifle
	var target *Chunk
	for _, c := range s.Chunks { // the panel straight ahead at eye height
		if c.distTo(mathx.Vec3{0, a.Player.Eye(1)[1], c.Centre[2]}) == 0 {
			target = c
		}
	}
	if target == nil {
		t.Fatal("no panel at eye height")
	}
	run(a, 0.25, Input{Fire: true}) // a couple of rounds: chipped, not broken
	if !target.Alive || target.Health() >= 1 {
		t.Fatalf("after a short burst the panel should be damaged but standing: alive %v health %.2f", target.Alive, target.Health())
	}
	run(a, 1, Input{Fire: true})
	if target.Alive {
		t.Error("a full second of fire should break a wooden panel")
	}

	g, gs := testWall(Glass)
	g.Current = WeaponRifle
	ev := run(g, frame, Input{Fire: true, FirePressed: true})
	if len(ev.Breaks) != 1 || ev.Breaks[0].Mat != Glass || gs.Alive() != len(gs.Chunks)-1 {
		t.Errorf("one round should shatter one glass pane: breaks %+v", ev.Breaks)
	}
}

func TestGrenadeBlastsAHoleAndCollapsesTheTop(t *testing.T) {
	a, s := testWall(Brick)
	a.Current = WeaponLauncher
	a.Player.Pitch = -0.25 // at the base of the wall
	var ev Events
	for i := 0; i < 90 && len(ev.Explosions) == 0; i++ {
		e := a.Step(frame, Input{Fire: i == 0, FirePressed: i == 0})
		ev.Launched = ev.Launched || e.Launched
		ev.Explosions = append(ev.Explosions, e.Explosions...)
		ev.Breaks = append(ev.Breaks, e.Breaks...)
	}
	if !ev.Launched || len(ev.Explosions) != 1 {
		t.Fatalf("launched %v, explosions %d", ev.Launched, len(ev.Explosions))
	}
	if at := ev.Explosions[0]; at[2] < s.Chunks[0].Centre[2]-0.5 || at[2] > s.Chunks[0].Centre[2]+0.6 {
		t.Errorf("grenade went off at %v, want at the wall (z ~ %v)", at, s.Chunks[0].Centre[2])
	}
	broken := 0
	for _, b := range ev.Breaks {
		if !b.Collapsed {
			broken++
		}
	}
	if broken < 6 {
		t.Errorf("a grenade broke only %d brick panels", broken)
	}
	if a.Launcher.Ammo != LauncherMag-1 {
		t.Errorf("launcher ammo %d, want %d", a.Launcher.Ammo, LauncherMag-1)
	}
}

func TestBlowingOutTheGroundFloorCollapsesAHouse(t *testing.T) {
	house := House(rand.New(rand.NewPCG(9, 9)), mathx.Vec3{0, 0, 0}, 0)
	a := newArena(1, Level()[:1], []*Structure{house}, HalfSize, mathx.Vec3{0, PlayerRadius, 20})
	ev := Events{}
	// Detonate charges along the bottom of every wall.
	for _, c := range house.Chunks {
		if c.anchored && c.Alive {
			a.blast(c.Centre, 0.8, 1000, 2, mathx.Vec3{}, false, &ev)
		}
	}
	a.settle(&ev)
	if house.Alive() != 0 {
		t.Errorf("%d chunks still standing with nothing under them", house.Alive())
	}
	collapsed := 0
	for _, b := range ev.Breaks {
		if b.Collapsed {
			collapsed++
		}
	}
	if collapsed == 0 {
		t.Error("the upper parts should come down as collapsing rubble")
	}
	// The rubble falls and comes to rest on the floor.
	run(a, 3, Input{})
	for _, d := range a.Debris {
		if d.Body.Position[1] < -0.1 {
			t.Fatalf("rubble fell through the floor: %v", d.Body.Position)
		}
	}
}

func TestRocketJump(t *testing.T) {
	a, _ := testWall(Concrete)
	a.Current = WeaponLauncher
	a.Player.Pitch = -1.4 // straight down at your feet
	run(a, frame, Input{Fire: true, FirePressed: true})
	peak := float32(0)
	for range 60 {
		a.Step(frame, Input{})
		peak = max(peak, a.Player.Body.Position[1])
	}
	if peak < 1.8 {
		t.Errorf("a grenade at your feet should launch you: peak y %.2f", peak)
	}
}

func TestWeaponSwitching(t *testing.T) {
	a, _ := testWall(Wood)
	a.Current = WeaponRifle
	ev := a.Step(frame, Input{Select: 3})
	if a.Current != WeaponLauncher || !ev.Switched || a.Switching == 0 {
		t.Fatalf("select 3: current %v switched %v", a.Current, ev.Switched)
	}
	if ev := a.Step(frame, Input{Fire: true, FirePressed: true}); ev.Launched {
		t.Error("can't fire while the new weapon is coming up")
	}
	run(a, SwitchTime, Input{})
	a.Step(frame, Input{Cycle: 1}) // wraps round to the hammer
	if a.Current != WeaponHammer {
		t.Errorf("cycling past the last slot: current %v, want the hammer", a.Current)
	}
	a.Step(frame, Input{Cycle: -1})
	if a.Current != WeaponLauncher {
		t.Errorf("cycling back: current %v, want the launcher", a.Current)
	}
}

func TestDemolitionBotLevelsTheSite(t *testing.T) {
	a := NewSite(11)
	start := time.Now()
	for range 60 * 30 { // 30 s
		a.Step(frame, a.Autopilot(frame))
	}
	elapsed := time.Since(start)
	if a.Destroyed < 60 {
		t.Errorf("the bot destroyed only %d chunks in 30 s", a.Destroyed)
	}
	t.Logf("destroyed %d chunks (%.0f%% of the site standing), %d debris, 30 s simulated in %v",
		a.Destroyed, a.Standing()*100, len(a.Debris), elapsed)
	if elapsed > 15*time.Second {
		t.Errorf("simulation too slow: 30 s took %v", elapsed)
	}
}
