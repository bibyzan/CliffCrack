package arena

import (
	"math/rand/v2"
	"testing"

	"CliffCrack/engine/mathx"
)

func chunk(c, h mathx.Vec3) *Chunk { return &Chunk{Centre: c, Half: h} }

func TestTouching(t *testing.T) {
	a := chunk(mathx.Vec3{0, 0, 0}, mathx.Vec3{0.5, 0.5, 0.5})
	cases := []struct {
		name string
		b    *Chunk
		want bool
	}{
		{"face to face", chunk(mathx.Vec3{1, 0, 0}, mathx.Vec3{0.5, 0.5, 0.5}), true},
		{"stacked", chunk(mathx.Vec3{0, 1, 0}, mathx.Vec3{0.5, 0.5, 0.5}), true},
		{"gap", chunk(mathx.Vec3{1.1, 0, 0}, mathx.Vec3{0.5, 0.5, 0.5}), false},
		{"edge only", chunk(mathx.Vec3{1, 1, 0}, mathx.Vec3{0.5, 0.5, 0.5}), false},
		{"thin pane against a face", chunk(mathx.Vec3{0.53, 0, 0}, mathx.Vec3{0.03, 0.4, 0.4}), true},
	}
	for _, c := range cases {
		if got := touching(a, c.b); got != c.want {
			t.Errorf("%s: touching = %v, want %v", c.name, got, c.want)
		}
	}
}

func allStanding(t *testing.T, s *Structure) {
	t.Helper()
	if u := s.unsupported(); len(u) != 0 {
		t.Fatalf("%s: %d chunks unsupported before any damage (first at %v)", s.Name, len(u), u[0].Centre)
	}
}

func TestBlueprintsStandUp(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	for _, build := range []func(*rand.Rand, mathx.Vec3, int) *Structure{House, Tower, Bunker, Glasshouse, Walls, Crates} {
		for turns := 0; turns < 4; turns++ {
			s := build(rng, mathx.Vec3{5, 0, -3}, turns)
			if len(s.Chunks) == 0 {
				t.Fatalf("%s has no chunks", s.Name)
			}
			allStanding(t, s)
			for _, c := range s.Chunks {
				if c.Centre[1]-c.Half[1] < -1e-4 {
					t.Fatalf("%s: chunk below ground at %v", s.Name, c.Centre)
				}
			}
		}
	}
}

func TestHouseHasADoor(t *testing.T) {
	s := House(rand.New(rand.NewPCG(2, 2)), mathx.Vec3{}, 0)
	// The door is centred in the front (+Z) wall, from the ground up to 2.2 m.
	door := mathx.Vec3{0, 1, 0}
	for _, c := range s.Chunks {
		if c.Centre[2] > 1 && c.distTo(mathx.Vec3{door[0], door[1], c.Centre[2]}) == 0 {
			t.Fatalf("a %s chunk blocks the doorway at %v", Materials[c.Mat].Name, c.Centre)
		}
	}
	glass := 0
	for _, c := range s.Chunks {
		if c.Mat == Glass {
			glass++
		}
	}
	if glass == 0 {
		t.Error("the house should have glazed windows")
	}
}

// breakWhere kills every standing chunk matching f.
func breakWhere(s *Structure, f func(*Chunk) bool) int {
	n := 0
	for _, c := range s.Chunks {
		if c.Alive && f(c) {
			c.Alive = false
			s.alive--
			n++
		}
	}
	return n
}

func TestLosingTheGroundFloorCollapsesAHouse(t *testing.T) {
	s := House(rand.New(rand.NewPCG(3, 3)), mathx.Vec3{}, 1)
	// Knock out the bottom row of every wall: nothing is grounded any more.
	breakWhere(s, func(c *Chunk) bool { return c.anchored })
	if got := len(s.unsupported()); got != s.Alive() {
		t.Errorf("%d of %d standing chunks unsupported, want all", got, s.Alive())
	}
}

func TestHoleInAWallDoesNotCollapseIt(t *testing.T) {
	s := House(rand.New(rand.NewPCG(4, 4)), mathx.Vec3{}, 0)
	// Punch a 2 x 2 panel hole in the back wall at mid height.
	breakWhere(s, func(c *Chunk) bool {
		return c.Centre[2] < -1 && c.Centre[1] > 0.8 && c.Centre[1] < 2.3 && abs(c.Centre[0]) < 1.1
	})
	if u := s.unsupported(); len(u) != 0 {
		t.Errorf("a hole left %d chunks hanging; the wall around it should hold", len(u))
	}
}

