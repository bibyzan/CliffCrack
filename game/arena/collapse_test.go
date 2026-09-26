package arena

import (
	"testing"

	"CliffCrack/engine/mathx"
)

// platform is a concrete deck on four posts at its corners, height tall,
// with players nowhere near.
func platform(height float32, decks int) (*Arena, *Structure) {
	b := newBuilder("platform", mathx.Vec3{0, 0, -10}, 0)
	storey := height / float32(decks)
	for i := range decks {
		base := storey * float32(i)
		for _, x := range []float32{-1.5, 1.5} {
			for _, z := range []float32{-1.5, 1.5} {
				b.column(x, z, base, storey-0.3, 0.6, Concrete) // the deck sits on top
			}
		}
		b.slab(-2.25, -2.25, 2.25, 2.25, base+storey, 0.3, Concrete)
	}
	s := b.finish()
	a := flatArena([]*Structure{s}, at(20, 20, 0))
	run(a, 0.2)
	return a, s
}

// bottom is the lowest chunk of the post at x, z.
func bottom(s *Structure, x, z float32) *Chunk {
	var best *Chunk
	for _, c := range s.Chunks {
		if abs(c.Centre[0]-x) < 0.1 && abs(c.Centre[2]-(z-10)) < 0.1 && (best == nil || c.Centre[1] < best.Centre[1]) {
			best = c
		}
	}
	return best
}

// Built, nothing's overstressed; knock out two of the four posts and the
// other two carry double: they creak and, a few seconds on, give way.
func TestOverstressedSupportsGiveWay(t *testing.T) {
	a, s := platform(3, 1)
	for _, c := range s.Chunks {
		if c.Strain() >= 1 {
			t.Fatalf("as built, a chunk is overstressed (%.2f)", c.Strain())
		}
	}
	var ev Events
	a.breakChunk(bottom(s, -1.5, -1.5), mathx.Vec3{}, nil, &ev)
	a.breakChunk(bottom(s, -1.5, 1.5), mathx.Vec3{}, nil, &ev)
	strained := len(run(a, 1).Strained)
	standing := s.Alive()
	if strained == 0 {
		t.Fatal("with half its posts gone, nothing creaks")
	}
	if standing < len(s.Chunks)/2 {
		t.Fatalf("it came down at once (%d of %d left): it should stand a while first", standing, len(s.Chunks))
	}
	// It holds a few seconds, creaking, then gives way piece by piece.
	run(a, 2)
	if s.Alive() < standing {
		t.Fatalf("it gave way within 3 s (%d -> %d): the overload should take a while", standing, s.Alive())
	}
	run(a, 15)
	if s.Alive() > standing-4 {
		t.Errorf("after 18 s, %d of %d still stand: the overloaded posts should have given way", s.Alive(), standing)
	}
}

// Losing one post of four, the other three carry a third more: that holds.
func TestOneLostPostHolds(t *testing.T) {
	a, s := platform(3, 1)
	var ev Events
	a.breakChunk(bottom(s, -1.5, -1.5), mathx.Vec3{}, nil, &ev)
	run(a, 15)
	if s.Remaining() < 0.7 {
		t.Errorf("one post lost of four and %.0f%% is left: the rest should hold it", s.Remaining()*100)
	}
}

