package course

import (
	"math"
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
		x := c.Centre(s)
		h := c.Height(x, -s)
		if h > prev {
			t.Errorf("course climbs between s=%v and s=%v (%v -> %v)", s-50, s, prev, h)
		}
		prev = h
		// The banks rise on both sides of the channel.
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
	cracks := c.Cracks(0, 2000)
	if len(cracks) < 5 {
		t.Fatalf("only %d cracks in the first 2 km", len(cracks))
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
	if total < 300 {
		t.Errorf("only %d obstacles in ~2.9 km", total)
	}
}

func TestStartIsAboveTheCliff(t *testing.T) {
	c := New(5)
	p := c.StartPosition()
	if drop := p[1] - c.Height(c.Centre(10), -10); drop < 20 {
		t.Errorf("the start is only %v m above the slope below the cliff", drop)
	}
}

func TestItGetsSteeperAndHarder(t *testing.T) {
	c := New(21)
	if a, b := GradeAt(100), GradeAt(3000); b < a*1.5 {
		t.Errorf("grade %v at 100 m vs %v at 3 km: the slope should steepen", a, b)
	}
	// Average obstacles per chunk early vs late.
	count := func(from, to int) float64 {
		n := 0
		for i := from; i < to; i++ {
			n += len(c.Obstacles(i))
		}
		return float64(n) / float64(to-from)
	}
	early, late := count(2, 12), count(60, 70)
	if late < early*1.5 {
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