func TestLosingMostOfTheFootingBringsItDown(t *testing.T) {
	s := Tower(rand.New(rand.NewPCG(5, 5)), mathx.Vec3{}, 0)
	// Destroy grounded chunks one at a time, keeping just one column: while
	// it's connected, only the load rule can bring the tower down.
	var keep *Chunk
	for _, c := range s.Chunks {
		if c.anchored && c.Half[0] < 0.35 && c.Half[2] < 0.35 {
			keep = c
			break
		}
	}
	breakWhere(s, func(c *Chunk) bool { return c.anchored && c != keep })
	falling := s.unsupported()
	if len(falling) != s.Alive()-1 {
		t.Errorf("%d of %d standing chunks fall; with one column left of the footing, all but that stub should", len(falling), s.Alive())
	}
	for _, c := range falling {
		if c == keep {
			t.Error("the grounded stub itself shouldn't fall")
		}
	}
}

func TestTowerStandsOnThreeColumns(t *testing.T) {
	s := Tower(rand.New(rand.NewPCG(5, 5)), mathx.Vec3{}, 0)
	// Take out the ground-floor column at the -X -Z corner entirely.
	breakWhere(s, func(c *Chunk) bool {
		return c.Centre[1] < storey && c.Centre[0] < -2.5 && c.Centre[2] < -2.5 && c.Half[0] < 0.35 && c.Half[2] < 0.35
	})
	allStanding(t, s)

	// Take out every ground-floor column and wall: the tower comes down.
	breakWhere(s, func(c *Chunk) bool { return c.Centre[1] < storey-0.3 })
	if got := len(s.unsupported()); got == 0 || got != s.Alive() {
		t.Errorf("without its ground floor %d of %d chunks unsupported, want all", got, s.Alive())
	}
}

func TestQuarterTurnsKeepShape(t *testing.T) {
	a := House(rand.New(rand.NewPCG(6, 6)), mathx.Vec3{}, 0)
	b := House(rand.New(rand.NewPCG(6, 6)), mathx.Vec3{}, 1)
	if len(a.Chunks) != len(b.Chunks) {
		t.Fatalf("turned house has %d chunks, unturned %d", len(b.Chunks), len(a.Chunks))
	}
	extent := func(s *Structure) (x, z float32) {
		for _, c := range s.Chunks {
			x = max(x, abs(c.Centre[0])+c.Half[0])
			z = max(z, abs(c.Centre[2])+c.Half[2])
		}
		return
	}
	ax, az := extent(a)
	bx, bz := extent(b)
	if abs(ax-bz) > 1e-4 || abs(az-bx) > 1e-4 {
		t.Errorf("a quarter turn should swap the footprint: %vx%v vs %vx%v", ax, az, bx, bz)
	}
}

func TestGenerateSite(t *testing.T) {
	a, b := GenerateSite(42), GenerateSite(42)
	if len(a.Structures) != len(b.Structures) {
		t.Fatal("the same seed should give the same site")
	}
	total := 0
	for i, s := range a.Structures {
		if len(s.Chunks) != len(b.Structures[i].Chunks) {
			t.Fatal("the same seed should give the same structures")
		}
		total += len(s.Chunks)
		allStanding(t, s)
		for _, c := range s.Chunks {
			if abs(c.Centre[0])+c.Half[0] > siteHalf || abs(c.Centre[2])+c.Half[2] > siteHalf {
				t.Fatalf("%s chunk outside the site at %v", s.Name, c.Centre)
			}
			for _, sp := range a.Spawns {
				if c.distTo(sp.At) < 1.5 {
					t.Fatalf("%s chunk on top of a spawn point", s.Name)
				}
			}
		}
	}
	if len(a.Spawns) != 2 || a.Spawns[0].At[2] <= 0 || a.Spawns[1].At[2] >= 0 {
		t.Errorf("want a south spawn then a north one, got %+v", a.Spawns)
	}
	if total < 300 || total > 5000 {
		t.Errorf("site has %d chunks, want a few hundred to a few thousand", total)
	}
	if c := GenerateSite(43); len(c.Structures) == len(a.Structures) && len(c.Structures[0].Chunks) == len(a.Structures[0].Chunks) &&
		len(c.Structures[1].Chunks) == len(a.Structures[1].Chunks) {
		t.Log("seeds 42 and 43 happen to start alike") // not an error, just unlikely
	}
}
