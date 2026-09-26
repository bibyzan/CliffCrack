package arena

import (
	"math"
	"testing"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
)

// chunksWhere returns the standing chunks of structures named name ("" for
// any) matching f.
func chunksWhere(a *Arena, name string, f func(*Chunk) bool) []*Chunk {
	var out []*Chunk
	for _, s := range a.Structures {
		if name != "" && s.Name != name {
			continue
		}
		for _, c := range s.Chunks {
			if c.Alive && f(c) {
				out = append(out, c)
			}
		}
	}
	return out
}

func collapsed(ev Events, m Material) int {
	n := 0
	for _, b := range ev.Breaks {
		if b.Collapsed && (m < 0 || b.Mat == m) {
			n++
		}
	}
	return n
}

func TestBlowingOutTheFloorDropsWhatStandsOnIt(t *testing.T) {
	a := New(3, 0, 0)
	var cover *Structure
	for _, s := range a.Structures {
		if !s.Shell {
			cover = s
			break
		}
	}
	lo, hi := footprint(cover)
	var ev Events
	for _, c := range chunksWhere(a, "floor", func(c *Chunk) bool {
		return c.Centre[0]+c.Half[0] > lo[0] && c.Centre[0]-c.Half[0] < hi[0] &&
			c.Centre[2]+c.Half[2] > lo[2] && c.Centre[2]-c.Half[2] < hi[2]
	}) {
		a.breakChunk(c, mathx.Vec3{}, nil, &ev)
	}
	a.settle(&ev)
	if cover.Alive() != 0 {
		t.Errorf("%d of the %s's %d chunks still standing on a floor that's gone", cover.Alive(), cover.Name, len(cover.Chunks))
	}
	if collapsed(ev, Plate) != 0 {
		t.Errorf("%d floor plates collapsed round the hole; the girders should hold the rest", collapsed(ev, Plate))
	}
}

func TestTheKeepComesDownWithoutItsColumns(t *testing.T) {
	a := New(3, 0, 0)
	tier := func(h float32) func(*Chunk) bool {
		return func(c *Chunk) bool { return abs(c.Top()-h) < 1e-3 && c.Half[1] < 0.2 && abs(c.Centre[0]) < keepDeckX }
	}
	deck, top := len(chunksWhere(a, "keep", tier(PlatformH))), len(chunksWhere(a, "keep", tier(TopH)))
	var ev Events
	for _, c := range chunksWhere(a, "keep", func(c *Chunk) bool {
		return c.Half[0] < 0.35 && c.Half[2] < 0.35 && c.Top() < PlatformH-0.25
	}) {
		a.breakChunk(c, mathx.Vec3{}, nil, &ev)
	}
	a.settle(&ev)
	left := chunksWhere(a, "keep", tier(PlatformH))
	if len(left) == 0 || len(left) == deck {
		t.Fatalf("%d of %d deck tiles left without the columns: the middle should fall, the edges hang off the stairs and bridges", len(left), deck)
	}
	for _, c := range left {
		if abs(c.Centre[0]) < 2.5 && abs(c.Centre[2]) < 2.5 {
			t.Errorf("a deck tile in the middle is still up at %v", c.Centre)
		}
	}
	if n := len(chunksWhere(a, "keep", tier(TopH))); n >= top {
		t.Errorf("the top tier (%d tiles) should come down with the deck under it; %d left", top, n)
	}
}

func TestKnockedIntoThePitCreditsTheAttacker(t *testing.T) {
	a := New(3, 2, 0)
	p0, victim := a.Players[0], a.Players[1]
	// Stand the victim between the girders, and blow the floor out from
	// under them.
	spot := mathx.Vec3{5, PlayerRadius + 0.02, 21}
	victim.Body.Position = spot
	victim.Body.Teleported()
	run(a, 0.3)
	var ev Events
	a.blast(mathx.Vec3{5, 0.1, 21}, 2.5, 1000, BlastPlayerDamage, 2, mathx.Vec3{}, p0, WeaponLauncher, &ev)
	if victim.Dead {
		t.Fatal("the blast alone shouldn't kill")
	}
	ev.Merge(run(a, 3))
	if !victim.Dead || len(ev.Kills) != 1 {
		t.Fatalf("victim dead %v at %v, kills %+v: want them down the hole and out", victim.Dead, victim.Body.Position, ev.Kills)
	}
	if k := ev.Kills[0]; k.By != p0 || k.Weapon != WeaponDrop {
		t.Errorf("kill %+v: want it credited to the attacker, as the drop", k)
	}
	if p0.Kills != 1 {
		t.Errorf("attacker has %d kills", p0.Kills)
	}
}

