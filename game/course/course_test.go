package course

import (
	"math"
	"sort"
	"testing"
)

func TestChunksShareTheirBoundaryRow(t *testing.T) {
	c := New(42)
	for index := -1; index < 6; index++ {
		a, b := c.Chunk(index), c.Chunk(index+1)
		last := a.Mesh.Vertices[len(a.Mesh.Vertices)-columns:]
		first := b.Mesh.Vertices[:columns]
		for i := range last {
			if last[i].Position != first[i].Position {
				t.Fatalf("chunk %d/%d seam differs at column %d: %v vs %v",
					index, index+1, i, last[i].Position, first[i].Position)
			}
		}
	}
}

func TestDeterministic(t *testing.T) {
	a := New(7)
	a.Chunk(0)
	a.Chunk(9) // generation order must not matter
	x := a.Chunk(4)
	y := New(7).Chunk(4)
	if len(x.Obstacles) != len(y.Obstacles) {
		t.Fatalf("obstacle counts differ: %d vs %d", len(x.Obstacles), len(y.Obstacles))
	}
	for i := range x.Obstacles {
		if x.Obstacles[i] != y.Obstacles[i] {
			t.Fatalf("obstacle %d differs", i)
		}
	}
	for i := range x.Mesh.Vertices {
		if x.Mesh.Vertices[i] != y.Mesh.Vertices[i] {
			t.Fatalf("vertex %d differs", i)
		}
	}
	if New(8).Height(3, -500) == a.Height(3, -500) {
		t.Error("different seeds should give different terrain")
	}
}

func TestRunDescendsInAChannel(t *testing.T) {
	c := New(1)
	prev := float32(math.Inf(1))
	for s := float32(0); s < 3000; s += 50 {
		if _, crack := c.CrackAt(s); crack {
			continue
		}
		// The path always goes downhill, sections included.
		h := c.Height(c.PathCentre(s), -s)
		if h > prev {
			t.Errorf("path climbs between s=%v and s=%v (%v -> %v)", s-50, s, prev, h)
		}
		prev = h
		if c.overlapsSection(s-30, s+30) {
			continue
		}
		// In plain valley, the banks rise on both sides of the channel.
		x := c.Centre(s)
		w := c.HalfWidth(s)
		for _, side := range []float32{-1, 1} {
			if bank := c.Height(x+side*(w+20), -s); bank < h+10 {
				t.Errorf("s=%v: bank at %v is only %v above the floor", s, side, bank-h)
			}
		}
	}
}

func TestCracks(t *testing.T) {
	c := New(3)
	cracks := c.Cracks(0, 4000)
	if len(cracks) < 3 { // plain valley only: sections bring their own hazards
		t.Fatalf("only %d cracks in the first 4 km", len(cracks))
	}
	if cracks[0].S < 200 {
		t.Errorf("first crack at %v, want a warm-up stretch", cracks[0].S)
	}
	for _, k := range cracks {
		s := k.S + k.Width/2
		x := c.Centre(s)
		if depth := c.Rim(x, -s) - c.Height(x, -s); depth < crackDepth-1 {
			t.Errorf("crack at %v is only %v deep in the channel", k.S, depth)
		}
		if lip := k.kicker(k.S - 1); lip != rampHeight {
			t.Errorf("crack at %v: kicker lip is %v high, want %v", k.S, lip, rampHeight)
		}
		if k.kicker(k.S-rampLength-1) != 0 || k.kicker(k.S) != 0 {
			t.Errorf("crack at %v: kicker extends past its run-in or into the opening", k.S)
		}
		if got, ok := c.CrackAt(s); !ok || got != k {
			t.Errorf("CrackAt(%v) = %v %v, want %v", s, got, ok, k)
		}
		if c.overlapsSection(k.S-rampLength, k.S+k.Width+landing) {
			t.Errorf("crack at %v runs into a special section", k.S)
		}
	}
}

func TestObstaclesStayClear(t *testing.T) {
	c := New(11)
	total := 0
	for index := -1; index < 60; index++ {
		for _, o := range c.Chunk(index).Obstacles {
			total++
			if o.Distance < StartClear {
				t.Errorf("obstacle at s=%v inside the clear start", o.Distance)
			}
			if c.inJumpZone(o.Distance) {
				t.Errorf("obstacle at s=%v on a crack, its kicker or its landing", o.Distance)
			}
			if got := c.Height(o.Base[0], o.Base[2]); math.Abs(float64(got-o.Base[1])) > 1e-3 {
				t.Errorf("obstacle base %v is not on the ground (%v)", o.Base, got)
			}
		}
	}
	if total < 200 {
		t.Errorf("only %d obstacles in ~2.9 km", total)
	}
}

