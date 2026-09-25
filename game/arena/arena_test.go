package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

const frame = 1.0 / 60

// run steps the arena for seconds with the same input every frame and merges
// the events.
func run(a *Arena, seconds float32, in Input) Events {
	var all Events
	for t := float32(0); t < seconds; t += frame {
		ev := a.Step(frame, in)
		all.Shots = append(all.Shots, ev.Shots...)
		all.Kills = append(all.Kills, ev.Kills...)
		all.Jumped = all.Jumped || ev.Jumped
		all.Landed = max(all.Landed, ev.Landed)
		all.Reloaded = all.Reloaded || ev.Reloaded
		all.Empty = all.Empty || ev.Empty
	}
	return all
}

// quiet parks every drone far overhead so tests control what's in the way.
func quiet(a *Arena) {
	for _, d := range a.Drones {
		d.Patrol = Patrol{Centre: mathx.Vec3{0, 60, 0}}
		d.place(a.Time)
	}
}

func TestWalkForwardOnTheFloor(t *testing.T) {
	a := New(1)
	quiet(a)
	run(a, 0.3, Input{}) // settle
	start := a.Player.Body.Position
	run(a, 1, Input{Move: [2]float32{0, 1}})
	p := a.Player.Body.Position
	if moved := start[2] - p[2]; moved < 4.5 || moved > 7 {
		t.Errorf("walked %.2f m north in 1 s, want ~6", moved)
	}
	if math.Abs(float64(p[0]-start[0])) > 0.05 {
		t.Errorf("drifted sideways to x = %v", p[0])
	}
	if math.Abs(float64(p[1]-PlayerRadius)) > 0.05 || !a.Player.OnGround() {
		t.Errorf("should stay on the floor: y = %v, grounded %v", p[1], a.Player.OnGround())
	}

	// Letting go stops quickly.
	run(a, 0.5, Input{})
	if v := a.Player.Body.Velocity.Len(); v > 0.2 {
		t.Errorf("still moving at %v m/s 0.5 s after letting go", v)
	}
}

func TestWallsStopThePlayer(t *testing.T) {
	a := New(1)
	quiet(a)
	a.Player.Yaw = math.Pi // face south (+Z), towards the nearest wall
	run(a, 4, Input{Move: [2]float32{0, 1}, Sprint: true})
	if z := a.Player.Body.Position[2]; z > HalfSize-PlayerRadius+0.02 {
		t.Errorf("walked through the south wall: z = %v", z)
	}
}

func TestRampLeadsOntoThePlatform(t *testing.T) {
	a := New(1)
	quiet(a)
	run(a, 4, Input{Move: [2]float32{0, 1}}) // north from the spawn: up the south ramp
	p := a.Player.Body.Position
	if math.Abs(float64(p[2])) > 4 {
		t.Fatalf("should be on the platform after 4 s, at z = %v", p[2])
	}
	if want := float32(PlatformH + PlayerRadius); math.Abs(float64(p[1]-want)) > 0.1 {
		t.Errorf("standing at y = %v, want the platform top %v", p[1], want)
	}
	// And stands still there (fixed rotation + friction).
	run(a, 1, Input{})
	if a.Player.Body.Velocity.Len() > 0.1 {
		t.Errorf("sliding on the platform: v = %v", a.Player.Body.Velocity)
	}
}

