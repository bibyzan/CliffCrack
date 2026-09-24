package camera

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func near(a, b mathx.Vec3) bool { return a.Sub(b).Len() < 1e-4 }

func TestDirectionConventions(t *testing.T) {
	cases := []struct {
		yaw, pitch float32
		want       mathx.Vec3
	}{
		{0, 0, mathx.Vec3{0, 0, -1}},            // default: down -Z
		{math.Pi / 2, 0, mathx.Vec3{1, 0, 0}},   // positive yaw turns right
		{0, math.Pi / 2, mathx.Vec3{0, 1, 0}},   // positive pitch looks up
		{0, -math.Pi / 2, mathx.Vec3{0, -1, 0}}, // negative pitch looks down
	}
	for _, c := range cases {
		if got := Direction(c.yaw, c.pitch); !near(got, c.want) {
			t.Errorf("Direction(%v, %v) = %v, want %v", c.yaw, c.pitch, got, c.want)
		}
	}
}

func TestFlyViewAndMove(t *testing.T) {
	c := Fly{Position: mathx.Vec3{0, 2, 10}}
	// A point straight ahead should be on the view axis at the right depth.
	if got := c.View().TransformPoint(mathx.Vec3{0, 2, 0}); !near(got, mathx.Vec3{0, 0, -10}) {
		t.Errorf("point ahead in view space = %v, want (0,0,-10)", got)
	}

	c.Pitch = -0.5 // looking down: strafing must not change height
	c.Move(0, 3, 0)
	if !near(c.Position, mathx.Vec3{3, 2, 10}) {
		t.Errorf("after strafe right, position = %v, want (3,2,10)", c.Position)
	}
	c.Move(0, 0, 1)
	if c.Position[1] != 3 {
		t.Errorf("after move up, y = %v, want 3", c.Position[1])
	}
}

func TestPitchClampAndYawWrap(t *testing.T) {
	var c Fly
	c.Look(0, 10)
	if c.Pitch != MaxPitch {
		t.Errorf("pitch = %v, want clamped to %v", c.Pitch, float32(MaxPitch))
	}
	for i := 0; i < 1000; i++ {
		c.Look(1, 0)
	}
	if c.Yaw < -math.Pi || c.Yaw >= math.Pi {
		t.Errorf("yaw %v escaped [-pi, pi)", c.Yaw)
	}
}

func TestOrbit(t *testing.T) {
	o := Orbit{Target: mathx.Vec3{0, 1, 0}, Distance: 5, Pitch: -math.Pi / 2 * 0.999, MinDist: 2, MaxDist: 20}
	eye := o.Eye()
	if d := eye.Sub(o.Target).Len(); math.Abs(float64(d-5)) > 1e-4 {
		t.Errorf("eye distance = %v, want 5", d)
	}
	if eye[1] <= o.Target[1] {
		t.Errorf("looking down, the eye (%v) should be above the target", eye)
	}
	if got := o.View().TransformPoint(o.Target); !near(got, mathx.Vec3{0, 0, -5}) {
		t.Errorf("target in view space = %v, want (0,0,-5)", got)
	}

	o.Zoom(0.1)
	if o.Distance != 2 {
		t.Errorf("zoom in: distance %v, want clamped to 2", o.Distance)
	}
	o.Zoom(100)
	if o.Distance != 20 {
		t.Errorf("zoom out: distance %v, want clamped to 20", o.Distance)
	}

	f := o.ToFly()
	if !near(f.Position, o.Eye()) || !near(f.Forward(), Direction(o.Yaw, o.Pitch)) {
		t.Error("ToFly should keep the same position and direction")
	}
}