func TestStartIsAtTheTopOfTheDrop(t *testing.T) {
	c := New(5)
	p := c.StartPosition()
	if drop := p[1] - c.Height(c.Centre(10), -10); drop < 20 {
		t.Errorf("the start is only %v m above the slope 10 m down the Drop", drop)
	}
	if drop := p[1] - c.Height(c.Centre(dropLength), -dropLength); drop < 600 {
		t.Errorf("the Drop only falls %v m: it should be huge", drop)
	}
	if up := c.Height(c.Centre(-10), 10) - p[1]; up < 20 {
		t.Errorf("the cliff behind the start is only %v m high", up)
	}
}

func TestItGetsSteeperAndHarder(t *testing.T) {
	c := New(21)
	if a, b := GradeAt(dropLength), GradeAt(3000); b < a*1.3 {
		t.Errorf("grade %v after the drop vs %v at 3 km: the slope should steepen", a, b)
	}
	if a, b := GradeAt(10), GradeAt(dropLength); a < b+0.5 {
		t.Errorf("grade %v on the drop vs %v after it: the drop should be much steeper", a, b)
	}
	// Average obstacles per plain-valley chunk, early vs late.
	count := func(from, to int) float64 {
		n, chunks := 0, 0
		for i := from; i < to; i++ {
			start := float32(i) * ChunkLength
			if c.overlapsSection(start-20, start+ChunkLength+20) {
				continue
			}
			n += len(c.Obstacles(i))
			chunks++
		}
		return float64(n) / float64(max(chunks, 1))
	}
	early, late := count(2, 12), count(50, 90)
	if late < early*1.3 {
		t.Errorf("%.1f obstacles per chunk early vs %.1f late: it should get busier", early, late)
	}
	if w0, w1 := c.HalfWidth(0), c.HalfWidth(4000); w1 >= w0 {
		t.Logf("width %v -> %v (noise can win locally)", w0, w1)
	}
	if Difficulty(5000) <= Difficulty(3000) {
		t.Error("difficulty should keep rising")
	}
}

func TestGatesLeaveAGap(t *testing.T) {
	c := New(8)
	gates := 0
	for index := 20; index < 80; index++ {
		// Group this chunk's rocks by distance: a gate is 6+ rocks at one s.
		rows := map[float32][]Obstacle{}
		for _, o := range c.Obstacles(index) {
			if o.Kind == Rock {
				rows[o.Distance] = append(rows[o.Distance], o)
			}
		}
		for s, rocks := range rows {
			if len(rocks) < 6 {
				continue
			}
			gates++
			// The widest gap between neighbouring rocks must let the ball through.
			xs := make([]float32, 0, len(rocks))
			for _, r := range rocks {
				xs = append(xs, r.Centre[0])
			}
			widest := float32(0)
			sortFloats(xs)
			for i := 1; i < len(xs); i++ {
				widest = max(widest, xs[i]-xs[i-1]-2*1.2*0.85)
			}
			if widest < 2.5 {
				t.Errorf("gate at s=%v has no gap wide enough (widest %v m)", s, widest)
			}
		}
	}
	if gates < 10 {
		t.Errorf("only %d gates in 60 chunks past 1 km", gates)
	}
}

func sortFloats(xs []float32) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// firstSection finds the first section of kind in seed's course.
func firstSection(t *testing.T, c *Course, kind SectionKind) Section {
	t.Helper()
	for s := float32(0); s < 20000; {
		k, ok := c.NextSection(s)
		if !ok {
			break
		}
		if k.Kind == kind {
			return k
		}
		s = k.Start
	}
	t.Fatalf("no section of kind %v", kind)
	return Section{}
}

func TestSectionsAreMixedIn(t *testing.T) {
	c := New(12)
	ridges, narrows := 0, 0
	prevEnd := float32(0)
	for s := float32(0); s < 10000; {
		k, ok := c.NextSection(s)
		if !ok || k.Start > 10000 {
			break
		}
		if k.Start < sectionsFrom {
			t.Errorf("section at %v inside the warm-up", k.Start)
		}
		if k.Start < prevEnd+100 {
			t.Errorf("section at %v starts right after the last one (%v)", k.Start, prevEnd)
		}
		switch k.Kind {
		case Ridge:
			ridges++
		case Narrows:
			narrows++
		}
		prevEnd, s = k.End(), k.Start
	}
	if ridges < 2 || narrows < 2 {
		t.Errorf("%d ridges and %d narrows in 10 km, want a mix", ridges, narrows)
	}
}

