package game

import (
	"testing"

	"CliffCrack/engine/mathx"
	"CliffCrack/game/course"
)

// newTestRide starts a headless run with obstacles generated on demand.
func newTestRide(seed uint64) *ride {
	c := course.New(seed)
	cache := map[int][]course.Obstacle{}
	return newRide(c, func(index int) []course.Obstacle {
		if o, ok := cache[index]; ok {
			return o
		}
		cache[index] = c.Chunk(index).Obstacles
		return cache[index]
	})
}

func TestDropLandsAndRidesDownhill(t *testing.T) {
	r := newTestRide(1)
	for i := 0; i < 8*60; i++ { // 8 s, no input
		r.step(1.0/60, rideInput{})
		if r.crashed {
			break
		}
	}
	if !r.landed {
		t.Fatal("the ball never landed after the drop")
	}
	if r.distance < 60 {
		t.Errorf("after 8 s the ball is only %v m down the course (crashed: %v %q)", r.distance, r.crashed, r.cause)
	}
	p := r.ball.Position
	if ground := r.course.Height(p[0], p[2]); p[1] < ground {
		t.Errorf("ball is under the terrain: y %v, ground %v", p[1], ground)
	}
}

func TestSpeedIsCapped(t *testing.T) {
	r := newTestRide(2)
	top := float32(0)
	for i := 0; i < 20*60 && !r.crashed; i++ {
		in := autopilot(r)
		in.throttle = 0
		r.step(1.0/60, in)
		top = max(top, r.ball.Velocity.Len())
	}
	if top > 45 {
		t.Errorf("top speed %v m/s after 20 s: the cruise speed should hold it near 25-30", top)
	}
	if top < 18 {
		t.Errorf("top speed %v m/s: the run should get properly fast", top)
	}
}

func TestAutopilotGetsWellDownTheCourse(t *testing.T) {
	best := float32(0)
	for seed := uint64(1); seed <= 3; seed++ {
		r := newTestRide(seed)
		for i := 0; i < 60*60 && !r.crashed; i++ { // up to a minute
			r.step(1.0/60, autopilot(r))
		}
		t.Logf("seed %d: %.0f m in %.1f s (%s)", seed, r.distance, r.time, r.cause)
		best = max(best, r.distance)
	}
	if best < 400 {
		t.Errorf("the autopilot's best run is %v m; the course or steering is broken", best)
	}
}

func TestHittingAnObstacleEndsTheRun(t *testing.T) {
	r := newTestRide(4)
	// Put the ball on the course just above a rock, moving straight at it.
	var rock course.Obstacle
	for index := 2; rock.Radius == 0; index++ {
		for _, o := range r.chunkFn(index) {
			if o.Kind == course.Rock {
				rock = o
				break
			}
		}
	}
	r.ball.Position = rock.Centre.Add(mathx.Vec3{0, 0, 6})
	r.ball.Velocity = mathx.Vec3{0, 0, -15}
	r.distance = 0
	for i := 0; i < 120 && !r.crashed; i++ {
		r.step(1.0/60, rideInput{})
	}
	if !r.crashed || r.cause != "hit a rock" {
		t.Fatalf("crashed = %v, cause %q; want a rock hit", r.crashed, r.cause)
	}
	if len(r.debris) == 0 {
		t.Error("a crash should shatter the ball")
	}
}

func TestFallingIntoACrackEndsTheRun(t *testing.T) {
	r := newTestRide(5)
	k := r.course.Cracks(0, 5000)[0]
	s := k.S + k.Width/2
	x := r.course.Centre(s)
	r.ball.Position = mathx.Vec3{x, r.course.Rim(x, -s) + 1, -s}
	r.ball.Velocity = mathx.Vec3{}
	r.landed = true
	for i := 0; i < 180 && !r.crashed; i++ {
		r.step(1.0/60, rideInput{})
	}
	if r.cause != "fell into a crack" {
		t.Fatalf("cause = %q, want a crack fall", r.cause)
	}
}

func TestSpeedBuildsWithDistance(t *testing.T) {
	// Obstacle-free so the run gets far enough to compare.
	r := newRide(course.New(2), func(int) []course.Obstacle { return nil })
	var early, late, nEarly, nLate float32
	for i := 0; i < 120*60 && !r.crashed && r.s() < 1600; i++ {
		r.step(1.0/60, autopilot(r))
		switch s := r.s(); {
		case s > 200 && s < 400:
			early, nEarly = early+r.speed(), nEarly+1
		case s > 1400 && s < 1600:
			late, nLate = late+r.speed(), nLate+1
		}
	}
	if nEarly == 0 || nLate == 0 {
		t.Fatalf("run ended early at %v m: %s", r.distance, r.cause)
	}
	early, late = early/nEarly, late/nLate
	t.Logf("average speed %.0f km/h at 200-400 m, %.0f km/h at 1.4-1.6 km", early*3.6, late*3.6)
	if late < early*1.2 {
		t.Errorf("speed should build as you go: %.1f m/s early vs %.1f m/s late", early, late)
	}
}

func TestHoldingJumpJumpsOnce(t *testing.T) {
	r := newTestRide(6)
	for i := 0; i < 8*60 && !r.landed; i++ {
		r.step(1.0/60, rideInput{})
	}
	for i := 0; i < 60 && !r.ball.Grounded; i++ { // settle onto the snow
		r.step(1.0/60, rideInput{})
	}
	jumps := 0
	for i := 0; i < 12; i++ { // jump held for 0.1 s at the physics rate
		if r.step(1.0/120, rideInput{jump: true}).jumped {
			jumps++
		}
	}
	if jumps != 1 {
		t.Errorf("holding jump jumped %d times, want once", jumps)
	}
}
