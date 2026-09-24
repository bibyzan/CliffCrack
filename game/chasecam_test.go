package game

import (
	"math"
	"testing"

	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
)

// camOnFlat returns a ride with the ball resting on the course (not dropping)
// and a camera that has settled behind it.
func camOnFlat(t *testing.T) (*ride, *chaseCam) {
	t.Helper()
	r := newTestRide(1)
	const s = 60
	x := r.course.Centre(s)
	r.ball.Position = mathx.Vec3{x, r.course.Height(x, -s) + rideBallRadius, -s}
	r.ball.Teleported()
	c := newChaseCam(r)
	for i := 0; i < 60; i++ {
		c.update(1.0/60, r, r.course, lookInput{})
	}
	return r, &c
}

// sideways is how far the eye sits to the right of the ball, across the
// direction of travel.
func sideways(r *ride, c *chaseCam) float32 {
	right := r.heading.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	return c.eye.Sub(r.ball.Position).Dot(right)
}

func TestChaseCamStartsBehindTheBall(t *testing.T) {
	r, c := camOnFlat(t)
	behind := c.eye.Sub(r.ball.Position).Dot(r.heading)
	if behind > -5 {
		t.Errorf("eye is %v along the heading from the ball, want well behind", behind)
	}
	if c.eye[1] < r.ball.Position[1]+1 {
		t.Error("the camera should look down on the ball")
	}
}

func TestLookingTurnsTheCameraAroundTheBall(t *testing.T) {
	r, c := camOnFlat(t)
	// Turn a quarter circle to the right: the camera swings round to the left
	// side of the ball, still at the same distance.
	before := c.eye.Sub(r.ball.Position).Len()
	c.update(1.0/60, r, r.course, lookInput{yaw: math.Pi / 2})
	if side := sideways(r, c); side > -5 {
		t.Errorf("after turning right the eye is %v to the right of the ball, want well to its left", side)
	}
	if after := c.eye.Sub(r.ball.Position).Len(); math.Abs(float64(after-before)) > 0.5 {
		t.Errorf("orbit distance changed from %v to %v", before, after)
	}
	// The look should be immediate, not eased in over several frames.
	if math.Abs(float64(c.lookYaw)-math.Pi/2) > 1e-5 {
		t.Errorf("look yaw = %v, want the full turn at once", c.lookYaw)
	}
}

func TestCameraSwingsBackBehindWhenLeftAlone(t *testing.T) {
	r, c := camOnFlat(t)
	c.update(1.0/60, r, r.course, lookInput{yaw: 1.2, elev: 0.3})
	for i := 0; i < int(camRecenterAfter*60)-5; i++ {
		c.update(1.0/60, r, r.course, lookInput{})
	}
	if c.lookYaw < 1.1 {
		t.Errorf("the camera recentred too soon (yaw %v)", c.lookYaw)
	}
	for i := 0; i < 3*60; i++ {
		c.update(1.0/60, r, r.course, lookInput{})
	}
	if math.Abs(float64(c.lookYaw)) > 0.02 || math.Abs(float64(c.lookElev)) > 0.02 {
		t.Errorf("after 3 s idle the look offset is (%v, %v), want back to 0", c.lookYaw, c.lookElev)
	}
}

func TestLookingUpAndDownIsLimited(t *testing.T) {
	r, c := camOnFlat(t)
	c.update(1.0/60, r, r.course, lookInput{elev: 10}) // way over the top
	over := float64(c.eye.Sub(c.pivot).Normalize()[1])
	if over > math.Sin(camMaxElev)+0.01 {
		t.Errorf("camera went over the top: height ratio %v", over)
	}
	c.update(1.0/60, r, r.course, lookInput{elev: -20}) // way under
	if ground := r.course.Height(c.eye[0], c.eye[2]); c.eye[1] < ground+1 {
		t.Errorf("camera is under the snow: eye %v, ground %v", c.eye[1], ground)
	}
}

func TestLookingDoesNotChangeTheSteering(t *testing.T) {
	a, b := newTestRide(2), newTestRide(2)
	ca, cb := newChaseCam(a), newChaseCam(b)
	for i := 0; i < 5*60; i++ {
		in := rideInput{steer: 0.5}
		a.step(1.0/60, in)
		b.step(1.0/60, in)
		ca.update(1.0/60, a, a.course, lookInput{})
		cb.update(1.0/60, b, b.course, lookInput{yaw: 0.05}) // spinning the camera the whole time
	}
	if a.ball.Position != b.ball.Position {
		t.Errorf("the camera changed the ride: %v vs %v", a.ball.Position, b.ball.Position)
	}
}

func TestMouseLooksOnlyWhileCapturedOrDragging(t *testing.T) {
	settings := DefaultSettings()
	r := &Run{ride: newTestRide(3), settings: &settings}
	var in input.State
	in.NewFrame()
	in.MoveEvent(100, 100)
	in.NewFrame()
	in.MoveEvent(140, 90) // moved right and up

	r.debugOpen = true // the F1 window wants the mouse
	if l := r.look(&in, true, 1.0/60); l.yaw != 0 || r.locked {
		t.Errorf("with the debug window open a plain mouse move looked around: %+v", l)
	}
	r.debugOpen = false
	l := r.look(&in, true, 1.0/60)
	if !r.locked {
		t.Fatal("riding should capture the mouse")
	}
	if l.yaw <= 0 || l.elev >= 0 {
		t.Errorf("mouse right and up should turn right and look up, got %+v", l)
	}
}

func TestFieldOfViewFollowsTheSettingWhilePaused(t *testing.T) {
	r, c := camOnFlat(t)
	s := DefaultSettings()
	before := c.fov(s.fovRadians())
	// Paused: the slider moves, but no update runs.
	s.FOV += 20
	after := c.fov(s.fovRadians())
	if want := float32(20 * math.Pi / 180); math.Abs(float64(after-before-want)) > 1e-5 {
		t.Errorf("fov went from %v to %v without an update, want +%v", before, after, want)
	}
	// Speed still widens it on top.
	r.ball.Velocity = mathx.Vec3{0, 0, -40}
	for i := 0; i < 120; i++ {
		c.update(1.0/60, r, r.course, lookInput{})
	}
	if c.fov(s.fovRadians()) <= after+0.2 {
		t.Error("at speed the view should widen beyond the setting")
	}
}
