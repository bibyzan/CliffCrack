package course

import "testing"

// Power-ups turn up regularly down the course, on the path, clear of the
// obstacles and of the jumps, and the same for a seed every time.
func TestPowerUpsLieOnThePathClearOfObstacles(t *testing.T) {
	c := New(12)
	counts := map[PowerKind]int{}
	for index := 0; index < 5000/ChunkLength; index++ {
		for _, p := range c.PowerUps(index) {
			counts[p.Kind]++
			s := p.Distance
			if s < StartClear {
				t.Errorf("a power-up at s=%.0f, on the Drop", s)
			}
			if u := abs(p.Pos[0] - c.PathCentre(s)); u > c.PathHalfWidth(s) {
				t.Errorf("power-up at s=%.0f is %.1f m off the path centre, past its edge", s, u)
			}
			if h := p.Pos[1] - c.Height(p.Pos[0], p.Pos[2]); h < 0.5 || h > 2 {
				t.Errorf("power-up at s=%.0f floats %.1f m over the snow", s, h)
			}
			if c.inJumpZone(s) {
				t.Errorf("power-up at s=%.0f on a jump", s)
			}
			for _, o := range c.Obstacles(index) {
				if !o.Scenery && o.Centre.Sub(p.Pos).Len() < o.Radius+powerClear {
					t.Errorf("power-up at s=%.0f right by an obstacle", s)
				}
			}
		}
	}
	t.Logf("in 5 km: %d boosts, %d shields", counts[Boost], counts[Shield])
	if counts[Boost] < 10 || counts[Shield] < 5 {
		t.Errorf("too few power-ups in 5 km: %d boosts, %d shields", counts[Boost], counts[Shield])
	}
	again := New(12)
	for index := 10; index < 30; index++ {
		a, b := c.PowerUps(index), again.PowerUps(index)
		if len(a) != len(b) || (len(a) > 0 && a[0] != b[0]) {
			t.Fatalf("chunk %d's power-ups differ for the same seed", index)
		}
	}
}
