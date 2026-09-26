package arena

import (
	"testing"

	"CliffCrack/engine/mathx"
)

var (
	forward = Input{Move: [2]float32{0, 1}}
	sprint  = Input{Move: [2]float32{0, 1}, Sprint: true}
)

// obstacle is a flat arena with one player at z = 10 facing north (-Z) and
// a box across their way: from z0 back (north) depth, height tall.
func obstacle(z0, depth, height float32) (*Arena, *Player) {
	b := newBuilder("obstacle", mathx.Vec3{}, 0)
	b.box(mathx.Vec3{-3, 0, z0 - depth}, mathx.Vec3{3, height, z0}, Concrete)
	a := flatArena([]*Structure{b.finish()}, at(0, 10, 0))
	run(a, 0.3) // settle
	return a, a.Players[0]
}

func speedOf(p *Player) float32 { return flat(p.Body.Velocity).Len() }

func TestCrouch(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0))
	p := a.Players[0]
	run(a, 0.3)
	eye, head := p.Eye(1)[1], p.Head()[1]
	run(a, 0.5, Input{Crouch: true})
	if drop := eye - p.Eye(1)[1]; drop < CrouchDrop*0.95 {
		t.Errorf("crouched, the eye dropped %.2f m, want %.2f", drop, float32(CrouchDrop))
	}
	if drop := head - p.Head()[1]; drop < CrouchDrop*0.95 {
		t.Errorf("crouched, the head dropped %.2f m, want %.2f (a smaller target)", drop, float32(CrouchDrop))
	}
	run(a, 0.6, Input{Crouch: true, Move: [2]float32{0, 1}})
	if s := speedOf(p); s > crouchSpeed+0.1 || s < crouchSpeed-0.5 {
		t.Errorf("crouch-walking at %.1f m/s, want ~%.1f", s, float32(crouchSpeed))
	}
	run(a, 0.5)
	if p.Eye(1)[1] < eye-0.02 {
		t.Error("let go of crouch: should stand back up")
	}
}

func TestSlide(t *testing.T) {
	a := flatArena(nil, at(0, 30, 0))
	p := a.Players[0]
	run(a, 0.3)
	run(a, 1, sprint)
	before := speedOf(p)
	crouch := Input{Move: [2]float32{0, 1}, Sprint: true, Crouch: true}
	ev := run(a, frame, crouch)
	if !p.Sliding || len(ev.Actions) == 0 {
		t.Fatalf("crouching at a sprint (%.1f m/s) should slide", before)
	}
	if s := speedOf(p); s < slideStart-0.1 || s < before+1 {
		t.Errorf("a slide from %.1f m/s starts at %.1f, want a burst past the sprint", before, s)
	}
	run(a, 0.4, crouch)
	if s := speedOf(p); !p.Sliding || s <= sprintSpeed {
		t.Errorf("0.4 s into a slide: sliding %v at %.1f m/s, want still sliding faster than a sprint", p.Sliding, s)
	}
	run(a, slideMax, crouch)
	if p.Sliding {
		t.Error("the slide should have run out")
	}
	if !p.crouched {
		t.Error("still holding crouch after a slide: should be crouched")
	}

	// Walking pace doesn't slide; letting go ends one.
	a2 := flatArena(nil, at(0, 30, 0))
	q := a2.Players[0]
	run(a2, 0.3)
	run(a2, 1, forward)
	run(a2, frame, Input{Move: [2]float32{0, 1}, Crouch: true})
	if q.Sliding {
		t.Error("crouching at walking pace slid")
	}
}

func TestSlideJumpKeepsSpeed(t *testing.T) {
	a := flatArena(nil, at(0, 30, 0))
	p := a.Players[0]
	run(a, 0.3)
	run(a, 1, sprint)
	crouch := Input{Move: [2]float32{0, 1}, Sprint: true, Crouch: true}
	run(a, 0.15, crouch)
	fast := speedOf(p)
	run(a, frame, Input{Move: [2]float32{0, 1}, Sprint: true, Jump: true})
	run(a, 0.3, sprint)
	if p.Sliding || p.OnGround() {
		t.Fatalf("jumped out of a slide: sliding %v, on the ground %v", p.Sliding, p.OnGround())
	}
	if s := speedOf(p); s < fast-1 {
		t.Errorf("a slide jump at %.1f m/s is going %.1f in the air: want the speed kept", fast, s)
	}
}