// climb walks a player from start facing yaw for seconds and returns where
// they end up.
func climb(a *Arena, start mathx.Vec3, yaw, seconds float32) mathx.Vec3 {
	p := a.AddPlayer(Spawn{At: start, Yaw: yaw})
	run(a, seconds, Input{Move: [2]float32{0, 1}})
	run(a, 0.5) // stop, and settle from the hop off the top step
	return p.Body.Position
}

func TestStairsClimbToTheKeep(t *testing.T) {
	// From the foot of the grand stairs at the south end, facing north.
	pos := climb(New(3, 0, 0), mathx.Vec3{6.7, PlayerRadius + 0.02, 14.1}, 0, 2.1)
	if abs(pos[1]-(PlatformH+PlayerRadius)) > 0.1 || pos[2] > keepDeckZ {
		t.Errorf("ended at %v: want up the stairs, on the keep's deck (y %.2f)", pos, PlatformH+PlayerRadius)
	}
	// From the bridge up the steep stairs to the top tier, facing west.
	yaw := float32(math.Pi / 2)
	if camera.Direction(yaw, 0)[0] > 0 {
		yaw = -yaw
	}
	pos = climb(New(3, 0, 0), mathx.Vec3{12, PlatformH + PlayerRadius + 0.02, 0.6}, yaw, 1.5)
	if abs(pos[1]-(TopH+PlayerRadius)) > 0.1 || abs(pos[0]) > keepTopX {
		t.Errorf("ended at %v: want up the steep stairs, on the top tier (y %.2f)", pos, TopH+PlayerRadius)
	}
}

// Falling rubble shoves you but never hurts, and the shove is credited to
// whoever brought it down (a fall into the pit is theirs).
func TestFallingRubbleShovesButDoesntHurt(t *testing.T) {
	a := flatArena(nil, at(0, 0, 0), at(10, 10, 0))
	p, by := a.Players[0], a.Players[1]
	run(a, 0.2)
	d := a.addDebris(p.Body.Position.Add(mathx.Vec3{-0.6, 3, 0}), mathx.Vec3{0.5, 0.5, 0.5}, Concrete, mathx.Vec3{6, -10, 0})
	d.By = by
	start := p.Body.Position
	ev := run(a, 0.5)
	if p.Durability() < MaxShield+MaxHealth || len(ev.Hurts) != 0 {
		t.Fatalf("falling concrete hurt: %v left", p.Durability())
	}
	if moved := p.Body.Position.Sub(start); moved.Len() < 0.3 {
		t.Errorf("falling concrete didn't shove the player (moved %v)", moved)
	}
	if p.lastHitBy != by {
		t.Error("the shove should be credited to whoever brought the rubble down")
	}
}

func TestPadGoesWithItsFloor(t *testing.T) {
	a := New(3, 0, 0)
	var pad Pad
	for _, p := range a.Pads {
		if !p.Spawn && p.Centre[2] > 0 {
			pad = p
		}
	}
	if !a.PadWorks(pad) {
		t.Fatal("the jump pad should work on an intact floor")
	}
	var ev Events
	for _, c := range chunksWhere(a, "floor", func(c *Chunk) bool { return c.distTo(pad.Centre) < 0.5 }) {
		a.breakChunk(c, mathx.Vec3{}, nil, &ev)
	}
	if a.PadWorks(pad) {
		t.Error("the pad still works with its floor blown out")
	}
}

func TestALaunchCantBeBrakedIntoThePit(t *testing.T) {
	a := New(3, 2, 0)
	a.Live = true
	back := Input{Move: [2]float32{0.5, -1}}
	run(a, 3, back, back)
	for i, p := range a.Players {
		pos := p.Body.Position
		if p.Dead || abs(pos[1]-PlayerRadius) > 0.1 || abs(pos[2]) > arenaHalfZ {
			t.Errorf("player %d, holding back all the way, is at %v (dead %v): want down in the arena", i, pos, p.Dead)
		}
	}
}

// facing is the yaw that looks along dir (a unit vector on X or Z).
func facing(dir mathx.Vec3) float32 {
	for _, yaw := range []float32{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
		if camera.Direction(yaw, 0).Dot(dir) > 0.9 {
			return yaw
		}
	}
	panic("no yaw")
}

// hop stands a player on the pad at centre and returns where it throws them.
func hop(a *Arena, centre mathx.Vec3) mathx.Vec3 {
	p := a.AddPlayer(Spawn{At: centre.Add(mathx.Vec3{0, PlayerRadius + 0.02, 0})})
	run(a, 2.5)
	return p.Body.Position
}

