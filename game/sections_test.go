package game

import (
	"testing"

	"CliffCrack/game/course"
)

// firstSection finds the first section of kind on c.
func firstSection(t *testing.T, c *course.Course, kind course.SectionKind) course.Section {
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
	return course.Section{}
}

func rideFrom(r *ride, s, u, speed float32) { r.placeAt(s, u, speed) }

// valleyStretch finds a stretch of plain valley at least length long.
func valleyStretch(t *testing.T, c *course.Course, length float32) float32 {
	t.Helper()
	for s := float32(100); s < 20000; s += 10 {
		k, ok := c.NextSection(s)
		if _, in := c.SectionAt(s); !in && (!ok || k.Start > s+length) {
			return s
		}
	}
	t.Fatal("no plain valley")
	return 0
}

func noObstacles(int) []course.Obstacle { return nil }

func TestBankPushesTheBallBackToThePath(t *testing.T) {
	c := course.New(12)
	r := newRide(c, noObstacles)
	s := valleyStretch(t, c, 400)
	w := c.HalfWidth(s)
	rideFrom(r, s, w+14, 20) // well up the right-hand bank
	for i := 0; i < 4*60 && !r.crashed; i++ {
		r.step(1.0/60, rideInput{})
	}
	if r.crashed {
		t.Fatalf("crashed: %s", r.cause)
	}
	if out, _ := r.bankOut(); out > rideBankFree+2 {
		t.Errorf("after 4 s with no input the ball is still %v m up the bank", out)
	}
}

func TestSnowballsRollAtABallOnTheBankButDontEndTheRun(t *testing.T) {
	c := course.New(12)
	r := newRide(c, noObstacles)
	s := valleyStretch(t, c, 400)
	rideFrom(r, s, c.HalfWidth(s)+12, 18)
	spawned := 0
	for i := 0; i < 6*60 && !r.crashed; i++ {
		// Fight the push: keep steering up the bank.
		r.step(1.0/60, rideInput{steer: 1})
		spawned = max(spawned, len(r.snowballs))
	}
	if spawned == 0 {
		t.Fatal("no snowballs rolled at a ball staying up on the bank")
	}
	if r.crashed && r.cause != "lost on the mountain" {
		t.Errorf("snowballs should knock you about, not end the run: %s", r.cause)
	}
}

func TestFallingOffTheRidgeEndsTheRun(t *testing.T) {
	c := course.New(12)
	k := firstSection(t, c, course.Ridge)
	r := newRide(c, noObstacles)
	mid := k.Start + k.Length/2
	rideFrom(r, mid, 0, 20)
	r.ball.Velocity[0] = k.Side * 15 // straight off the far edge
	for i := 0; i < 4*60 && !r.crashed; i++ {
		r.step(1.0/60, rideInput{})
	}
	if r.cause != "fell off the ridge" {
		t.Errorf("cause = %q, want falling off the ridge", r.cause)
	}
}

// The autopilot follows the path line; if it gets through a section's climb,
// crest and descent, a player can too.
func TestSectionsAreRideable(t *testing.T) {
	for _, kind := range []course.SectionKind{course.Ridge, course.Narrows} {
		c := course.New(12)
		k := firstSection(t, c, kind)
		r := newRide(c, noObstacles)
		rideFrom(r, k.Start-40, 0, 25)
		for i := 0; i < 90*60 && !r.crashed && r.s() < k.End()+40; i++ {
			r.step(1.0/60, autopilot(r))
		}
		t.Logf("kind %v: section %.0f-%.0f m, ball reached %.0f m after %.1f s", kind, k.Start, k.End(), r.s(), r.time)
		if r.s() < k.End() && !r.crashed {
			t.Errorf("kind %v: the ride stopped short of the section's end", kind)
		}
		if r.crashed {
			t.Errorf("kind %v: crashed at %.0f m (%.0f m into the section): %s",
				kind, r.distance, r.distance-k.Start, r.cause)
		}
	}
}

// TestSnowballsKnockAWallRiderBackIntoTheNarrows holds the ball against a
// gorge wall, climbing it: snowballs tumble off the wall into it and knock
// it back onto the gorge floor, again and again.
func TestSnowballsKnockAWallRiderBackIntoTheNarrows(t *testing.T) {
	c := course.New(12)
	k := firstSection(t, c, course.Narrows)
	r := newRide(c, noObstacles)
	s := k.Start + 110 // past the pinch, in the gorge proper
	rideFrom(r, s, c.PathHalfWidth(s)+3, 25)
	hits, knocked := 0, 0
	var top float32
	lastHit := float32(-10)
	for i := 0; i < 5*60 && !r.crashed && r.s() < k.End()-100; i++ {
		r.step(1.0/60, rideInput{steer: 1}) // keep trying to climb the right wall
		for _, sb := range r.snowballs {
			if sb.hit && sb.age >= 0 {
				hits++
				sb.age = -1000 // (counted)
				lastHit = r.time
			}
		}
		out, _, _ := r.wallOut()
		top = max(top, out)
		if out < 0 && r.time-lastHit < 1 {
			knocked++ // back on the floor, just after a hit
			lastHit = -10
		}
	}
	t.Logf("%d hits, knocked back onto the floor %d times; at most %.1f m up the wall", hits, knocked, top)
	if hits == 0 {
		t.Fatal("no snowballs struck a ball climbing the gorge wall")
	}
	if knocked < 2 {
		t.Errorf("knocked back onto the floor %d times in 5 s of climbing the wall; want it again and again", knocked)
	}
	if top > 8 {
		t.Errorf("the ball got %.1f m up the wall: the snowballs should keep it low", top)
	}
	if r.crashed {
		t.Errorf("snowballs should knock you about, not end the run: %s", r.cause)
	}
}