func TestJumpAndLand(t *testing.T) {
	a := New(1)
	quiet(a)
	run(a, 0.3, Input{})
	ev := a.Step(frame, Input{Jump: true})
	if !ev.Jumped {
		t.Fatal("jump from the ground should work")
	}
	peak := float32(0)
	var landed float32
	for range 90 {
		e := a.Step(frame, Input{Jump: true}) // holding jump mustn't double-jump in the air
		peak = max(peak, a.Player.Body.Position[1])
		landed = max(landed, e.Landed)
		if e.Jumped && landed == 0 {
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

// placeDroneAhead parks drone 0 straight ahead of the player's eye.
func placeDroneAhead(a *Arena, dist float32) *Drone {
	d := a.Drones[0]
	eye := a.Player.Eye(1)
	d.Patrol = Patrol{Centre: eye.Add(a.Player.Forward().Scale(dist))}
	d.place(a.Time)
	return d
}

func TestShootingKillsAndRespawns(t *testing.T) {
	a := New(1)
	quiet(a)
	run(a, 0.3, Input{})
	d := placeDroneAhead(a, 8)

	ev := run(a, 0.5, Input{Fire: true, FirePressed: true})
	if len(ev.Kills) != 1 || ev.Kills[0] != d || !d.Dead {
		t.Fatalf("three hits should kill the drone: kills %d, dead %v, health %d", len(ev.Kills), d.Dead, d.Health)
	}
	if a.Score != KillScore || a.Kills != 1 || a.ShotsHit < DroneHealth {
		t.Errorf("score %d kills %d hits %d", a.Score, a.Kills, a.ShotsHit)
	}
	if len(a.Debris) != debrisPerKill {
		t.Errorf("%d debris pieces, want %d", len(a.Debris), debrisPerKill)
	}
	if a.Alive() != len(a.Drones)-1 {
		t.Errorf("%d alive, want one down", a.Alive())
	}

	// Debris clears and the drone comes back.
	run(a, debrisLife+0.1, Input{})
	if len(a.Debris) != 0 {
		t.Errorf("%d debris left after %v s", len(a.Debris), debrisLife)
	}
	if d.Dead || d.Health != DroneHealth {
		t.Errorf("drone should respawn at full health: dead %v health %d", d.Dead, d.Health)
	}
}

func TestCoverBlocksShots(t *testing.T) {
	a := New(1)
	quiet(a)
	// Stand south of the north-east pillar (10, _, -12) and look at it; put a drone behind it.
	a.Player.Body.Position = mathx.Vec3{10, PlayerRadius + 0.02, -6}
	a.Player.Body.Teleported()
	a.Player.Yaw, a.Player.Pitch = 0, 0
	run(a, 0.3, Input{})
	d := a.Drones[0]
	d.Patrol = Patrol{Centre: mathx.Vec3{10, a.Player.Eye(1)[1], -18}}
	d.place(a.Time)

	ev := run(a, 0.5, Input{Fire: true, FirePressed: true})
	if d.Health != DroneHealth {
		t.Errorf("drone behind the pillar took damage: health %d", d.Health)
	}
	if len(ev.Shots) == 0 {
		t.Fatal("no shots fired")
	}
	for _, s := range ev.Shots {
		if s.To[2] < -11.1 || s.Drone != nil {
			t.Errorf("bullet passed the pillar's face (z = -11): stopped at %v", s.To)
		}
	}
}

func TestMagazineAndReload(t *testing.T) {
	a := New(1)
	quiet(a)
	a.Player.Pitch = 0.5 // shoot at the sky: no targets
	ev := run(a, float32(MagSize)*fireInterval+0.05, Input{Fire: true})
	if len(ev.Shots) != MagSize {
		t.Errorf("fired %d shots from a full magazine, want %d", len(ev.Shots), MagSize)
	}
	if !ev.Reloaded || a.Weapon.Reloading == 0 {
		t.Fatal("an empty magazine should start a reload")
	}
	if ev := run(a, 0.5, Input{Fire: true}); len(ev.Shots) != 0 {
		t.Error("can't fire while reloading")
	}
	run(a, ReloadTime, Input{})
	if a.Weapon.Ammo != MagSize || a.Weapon.Reloading != 0 {
		t.Errorf("after reloading: ammo %d, reloading %v", a.Weapon.Ammo, a.Weapon.Reloading)
	}

	// Manual reload of a partial magazine.
	run(a, 0.35, Input{Fire: true})
	if ev := a.Step(frame, Input{Reload: true}); !ev.Reloaded {
		t.Error("R with a partial magazine should reload")
	}
}

func TestRecoilClimbsAndRecovers(t *testing.T) {
	a := New(1)
	quiet(a)
	a.Player.Pitch = 0.3
	run(a, 0.5, Input{Fire: true})
	if kick := a.Player.ViewPitch() - a.Player.Pitch; kick < 0.02 {
		t.Errorf("recoil after a burst = %v rad, want a noticeable climb", kick)
	}
	run(a, 1, Input{})
	if kick := a.Player.ViewPitch() - a.Player.Pitch; kick > 0.002 {
		t.Errorf("recoil should settle back, still %v", kick)
	}
}

func TestAutopilotScores(t *testing.T) {
	a := New(3)
	for range 60 * 20 { // 20 s
		a.Step(frame, a.Autopilot(frame))
	}
	if a.Kills < 8 {
		t.Errorf("autopilot got %d kills in 20 s, want at least 8", a.Kills)
	}
	if acc := a.Accuracy(); acc < 0.3 {
		t.Errorf("autopilot accuracy %.0f%%, want it to mostly hit", acc*100)
	}
}

func TestDronesFlyInsideTheArena(t *testing.T) {
	a := New(7)
	for step := 0; step < 600; step++ {
		a.Step(frame, Input{})
		for i, d := range a.Drones {
			p := d.Body.Position
			if math.Abs(float64(p[0])) > HalfSize-1 || math.Abs(float64(p[2])) > HalfSize-1 || p[1] < 2 {
				t.Fatalf("drone %d left the arena or dipped too low: %v", i, p)
			}
		}
	}
}