func TestRidgeIsACrestOverAPit(t *testing.T) {
	c := New(12)
	k := firstSection(t, c, Ridge)
	mid := k.Start + k.Length/2
	pc := c.PathCentre(mid)
	if shift := pc - c.Centre(mid); shift*k.Side < 40 {
		t.Errorf("mid-ridge the path is %v off the valley centre, want it up the mountain on side %v", shift, k.Side)
	}
	top := c.Height(pc, -mid)
	near := [2]float32{}
	for i, side := range []float32{-1, 1} {
		// Off either edge of the crest, the ground is far below: you can fall off.
		off := c.Height(pc+side*(ridgeHalfWidth+25), -mid)
		if top-off < 100 {
			t.Errorf("side %v: only %v m below the crest", side, top-off)
		}
		near[i] = top - c.Height(pc+side*(ridgeHalfWidth+8), -mid)
	}
	// Eight metres off the floor is mountainside, not more road. A ridge of
	// the peak field may stand as high as the crest; it may not do that on
	// both sides at once, which would be a plateau.
	if near[0] < 3 && near[1] < 3 && abs(near[0]-near[1]) < 4 {
		t.Errorf("shoulders are a shelf: %v and %v m below the crest", near[0], near[1])
	}
	// The racing line itself stays under the ball.
	if d := abs(c.Height(pc+2, -mid) - top); d > 1.2 {
		t.Errorf("2 m off the crest the ground has dropped %v m", d)
	}
	// The summit pitches, and the face beside it is the ridged mountains,
	// not a smooth ramp. Measure the face relative to the crest so the
	// overall downhill grade doesn't count.
	from, to := k.Start+ridgeClimb+70, k.End()-ridgeClimb-100
	minR, maxR := float32(1e9), float32(-1e9)
	minSky, maxSky := float32(1e9), float32(-1e9)
	for s := from; s < to; s += 3 {
		crest := c.Height(c.PathCentre(s), -s)
		sky := crest + drop(s)
		minSky, maxSky = min(minSky, sky), max(maxSky, sky)
		rel := c.Height(c.PathCentre(s)+k.Side*14, -s) - crest
		minR, maxR = min(minR, rel), max(maxR, rel)
	}
	if maxSky-minSky < 4 {
		t.Errorf("crest skyline only varies by %v m above the slope", maxSky-minSky)
	}
	if maxR-minR < 8 {
		t.Errorf("shoulder relief only varies by %v m, want the ridged mountains", maxR-minR)
	}
}

func TestRidgeClimbIsRideable(t *testing.T) {
	c := New(12)
	k := firstSection(t, c, Ridge)
	// Along the path through the whole section, the ground never falls away
	// under the path's centre and never climbs steeply: you can ride it.
	prev := c.Height(c.PathCentre(k.Start-10), -(k.Start - 10))
	for s := k.Start - 9; s < k.End()+10; s++ {
		h := c.Height(c.PathCentre(s), -s)
		if dh := h - prev; dh > 0.6 || dh < -2 {
			t.Fatalf("s=%v: path height jumps by %v m in a metre", s, dh)
		}
		// Across the path's floor, it's solid (no pit under it).
		hw := c.PathHalfWidth(s)
		for _, f := range []float32{-0.8, 0.8} {
			if side := c.Height(c.PathCentre(s)+f*hw, -s); h-side > 4 {
				t.Fatalf("s=%v: the path's floor drops %v m at %v of its width", s, h-side, f)
			}
		}
		prev = h
	}
}

func TestNarrowsHaveSheerWalls(t *testing.T) {
	c := New(12)
	k := firstSection(t, c, Narrows)
	mid := k.Start + k.Length/2
	pc := c.PathCentre(mid)
	hw := c.PathHalfWidth(mid)
	if hw < 4 || hw > 9 {
		t.Errorf("narrows half width %v, want a gorge a few metres wide", hw)
	}
	floor := c.Height(pc, -mid)
	for _, side := range []float32{-1, 1} {
		wall := c.Height(pc+side*(hw+12), -mid)
		if wall-floor < 12 {
			t.Errorf("side %v: wall only %v m high 12 m past the edge", side, wall-floor)
		}
	}
	// Not one straight slot: the floor weaves, the width breathes, and the
	// walls rise and fall with the mountain ridges.
	from, to := k.Start+narrowsIn+10, k.End()-narrowsIn-10
	minW, maxW := float32(1e9), float32(0)
	minS, maxS := float32(1e9), float32(-1e9)
	minH, maxH := float32(1e9), float32(-1e9)
	for s := from; s < to; s += 4 {
		w := c.PathHalfWidth(s)
		minW, maxW = min(minW, w), max(maxW, w)
		sh := c.PathCentre(s) - c.Centre(s)
		minS, maxS = min(minS, sh), max(maxS, sh)
		cx := c.PathCentre(s)
		rel := c.Height(cx+c.PathHalfWidth(s)+6, -s) - c.Height(cx, -s)
		minH, maxH = min(minH, rel), max(maxH, rel)
	}
	if maxW-minW < 2 {
		t.Errorf("gorge width only varies by %v m", maxW-minW)
	}
	if maxS-minS < 6 {
		t.Errorf("gorge only weaves %v m", maxS-minS)
	}
	if maxH-minH < 8 {
		t.Errorf("wall height only varies by %v m", maxH-minH)
	}
}