func TestVault(t *testing.T) {
	// A waist-high wall, 0.3 m thick, 3 m ahead.
	a, p := obstacle(7, 0.3, 1.0)
	var vaulted bool
	for range 90 {
		ev := run(a, frame, sprint)
		for _, act := range ev.Actions {
			vaulted = vaulted || act.Kind == ActVault
		}
	}
	if !vaulted {
		t.Fatal("sprinting at a 1 m wall: no vault")
	}
	if z := p.Body.Position[2]; z > 6.7-PlayerRadius {
		t.Errorf("after vaulting, at z %.2f: want past the wall (z < %.2f)", z, 6.7-PlayerRadius)
	}
	if s := speedOf(p); s < walkSpeed {
		t.Errorf("after the vault going %.1f m/s: want the pace kept", s)
	}
}

func TestClimb(t *testing.T) {
	// A 2 m platform, 3 m deep, starting 2 m ahead.
	a, p := obstacle(8, 3, 2.0)
	run(a, 0.5, forward) // walk up to it
	var climbed bool
	for range 60 {
		ev := run(a, frame, Input{Move: [2]float32{0, 1}, Jump: true})
		for _, act := range ev.Actions {
			climbed = climbed || act.Kind == ActClimb
		}
	}
	run(a, 0.5, forward)
	if !climbed {
		t.Fatal("jumping at a 2 m ledge: no climb")
	}
	if feet := p.Body.Position[1] - PlayerRadius; feet < 1.9 {
		t.Errorf("after climbing, feet at %.2f m: want on the 2 m platform", feet)
	}
}

// A jump and a climb reach about 3.6 m; a 4 m wall is out of reach.
func TestTooHighToClimb(t *testing.T) {
	a, p := obstacle(8, 1, 4)
	run(a, 0.5, forward)
	for range 90 {
		run(a, frame, Input{Move: [2]float32{0, 1}, Jump: true})
	}
	if p.Mantle.On || p.Body.Position[1] > 2 || p.Body.Position[2] < 8 {
		t.Errorf("a 4 m wall was got over (at %v)", p.Body.Position)
	}
}

func TestNoStandingUnderACeiling(t *testing.T) {
	// A slab overhead at 1.5 m: room crouched, not standing.
	b := newBuilder("ceiling", mathx.Vec3{}, 0)
	b.box(mathx.Vec3{-3, 1.5, 0}, mathx.Vec3{3, 1.7, 6}, Concrete)
	a := flatArena([]*Structure{b.finish()}, at(0, 10, 0))
	p := a.Players[0]
	run(a, 0.3)
	run(a, 2, Input{Move: [2]float32{0, 1}, Crouch: true}) // crawl under it
	if z := p.Body.Position[2]; z > 5.5 {
		t.Fatalf("crouch-walking under a 1.5 m ceiling got to z %.2f, want under it (z < 5.5)", z)
	}
	run(a, 0.5)
	if !p.crouched || p.Crouch < 0.9 {
		t.Errorf("let go of crouch under a 1.5 m ceiling: crouched %v (%.2f); want kept down", p.crouched, p.Crouch)
	}
}

// A thin wall's top is a ledge too: jump at a 2 m wall 0.1 m thick and you
// go up and over.
func TestClimbAThinWall(t *testing.T) {
	a, p := obstacle(8, 0.1, 2.0)
	run(a, 0.5, forward)
	for range 60 {
		run(a, frame, Input{Move: [2]float32{0, 1}, Jump: true})
	}
	run(a, 1, forward)
	if z := p.Body.Position[2]; z > 7.9-PlayerRadius {
		t.Errorf("jumping at a thin 2 m wall, at z %.2f: want up and over it (z < %.2f)", z, 7.9-PlayerRadius)
	}
}
