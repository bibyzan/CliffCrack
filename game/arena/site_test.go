package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func TestGenerateSite(t *testing.T) {
	a, b := GenerateSite(42), GenerateSite(42)
	if len(a.Structures) != len(b.Structures) || len(a.Structures) == 0 {
		t.Fatalf("the same seed should give the same site (%d vs %d structures)", len(a.Structures), len(b.Structures))
	}
	total := 0
	for i, s := range a.Structures {
		if len(s.Chunks) != len(b.Structures[i].Chunks) {
			t.Fatal("the same seed should give the same structures")
		}
		total += len(s.Chunks)
		allStanding(t, s)
		for _, c := range s.Chunks {
			if abs(c.Centre[0])+c.Half[0] > arenaHalfX || abs(c.Centre[2])+c.Half[2] > arenaHalfZ {
				t.Fatalf("%s chunk outside the arena at %v", s.Name, c.Centre)
			}
		}
	}
	if total < 60 || total > 3000 {
		t.Errorf("site has %d chunks, want enough cover to fight around", total)
	}
	if c := GenerateSite(43); len(c.Structures) == len(a.Structures) && len(c.Structures[0].Chunks) == len(a.Structures[0].Chunks) {
		t.Log("seeds 42 and 43 happen to start alike") // not an error, just unlikely
	}
}

func TestSiteIsMirrored(t *testing.T) {
	s := GenerateSite(7)
	if len(s.Structures)%2 != 0 {
		t.Fatalf("%d structures: want them in mirrored pairs", len(s.Structures))
	}
	for i := 0; i < len(s.Structures); i += 2 {
		south, north := s.Structures[i], s.Structures[i+1]
		if len(south.Chunks) != len(north.Chunks) {
			t.Fatalf("pair %d: %d vs %d chunks", i/2, len(south.Chunks), len(north.Chunks))
		}
		for j, c := range south.Chunks {
			m := north.Chunks[j]
			want := mathx.Vec3{-c.Centre[0], c.Centre[1], -c.Centre[2]}
			if m.Centre.Sub(want).Len() > 1e-3 || m.Half.Sub(c.Half).Len() > 1e-3 || m.Mat != c.Mat {
				t.Fatalf("pair %d chunk %d: south %v, north %v; want the north one at %v", i/2, j, c.Centre, m.Centre, want)
			}
		}
		if lo, _ := footprint(south); lo[2] < 1 {
			t.Errorf("south structure %s reaches the middle (z from %v)", south.Name, lo[2])
		}
	}
	// Spawns alternate ends, facing the middle.
	for i, sp := range s.Spawns {
		south := i%2 == 0
		if (sp.At[2] > 0) != south || sp.At[2]*float32(math.Cos(float64(sp.Yaw))) <= 0 {
			t.Errorf("spawn %d at %v yaw %v: want alternating ends, facing the middle", i, sp.At, sp.Yaw)
		}
	}
}

func TestLaunchBaysFireWhenTheRoundStarts(t *testing.T) {
	a := New(3, 2, 0)
	p := a.Players[0]
	run(a, 1) // on the pad, not live: stays put
	start := p.Body.Position
	if start[2] < arenaHalfZ || start[1] < bayFloor {
		t.Fatalf("player 0 should start in the south bay, at %v", start)
	}
	if moved := p.Body.Position.Sub(start).Len(); moved > 0.05 {
		t.Fatalf("a spawn pad fired before the round was live")
	}
	a.Live = true
	ev := a.Step(frame, nil)
	if !ev.Did(p, ActBoost) {
		t.Fatal("the pad should fire once the round is live")
	}
	// Over the end wall and down in the landing zone, still on your feet.
	peak := float32(0)
	for range 180 {
		a.Step(frame, nil)
		peak = max(peak, p.Body.Position[1])
	}
	pos := p.Body.Position
	if peak < endWallLow+1 {
		t.Errorf("launch peaked at %.1f m, want it to clear the %.1f m end wall", peak, endWallLow)
	}
	if pos[2] < 12 || pos[2] > 24 || math.Abs(float64(pos[1]-PlayerRadius)) > 0.05 {
		t.Errorf("landed at %v, want on the floor in the landing zone (z 12..24)", pos)
	}
	q := a.Players[1].Body.Position
	if q[2] > -12 || q[2] < -24 {
		t.Errorf("player 1 landed at %v, want the north landing zone", q)
	}
	// And it doesn't fire again: nobody's on it.
	if ev := a.Step(frame, nil); ev.Did(p, ActBoost) {
		t.Error("launched again")
	}
}

func TestJumpPadReachesThePlatform(t *testing.T) {
	a := New(3, 0, 0)
	var pad Pad
	for _, p := range a.Pads {
		if !p.Spawn && p.Centre[2] > 0 {
			pad = p
		}
	}
	// Walk onto it from the south.
	p := a.AddPlayer(Spawn{At: pad.Centre.Add(mathx.Vec3{0, PlayerRadius + 0.02, 3}), Yaw: 0})
	var boosted bool
	for range 120 {
		ev := a.Step(frame, []Input{{Move: [2]float32{0, 1}}})
		boosted = boosted || ev.Did(p, ActBoost)
	}
	run(a, 1)
	pos := p.Body.Position
	if !boosted {
		t.Fatal("walking over the jump pad should launch you")
	}
	if math.Abs(float64(pos[1]-(PlatformH+PlayerRadius))) > 0.1 {
		t.Errorf("ended at %v: want on top of the centre platform (y %.1f)", pos, PlatformH+PlayerRadius)
	}
}

func TestMatchArmsTheBaysAtTheFight(t *testing.T) {
	m := NewMatch(3, 2)
	if m.Arena.Live {
		t.Fatal("live during the countdown")
	}
	toFight(t, m)
	if !m.Arena.Live {
		t.Error("the round should be live once the fight starts")
	}
}