func TestChunksShareBoundaryRowsThroughSections(t *testing.T) {
	c := New(12)
	for _, kind := range []SectionKind{Ridge, Narrows} {
		k := firstSection(t, c, kind)
		for index := ChunkAt(k.Start) - 1; index <= ChunkAt(k.End()); index++ {
			a, b := c.Chunk(index), c.Chunk(index+1)
			last := a.Mesh.Vertices[len(a.Mesh.Vertices)-columns:]
			for i, v := range b.Mesh.Vertices[:columns] {
				if last[i].Position != v.Position {
					t.Fatalf("chunk %d/%d seam differs at column %d", index, index+1, i)
				}
			}
		}
	}
}

func TestSectionObstaclesStayOnThePath(t *testing.T) {
	c := New(12)
	for _, kind := range []SectionKind{Ridge, Narrows} {
		k := firstSection(t, c, kind)
		for index := ChunkAt(k.Start); index <= ChunkAt(k.End()); index++ {
			for _, o := range c.Obstacles(index) {
				p, ok := c.profileAt(o.Distance)
				if !ok {
					continue // valley chunk ends
				}
				if p.corridor < 0.99 {
					t.Errorf("obstacle at s=%v where the section's path is still joining the valley", o.Distance)
				}
				if u := abs(o.Base[0] - c.PathCentre(o.Distance)); u > c.PathHalfWidth(o.Distance) && !o.Scenery {
					t.Errorf("obstacle at s=%v is %v m off the path centre, past its edge", o.Distance, u)
				}
			}
		}
	}
}

// TestSectionsAreBusyButPassable: ridges and narrows are thick with rocks
// and pines, on the path and off it, yet wherever something stands on the
// path there's still a lane past it.
func TestSectionsAreBusyButPassable(t *testing.T) {
	c := New(12)
	for _, kind := range []SectionKind{Ridge, Narrows} {
		k := firstSection(t, c, kind)
		var onPath []Obstacle
		scenery, length := 0, float32(0)
		for s := k.Start; s < k.End(); s++ {
			if p, ok := c.profileAt(s); ok && p.corridor > 0.99 {
				length++
			}
		}
		for index := ChunkAt(k.Start); index <= ChunkAt(k.End()); index++ {
			for _, o := range c.Obstacles(index) {
				if _, ok := c.SectionAt(o.Distance); !ok {
					continue
				}
				if o.Scenery {
					scenery++
				} else {
					onPath = append(onPath, o)
				}
			}
		}
		per100 := float32(len(onPath)) / length * 100
		t.Logf("kind %v: %.0f m: %d on the path (%.0f per 100 m), %d scenery", kind, length, len(onPath), per100, scenery)
		if per100 < 25 {
			t.Errorf("kind %v: only %.0f obstacles per 100 m on the path: too bare", kind, per100)
		}
		if float32(scenery)/length*100 < 15 {
			t.Errorf("kind %v: only %d scenery pieces over %.0f m: the walls and slopes are bare", kind, scenery, length)
		}
		// A lane: at every metre, the widest gap between things on the path
		// (and the path's edges) fits the ball with room to spare.
		for s := k.Start; s < k.End(); s++ {
			if p, ok := c.profileAt(s); !ok || p.corridor < 0.99 {
				continue
			}
			pc, hw := c.PathCentre(s), c.PathHalfWidth(s)
			type span struct{ lo, hi float32 }
			var blocked []span
			for _, o := range onPath {
				if dz := abs(o.Distance - s); dz < o.Radius+0.5 {
					u := o.Base[0] - pc
					blocked = append(blocked, span{u - o.Radius, u + o.Radius})
				}
			}
			sort.Slice(blocked, func(i, j int) bool { return blocked[i].lo < blocked[j].lo })
			best, at := float32(0), -hw
			for _, b := range blocked {
				best = max(best, b.lo-at)
				at = max(at, b.hi)
			}
			best = max(best, hw-at)
			if best < 2 {
				t.Errorf("kind %v: at s=%.0f the widest lane is %.1f m", kind, s, best)
				break
			}
		}
	}
}