// A tall tower with its base shot out falls over as one, away from where it
// was hit, and shatters where it lands.
func TestTowerToppples(t *testing.T) {
	a, s := platform(12, 4)
	top := s.Chunks[0]
	for _, c := range s.Chunks {
		if c.Centre[1] > top.Centre[1] {
			top = c
		}
	}
	var ev Events
	for _, x := range []float32{-1.5, 1.5} {
		for _, z := range []float32{-1.5, 1.5} {
			a.breakChunk(bottom(s, x, z), mathx.Vec3{}, nil, &ev)
		}
	}
	a.lastBreakAt = mathx.Vec3{8, 0, -10} // hit from the east: it falls east
	a.settle(&ev)
	if len(ev.Topples) != 1 {
		t.Fatalf("%d pieces started to topple, want the tower as one", len(ev.Topples))
	}
	tp := ev.Topples[0]
	furthest := float32(0)
	var shattered bool
	for range 60 * 8 {
		f := tp.Frame()
		furthest = max(furthest, f.TransformPoint(top.Centre)[0])
		if run(a, frame); tp.Down {
			shattered = true
			break
		}
	}
	if furthest < 5 {
		t.Errorf("the tower's top got %.1f m east: it should have fallen over, not straight down", furthest)
	}
	if !shattered || len(a.Topples) != 0 {
		t.Fatal("it never landed and shattered")
	}
	if len(a.Debris) < len(tp.Chunks) {
		t.Errorf("%d pieces of rubble from %d chunks", len(a.Debris), len(tp.Chunks))
	}
}

// A toppling tower sweeps a player in its way aside, without hurting them.
func TestToppleSweepsPlayers(t *testing.T) {
	a, s := platform(12, 4)
	p := a.Players[0]
	p.Body.Position = mathx.Vec3{5, PlayerRadius + 0.02, -10} // east of it, where it falls
	var ev Events
	for _, x := range []float32{-1.5, 1.5} {
		for _, z := range []float32{-1.5, 1.5} {
			a.breakChunk(bottom(s, x, z), mathx.Vec3{}, nil, &ev)
		}
	}
	a.lastBreakAt = mathx.Vec3{8, 0, -10}
	a.settle(&ev)
	start := p.Body.Position
	all := run(a, 6)
	if p.Body.Position.Sub(start).Len() < 1 {
		t.Error("the tower fell on the player and didn't move them")
	}
	if len(all.Hurts) != 0 || p.Durability() < MaxShield+MaxHealth {
		t.Error("the tower hurt the player: the environment only pushes")
	}
}

// On a real site: shoot out the foot of the spire's four posts and the
// spire falls over, most of it as one.
func TestSpireFallsOver(t *testing.T) {
	a := New(11, 2, 0)
	// The spire is built in two halves (each end's half of the keep), two
	// posts each: take out all four.
	var spire []*Chunk
	for _, s := range a.Structures {
		if s.Name == "spire" {
			spire = append(spire, s.Chunks...)
		}
	}
	if len(spire) == 0 {
		t.Fatal("no spire on the site")
	}
	lo := float32(1e9)
	for _, c := range spire {
		lo = min(lo, c.Centre[1])
	}
	var ev Events
	for _, c := range spire {
		if c.Alive && c.Centre[1] < lo+0.1 {
			a.breakChunk(c, mathx.Vec3{}, nil, &ev)
		}
	}
	a.settle(&ev)
	toppled := 0
	for _, tp := range ev.Topples {
		toppled += len(tp.Chunks)
	}
	if toppled < len(spire)/2 {
		t.Errorf("%d of the spire's %d chunks toppled: most of it should fall over as one", toppled, len(spire))
	}
	var spireFall *Topple
	for _, tp := range ev.Topples {
		if spireFall == nil || len(tp.Chunks) > len(spireFall.Chunks) {
			spireFall = tp
		}
	}
	top := spireFall.Chunks[0]
	for _, c := range spireFall.Chunks {
		if c.Centre[1] > top.Centre[1] {
			top = c
		}
	}
	start := top.Centre
	for range 60 * 8 {
		if run(a, frame); spireFall.Down {
			break
		}
	}
	if !spireFall.Down {
		t.Fatal("8 s on, the spire still hasn't landed")
	}
	if spireFall.Angle < 0.6 {
		t.Errorf("the spire shattered at %.0f degrees over: it should fall well over first", spireFall.Angle*180/3.14159)
	}
	end := spireFall.Frame().TransformPoint(top.Centre)
	if dx := end.Sub(start); dx[0]*dx[0]+dx[2]*dx[2] < 25 {
		t.Errorf("the spire's top came down %.1f m from where it stood: it should have fallen over", dx.Len())
	}
}
