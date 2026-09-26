package game

import (
	"testing"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
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
	// Past the Drop (which is free to get as fast as it likes), riding the
	// valley flat out, the drag above cruise holds the speed.
	c := course.New(2)
	r := newRide(c, noObstacles)
	s := valleyStretch(t, c, 150)
	r.placeAt(s, 0, 75) // well over cruise
	top := float32(0)
	for i := 0; i < 5*60 && !r.crashed; i++ {
		if _, in := c.SectionAt(r.s()); in {
			break // (a ridge's pit is a long way down: not the valley any more)
		}
		in := autopilot(r)
		in.throttle = 0
		r.step(1.0/60, in)
		if i > 90 {
			top = max(top, r.speed())
		}
	}
	if top == 0 {
		t.Fatal("reached a section before measuring anything")
	}
	t.Logf("from 270 km/h, at most %.0f km/h after 1.5 s (cruise %.0f)", top*3.6, r.cruise()*3.6)
	if top > r.cruise()*1.2 {
		t.Errorf("still at %.0f km/h after 1.5 s: the drag above cruise should rein it in", top*3.6)
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
	at := rock.Centre.Add(mathx.Vec3{0, 0, 6})
	at[1] = r.course.Height(at[0], at[2]) + rideBallRadius // on the snow, uphill of it
	r.ball.Position = at
	r.ball.Velocity = mathx.Vec3{0, -15 * course.GradeAt(-at[2]), -15} // down the slope
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
	// Ride a stretch of plain valley early on and one far down, each from
	// the same speed, with nothing in the way: the far one is faster.
	c := course.New(2)
	average := func(from float32) float32 {
		r := newRide(c, noObstacles)
		s := from
		for ; ; s += 10 {
			k, ok := c.NextSection(s)
			if _, in := c.SectionAt(s); !in && (!ok || k.Start > s+250) { // (the autopilot jumps cracks)
				break
			}
		}
		r.placeAt(s, 0, 40)
		var sum float32
		n := 0
		for i := 0; i < 5*60 && !r.crashed; i++ {
			in := autopilot(r)
			in.throttle = 0
			r.step(1.0/60, in)
			if i > 60 {
				sum, n = sum+r.speed(), n+1
			}
		}
		t.Logf("from %.0f m: %d samples, ended at %.0f m (%s)", s, n, r.s(), r.cause)
		return sum / float32(n)
	}
	early, late := average(600), average(3000)
	t.Logf("average speed in plain valley %.0f km/h early, %.0f km/h 3 km down", early*3.6, late*3.6)
	if late < early*1.1 {
		t.Errorf("speed should build as you go: %.1f m/s early vs %.1f m/s late", early, late)
	}
}

// TestTheDropGetsYouFast: riding the Drop with no input, the ball is past
// 150 km/h within a few seconds.
func TestTheDropGetsYouFast(t *testing.T) {
	r := newTestRide(1)
	for i := 0; i < 5*60 && !r.crashed; i++ {
		r.step(1.0/60, rideInput{})
	}
	t.Logf("%.0f km/h after 5 s, %.0f m down", r.speed()*3.6, r.distance)
	if r.speed()*3.6 < 150 {
		t.Errorf("only %.0f km/h 5 s down the Drop", r.speed()*3.6)
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

// TestJumpIsOffTheSlope: down the steep Drop a jump pops the ball clear of
// the snow without cancelling its dive, so it doesn't sail far out.
func TestJumpIsOffTheSlope(t *testing.T) {
	r := newRide(course.New(1), func(int) []course.Obstacle { return nil })
	r.placeAt(100, 0, 50)
	for i := 0; i < 30 && !r.ball.Grounded; i++ { // settle onto the snow
		r.step(1.0/120, rideInput{})
	}
	before := r.ball.Velocity
	r.sinceJump = rideJumpCool
	if !r.step(1.0/120, rideInput{jump: true}).jumped {
		t.Fatal("no jump")
	}
	if r.ball.Velocity[1] > before[1]*0.5 {
		t.Errorf("the jump cancelled the dive: vertical speed %.1f -> %.1f m/s", before[1], r.ball.Velocity[1])
	}
	// Clearance square to the slope (straight down, a steep face makes any
	// hop look tall).
	top := float32(0)
	for i := 0; i < 120; i++ {
		r.step(1.0/120, rideInput{})
		p := r.ball.Position
		n := physics.TerrainNormal(r.course.Height, p[0], p[2], 0.5)
		top = max(top, (p[1]-r.course.Height(p[0], p[2]))*n[1]-rideBallRadius)
	}
	t.Logf("highest %.1f m off the slope", top)
	if top < 1.5 || top > 6 {
		t.Errorf("the jump took the ball %.1f m off the slope; want a hop, not a leap", top)
	}
}

// TestCracksCanStillBeJumped: at full speed, with nothing else in the way,
// the autopilot's jumps clear every crack for a couple of kilometres.
func TestCracksCanStillBeJumped(t *testing.T) {
	r := newRide(course.New(3), func(int) []course.Obstacle { return nil })
	for i := 0; i < 90*60 && !r.crashed && r.s() < 2500; i++ {
		r.step(1.0/60, autopilot(r))
	}
	cracks := len(r.course.Cracks(0, r.s()))
	t.Logf("reached %.0f m over %d cracks (%s)", r.distance, cracks, r.cause)
	if r.crashed && r.cause == "fell into a crack" {
		t.Errorf("fell into a crack at %.0f m", r.distance)
	}
	if cracks == 0 {
		t.Error("no cracks on the way: the test proves nothing")
	}
}

// TestTheRunStartsFlatOut: the ball starts at the Drop's terminal speed and
// keeps it, rather than hitting the slope and losing it.
func TestTheRunStartsFlatOut(t *testing.T) {
	r := newTestRide(1)
	low := r.speed()
	for i := 0; i < 60; i++ {
		r.step(1.0/60, rideInput{})
		low = min(low, r.speed())
	}
	t.Logf("slowest %.0f km/h in the first second", low*3.6)
	if low < rideStartSpeed*0.9 {
		t.Errorf("the ball dropped to %.0f km/h at the start", low*3.6)
	}
}

// TestJumpClearsTheSameOnAnySlope: in the valley, on a gentler slope, a
// jump clears the snow by about as much as it does down the Drop.
func TestJumpClearsTheSameOnAnySlope(t *testing.T) {
	c := course.New(1)
	clearance := func(s float32) float32 {
		r := newRide(c, func(int) []course.Obstacle { return nil })
		r.placeAt(s, 0, 40)
		for i := 0; i < 30 && !r.ball.Grounded; i++ {
			r.step(1.0/120, rideInput{})
		}
		r.sinceJump = rideJumpCool
		r.step(1.0/120, rideInput{jump: true})
		top := float32(0)
		for i := 0; i < 240; i++ {
			r.step(1.0/120, rideInput{})
			p := r.ball.Position
			n := physics.TerrainNormal(r.course.Height, p[0], p[2], 0.5)
			top = max(top, (p[1]-r.course.Height(p[0], p[2]))*n[1]-rideBallRadius)
		}
		return top
	}
	s := valleyStretch(t, c, 150)
	steep, gentle := clearance(100), clearance(s)
	t.Logf("clears %.1f m down the Drop, %.1f m in the valley", steep, gentle)
	if gentle < 2.5 || gentle > 6 {
		t.Errorf("a jump in the valley clears %.1f m: want a proper hop, not a nudge or a leap", gentle)
	}
}