func TestPadsClimbTheSpire(t *testing.T) {
	// Every spire pad throws you up a level onto the next balcony (or the
	// cap), and not onto another pad.
	a := New(3, 0, 0)
	n := 0
	for _, pad := range a.Pads {
		if pad.Spawn || pad.Centre[1] < TopH || abs(pad.Centre[0]) > 7 {
			continue
		}
		n++
		b := New(3, 0, 0)
		p := b.AddPlayer(Spawn{At: pad.Centre.Add(mathx.Vec3{0, PlayerRadius + 0.02, 0})})
		boosts := 0
		for range 150 {
			if ev := b.Step(frame, nil); ev.Did(p, ActBoost) {
				boosts++
			}
		}
		pos := p.Body.Position
		if abs(pos[1]-(pad.Centre[1]+spireStep+PlayerRadius)) > 0.1 {
			t.Errorf("pad at %v: ended at %v, want a level up (y %.1f)", pad.Centre, pos, pad.Centre[1]+spireStep+PlayerRadius)
		}
		if boosts != 1 {
			t.Errorf("pad at %v: thrown %d times, want once (not onto another pad)", pad.Centre, boosts)
		}
	}
	if want := 2 * 2 * (1 + int((SpireH-firstBalcony)/spireStep)); n != want {
		t.Errorf("%d spire pads, want %d", n, want)
	}
}

func TestHopPadReachesThePinnacle(t *testing.T) {
	pos := hop(New(3, 0, 0), mathx.Vec3{-26.8, SummitH, 0})
	if abs(pos[1]-(PinnacleH+PlayerRadius)) > 0.1 {
		t.Errorf("summit pad: ended at %v, want on the pinnacle (y %.1f)", pos, PinnacleH+PlayerRadius)
	}
}

func TestClimbTheMountain(t *testing.T) {
	east, south, north := mathx.Vec3{1, 0, 0}, mathx.Vec3{0, 0, 1}, mathx.Vec3{0, 0, -1}
	r := float32(PlayerRadius + 0.02)
	for _, leg := range []struct {
		name    string
		from    mathx.Vec3
		dir     mathx.Vec3
		seconds float32
		height  float32
		arrived func(mathx.Vec3) bool
	}{
		{"off the side ledge onto the saddle", mathx.Vec3{17, PlatformH + r, 7}, east, 1.2, PlatformH, func(p mathx.Vec3) bool { return p[0] > 20.5 }},
		{"up the first ramp to the lower terrace", mathx.Vec3{22.75, PlatformH + r, 8}, south, 1.8, TopH, func(p mathx.Vec3) bool { return p[2] > 15.5 }},
		{"up the second ramp to the upper terrace", mathx.Vec3{27.75, TopH + r, 18}, north, 2, CrownH, func(p mathx.Vec3) bool { return p[2] < 10.5 }},
		{"up the last ramp to the summit", mathx.Vec3{28, CrownH + r, 8.5}, north, 1.3, SummitH, func(p mathx.Vec3) bool { return p[2] < 1.5 && p[2] > -1.5 }},
		{"from a tower over the sky bridge to the mountain", mathx.Vec3{9, TopH + r, towerZ}, east, 2.6, TopH, func(p mathx.Vec3) bool { return p[0] > 21 }},
	} {
		pos := climb(New(3, 0, 0), leg.from, facing(leg.dir), leg.seconds)
		if abs(pos[1]-(leg.height+PlayerRadius)) > 0.1 || !leg.arrived(pos) {
			t.Errorf("%s: ended at %v, want at height %.1f", leg.name, pos, leg.height)
		}
	}
}

func TestJumpingUnderADeckKeepsYourHeadBelowIt(t *testing.T) {
	a := New(3, 0, 0)
	p := a.AddPlayer(Spawn{At: mathx.Vec3{2.2, PlayerRadius + 0.02, 3}}) // under the keep's deck
	run(a, 0.3)
	underside := float32(PlatformH - 0.3)
	peak := float32(0)
	for i := range 60 {
		a.Step(frame, []Input{{Jump: i == 0}})
		peak = max(peak, p.Eye(1)[1])
	}
	if peak > underside-0.05 {
		t.Errorf("eye rose to %.2f jumping under a deck whose underside is at %.2f: the camera goes through it", peak, underside)
	}
	if peak < 1.9 {
		t.Errorf("eye only rose to %.2f: the jump should still happen, just stop short of the deck", peak)
	}
}
