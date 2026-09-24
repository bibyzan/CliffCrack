package physics

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func ground(w *World) *Body {
	g := NewBox(mathx.Vec3{50, 0.5, 50}, Static)
	g.Position = mathx.Vec3{0, -0.5, 0} // top face at y = 0
	if err := w.Add(g); err != nil {
		panic(err)
	}
	return g
}

func run(w *World, seconds float32) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		w.Update(1.0 / 60)
	}
}

func TestBallComesToRestOnGround(t *testing.T) {
	w := NewWorld()
	ground(w)
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{0, 5, 0}
	if err := w.Add(ball); err != nil {
		t.Fatal(err)
	}
	run(w, 6)
	if y := ball.Position[1]; math.Abs(float64(y-0.5)) > 0.02 {
		t.Errorf("ball rests at y = %v, want ~0.5 (radius above the ground)", y)
	}
	if v := ball.Velocity.Len(); v > 0.05 {
		t.Errorf("ball still moving at %v m/s after 6 s", v)
	}
	if !ball.Grounded {
		t.Error("resting ball should be grounded")
	}
	ball.Velocity[1] = 5
	w.Update(0.1)
	if ball.Grounded {
		t.Error("ball in the air should not be grounded")
	}
}

func TestWallContactIsNotGround(t *testing.T) {
	w := NewWorld()
	w.Gravity = mathx.Vec3{}
	wall := NewBox(mathx.Vec3{0.5, 5, 5}, Static)
	w.Add(wall)
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{0.9, 0, 0} // touching the wall's +X face
	w.Add(ball)
	w.Update(w.FixedStep)
	if ball.Grounded {
		t.Error("touching a vertical wall must not count as standing on ground")
	}
}

func TestBounceAndImpact(t *testing.T) {
	w := NewWorld()
	ground(w)
	ball := NewSphere(0.5, 1)
	ball.Restitution = 0.8
	ball.Position = mathx.Vec3{0, 3, 0}
	w.Add(ball)

	var peakAfterBounce float32
	bounced, impacts := false, 0
	for i := 0; i < 180; i++ {
		w.Update(1.0 / 60)
		impacts += len(w.Impacts())
		if ball.Velocity[1] > 0 {
			bounced = true
		}
		if bounced {
			peakAfterBounce = max(peakAfterBounce, ball.Position[1])
		}
	}
	// Falling 2.5 m, restitution 0.8 -> rebound height ~0.64 * 2.5 = 1.6 m above rest.
	if !bounced || peakAfterBounce < 1.6 || peakAfterBounce > 2.6 {
		t.Errorf("rebound peak y = %v (bounced %v), want about 2.1", peakAfterBounce, bounced)
	}
	if impacts == 0 {
		t.Error("landing should report an impact")
	}
}

func TestHeadOnCollisionConservesMomentum(t *testing.T) {
	w := NewWorld()
	w.Gravity = mathx.Vec3{}
	a, b := NewSphere(0.5, 1), NewSphere(0.5, 1)
	a.Restitution, b.Restitution = 1, 1
	a.Position, b.Position = mathx.Vec3{-2, 0, 0}, mathx.Vec3{2, 0, 0}
	a.Velocity, b.Velocity = mathx.Vec3{3, 0, 0}, mathx.Vec3{-1, 0, 0}
	w.Add(a)
	w.Add(b)
	run(w, 2)

	// Equal masses, elastic: velocities swap (damping takes a couple of %).
	if a.Velocity[0] > -0.9 || a.Velocity[0] < -1.05 || b.Velocity[0] < 2.8 || b.Velocity[0] > 3.05 {
		t.Errorf("after collision a.vx = %v, b.vx = %v; want about -1 and 3", a.Velocity[0], b.Velocity[0])
	}
	if p := a.Velocity[0] + b.Velocity[0]; math.Abs(float64(p-2)) > 0.1 {
		t.Errorf("momentum %v, want ~2", p)
	}
}

func TestRotatedBoxAndKinematicPush(t *testing.T) {
	w := NewWorld()
	ground(w)
	// A ramp: box rotated 20 degrees around Z. A ball dropped on it rolls downhill (-X).
	ramp := NewBox(mathx.Vec3{3, 0.2, 2}, Static)
	ramp.Position = mathx.Vec3{0, 1.5, 0}
	ramp.Rotation = mathx.AxisAngle(mathx.Vec3{0, 0, 1}, 20*math.Pi/180)
	w.Add(ramp)
	ball := NewSphere(0.3, 1)
	ball.Position = mathx.Vec3{0.5, 3, 0}
	w.Add(ball)
	run(w, 1)
	if ball.Velocity[0] >= -0.5 {
		t.Errorf("ball on a ramp tilted down towards -X has vx = %v, want clearly negative", ball.Velocity[0])
	}
	if ball.AngularVelocity.Len() < 1 {
		t.Errorf("friction should make the ball roll, angular speed %v", ball.AngularVelocity.Len())
	}

	// A kinematic paddle moving into a resting ball knocks it away.
	w2 := NewWorld()
	ground(w2)
	resting := NewSphere(0.5, 1)
	resting.Position = mathx.Vec3{0, 0.5, 0}
	w2.Add(resting)
	paddle := NewBox(mathx.Vec3{0.2, 1, 1}, Kinematic)
	paddle.Position = mathx.Vec3{-2, 1, 0}
	paddle.Velocity = mathx.Vec3{4, 0, 0}
	w2.Add(paddle)
	run(w2, 0.6)
	if resting.Velocity[0] < 1 {
		t.Errorf("kinematic paddle should push the ball, vx = %v", resting.Velocity[0])
	}
}

func TestSphereInsideBoxIsPushedOut(t *testing.T) {
	w := NewWorld()
	w.Gravity = mathx.Vec3{}
	box := NewBox(mathx.Vec3{1, 1, 1}, Static)
	w.Add(box)
	ball := NewSphere(0.25, 1)
	ball.Position = mathx.Vec3{0.8, 0, 0} // inside, nearest face is +X
	w.Add(ball)
	run(w, 1)
	if ball.Position[0] < 1.2 {
		t.Errorf("ball x = %v, want pushed out past the +X face (>= 1.25)", ball.Position[0])
	}
}

func TestAddValidatesAndRemove(t *testing.T) {
	w := NewWorld()
	if err := w.Add(NewBox(mathx.Vec3{1, 1, 1}, Dynamic)); err == nil {
		t.Error("dynamic boxes are unsupported and should be rejected")
	}
	if err := w.Add(NewSphere(0.5, 0)); err == nil {
		t.Error("zero-mass dynamic sphere should be rejected")
	}
	b := NewSphere(0.5, 1)
	w.Add(b)
	w.Remove(b)
	if len(w.Bodies()) != 0 {
		t.Error("Remove should drop the body")
	}
}

func TestFixedStepAccumulates(t *testing.T) {
	w := NewWorld()
	if n := w.Update(w.FixedStep / 2); n != 0 {
		t.Errorf("half a step ran %d steps", n)
	}
	if n := w.Update(w.FixedStep / 2); n != 1 {
		t.Errorf("accumulated half steps ran %d steps, want 1", n)
	}
	if n := w.Update(10); n != w.MaxSteps {
		t.Errorf("a long hitch ran %d steps, want capped at %d", n, w.MaxSteps)
	}
}
