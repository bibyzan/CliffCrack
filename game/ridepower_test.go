package game

import (
	"testing"

	"CliffCrack/engine/mathx"
	"CliffCrack/game/course"
)

// firstPowerUp finds the first pickup of kind on r's course, and its chunk.
func firstPowerUp(t *testing.T, r *ride, kind course.PowerKind) (course.PowerUp, int) {
	t.Helper()
	for index := 0; index < 400; index++ {
		for _, p := range r.course.PowerUps(index) {
			if p.Kind == kind {
				return p, index
			}
		}
	}
	t.Fatalf("no power-up of kind %v", kind)
	return course.PowerUp{}, 0
}

// rideInto puts the ball just up the slope from p, rolling straight at it.
func rideInto(r *ride, p course.PowerUp, speed float32) {
	s := p.Distance - 6
	r.placeAt(s, p.Pos[0]-r.course.PathCentre(s), speed)
}

func TestABoostSpeedsTheBallUpForAWhile(t *testing.T) {
	c := course.New(12)
	speedAfter := func(take bool) (float32, bool) {
		r := newRide(c, noObstacles)
		p, index := firstPowerUp(t, r, course.Boost)
		rideInto(r, p, 30)
		if !take { // as if already taken
			r.taken = map[powerID]bool{}
			for i := range r.powerUps(index) {
				r.taken[powerID{index, i}] = true
			}
		}
		got := false
		for i := 0; i < 2*60 && !r.crashed; i++ {
			for _, k := range r.step(1.0/60, rideInput{}).picked {
				got = got || k == course.Boost
			}
		}
		return r.speed(), got
	}
	boosted, got := speedAfter(true)
	plain, _ := speedAfter(false)
	t.Logf("2 s on: %.0f km/h boosted, %.0f km/h without", boosted*3.6, plain*3.6)
	if !got {
		t.Fatal("rolling through the boost didn't pick it up")
	}
	if boosted < plain+8 {
		t.Errorf("a boost should make you much faster: %.1f m/s vs %.1f", boosted, plain)
	}
}

func TestAShieldSmashesThroughAnObstacle(t *testing.T) {
	r := newTestRide(4)
	var rock course.Obstacle
	for index := 8; rock.Radius == 0; index++ {
		for _, o := range r.chunkFn(index) {
			if o.Kind == course.Rock && !o.Scenery {
				rock = o
				break
			}
		}
	}
	at := rock.Centre.Add(mathx.Vec3{0, 0, 6})
	at[1] = r.course.Height(at[0], at[2]) + rideBallRadius
	r.ball.Position = at
	r.ball.Velocity = mathx.Vec3{0, -15 * course.GradeAt(-at[2]), -15}
	r.ball.Teleported()
	r.distance = -at[2]
	r.syncBodies()
	r.shield = rideShieldTime
	smashed := 0
	for i := 0; i < 90 && !r.crashed; i++ {
		smashed += len(r.step(1.0/60, rideInput{}).smashed)
	}
	if r.crashed {
		t.Fatalf("crashed with the shield up: %s", r.cause)
	}
	if smashed == 0 {
		t.Fatal("rolled into a rock with the shield up and nothing was smashed")
	}
	if len(r.shards) == 0 {
		t.Error("a smashed rock should fly apart")
	}
	if r.speed() < 10 {
		t.Errorf("the ball should carry on through: only %.1f m/s", r.speed())
	}
	// Once the shield's gone, the next one ends the run as usual.
	for i := 0; i < int(rideShieldTime*60) && r.shield > 0; i++ {
		r.updatePowers(1.0 / 60)
	}
	if r.shield != 0 {
		t.Error("the shield never ran out")
	}
}
