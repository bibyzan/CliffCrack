package arena

import (
	"math/rand/v2"
	"testing"

	"CliffCrack/engine/mathx"
)

// testWall builds a flat arena with one player and one straight wall of the
// given material 2 m in front of them, facing them.
func testWall(m Material) (*Arena, *Player, *Structure) {
	const z = 10
	b := newBuilder("test wall", mathx.Vec3{0, 0, z - 2.4}, 0)
	b.wall(-2.5, 0, 2.5, 0, 0, 3, 0.3, m) // five panels: the middle one is straight ahead
	s := b.finish()
	a := flatArena([]*Structure{s}, at(0, z, 0))
	run(a, 0.2) // settle
	return a, a.Players[0], s
}

// swing brings out the hammer (the gadget) if it isn't already, swings it
// once and lets the swing finish.
func swing(a *Arena) Events {
	p := a.Players[0]
	p.Gadget = GadgetHammer
	var ev Events
	if !p.HammerOut {
		ev.Merge(run(a, frame, Input{Gadget: true}))
		ev.Merge(run(a, hammerDraw))
	}
	ev.Merge(run(a, frame, Input{Fire: true, FirePressed: true}))
	ev.Merge(run(a, HammerSwing))
	return ev
}

func TestHammerSmashesAHole(t *testing.T) {
	a, p, s := testWall(Wood)
	before := s.Alive()
	ev := swing(a)
	if !ev.Did(p, ActSwing) || len(ev.Smashes) != 1 || ev.Smashes[0].Mat != Wood {
		t.Fatalf("swing: swung %v, smashes %+v", ev.Did(p, ActSwing), ev.Smashes)
	}
	broken := before - s.Alive()
	if broken < 2 || broken > before/3 {
		t.Errorf("one blow broke %d of %d wooden panels, want a hole, not the whole wall", broken, before)
	}
	if len(a.Debris) == 0 {
		t.Error("broken panels should leave rubble")
	}
	if p.Destroyed != broken {
		t.Errorf("credited with %d destroyed for %d broken", p.Destroyed, broken)
	}
}

func TestConcreteTakesMoreBlowsThanWood(t *testing.T) {
	blowsToBreak := func(m Material) int {
		a, _, s := testWall(m)
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
	a, p, s := testWall(Wood)
	var target *Chunk
	for _, c := range s.Chunks { // the panel straight ahead at eye height
		if c.distTo(mathx.Vec3{0, p.Eye(1)[1], c.Centre[2]}) == 0 {
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

	g, _, gs := testWall(Glass)
	ev := run(g, frame, Input{Fire: true, FirePressed: true})
	if len(ev.Breaks) != 1 || ev.Breaks[0].Mat != Glass || gs.Alive() != len(gs.Chunks)-1 {
		t.Errorf("one round should shatter one glass pane: breaks %+v", ev.Breaks)
	}
}

func TestGrenadeBlastsAHole(t *testing.T) {
	a, p, s := testWall(Brick)
	arm(p, WeaponLauncher)
	p.Pitch = -0.25 // at the base of the wall
	var ev Events
	for i := 0; i < 90 && len(ev.Explosions) == 0; i++ {
		ev.Merge(a.Step(frame, []Input{{Fire: i == 0, FirePressed: i == 0}}))
	}
	if !ev.Did(p, ActLaunch) || len(ev.Explosions) != 1 {
		t.Fatalf("launched %v, explosions %d", ev.Did(p, ActLaunch), len(ev.Explosions))
	}
	if at := ev.Explosions[0].At; at[2] < s.Chunks[0].Centre[2]-0.5 || at[2] > s.Chunks[0].Centre[2]+0.6 {
		t.Errorf("grenade went off at %v, want at the wall (z ~ %v)", at, s.Chunks[0].Centre[2])
	}
	if ev.Explosions[0].By != p {
		t.Error("the explosion should be credited to the shooter")
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
	if p.Launcher.Ammo != LauncherMag-1 {
		t.Errorf("launcher ammo %d, want %d", p.Launcher.Ammo, LauncherMag-1)
	}
}

func TestBlowingOutTheGroundFloorCollapsesAHouse(t *testing.T) {
	house := House(rand.New(rand.NewPCG(9, 9)), mathx.Vec3{0, 0, 0}, 0)
	a := flatArena([]*Structure{house}, at(0, 20, 0))
	ev := Events{}
	// Detonate charges along the bottom of every wall.
	for _, c := range house.Chunks {
		if c.anchored && c.Alive {
			a.blast(c.Centre, 0.8, 1000, 0, 2, mathx.Vec3{}, nil, WeaponLauncher, &ev)
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
	run(a, 3)
	for _, d := range a.Debris {
		if d.Body.Position[1] < -0.1 {
			t.Fatalf("rubble fell through the floor: %v", d.Body.Position)
		}
	}
}

func TestTwoSlotsAndSwitching(t *testing.T) {
	a, p, _ := testWall(Wood)
	if p.Slots != [2]WeaponKind{WeaponRifle, WeaponPistol} || p.Current != WeaponRifle {
		t.Fatalf("start with the rifle in hand and the pistol: slots %v current %v", p.Slots, p.Current)
	}
	ev := a.Step(frame, []Input{{Select: 2}})
	if p.Current != WeaponPistol || !ev.Did(p, ActSwitch) || p.Switching == 0 {
		t.Fatalf("select slot 2: current %v switched %v", p.Current, ev.Did(p, ActSwitch))
	}
	if ev := a.Step(frame, []Input{{Fire: true, FirePressed: true}}); len(ev.Shots) != 0 {
		t.Error("can't fire while the other weapon is coming up")
	}
	run(a, SwitchTime)
	a.Step(frame, []Input{{Cycle: 1}})
	if p.Current != WeaponRifle {
		t.Errorf("swapping back: current %v, want the rifle", p.Current)
	}
	// The hammer comes out over whatever's held, and goes back to it.
	run(a, SwitchTime)
	if ev := swing(a); !ev.Did(p, ActSwing) || p.Current != WeaponRifle || p.Holding() != WeaponHammer {
		t.Errorf("hammer: swung %v, current %v, holding %v", ev.Did(p, ActSwing), p.Current, p.Holding())
	}
	a.Step(frame, []Input{{Cycle: 1}})
	if p.HammerOut || p.Current != WeaponRifle {
		t.Errorf("swap with the hammer out: hammer out %v, current %v; want it away and the rifle back", p.HammerOut, p.Current)
	}
}
