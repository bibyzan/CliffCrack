package physics

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func TestBallRestsOnFlatHeightfield(t *testing.T) {
	w := NewWorld()
	if err := w.Add(NewHeightfield(func(x, z float32) float32 { return 2 })); err != nil {
		t.Fatal(err)
	}
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{3, 6, -4}
	w.Add(ball)
	run(w, 5)
	if y := ball.Position[1]; math.Abs(float64(y-2.5)) > 0.02 {
		t.Errorf("ball rests at y = %v, want ~2.5", y)
	}
	if !ball.Grounded {
		t.Error("ball on a heightfield should be grounded")
	}
}

func TestBallRollsDownSlope(t *testing.T) {
	// A plane dropping 0.4 m per metre towards -Z (about 22 degrees).
	const grade = 0.4
	slope := func(x, z float32) float32 { return grade * z }
	w := NewWorld()
	w.LinearDamping, w.AngularDamping = 0, 0 // compare with the ideal
	w.Add(NewHeightfield(slope))
	ball := NewSphere(0.5, 1)
	ball.Friction = 0.9
	ball.Position = mathx.Vec3{0, 1, 0}
	w.Add(ball)
	run(w, 3)

	if z := ball.Position[2]; z > -10 {
		t.Errorf("after 3 s the ball is at z = %v, want well down the slope", z)
	}
	// It must stay on the surface, neither sinking in nor flying off.
	p := ball.Position
	n := TerrainNormal(slope, p[0], p[2], 0.25)
	dist := p.Sub(mathx.Vec3{p[0], slope(p[0], p[2]), p[2]}).Dot(n)
	if math.Abs(float64(dist-0.5)) > 0.05 {
		t.Errorf("ball centre is %v from the slope, want ~0.5", dist)
	}
	// Rolling without slipping: a = g sin(theta) * 5/7 for a solid sphere.
	sin := grade / math.Sqrt(1+grade*grade)
	want := 9.81 * sin * 5 / 7 * 3
	if v := float64(ball.Velocity.Len()); math.Abs(v-want) > want*0.15 {
		t.Errorf("speed after 3 s = %v, want ~%v (rolling)", v, want)
	}
}

func TestFastBallDoesNotTunnelThroughHeightfield(t *testing.T) {
	w := NewWorld()
	w.Add(NewHeightfield(func(x, z float32) float32 { return 0 }))
	ball := NewSphere(0.3, 1)
	ball.Position = mathx.Vec3{0, 1, 0}
	ball.Velocity = mathx.Vec3{0, -80, 0} // 0.67 m per step: straight past the surface
	w.Add(ball)
	run(w, 1)
	if y := ball.Position[1]; y < 0 {
		t.Errorf("ball went through the terrain: y = %v", y)
	}
}

func TestHeightfieldMustBeStatic(t *testing.T) {
	w := NewWorld()
	b := NewHeightfield(func(x, z float32) float32 { return 0 })
	b.Kind = Kinematic
	if err := w.Add(b); err == nil {
		t.Error("a kinematic heightfield should be rejected")
	}
	if err := w.Add(&Body{Kind: Static, Shape: Heightfield}); err == nil {
		t.Error("a heightfield without a height function should be rejected")
	}
}

func TestBallRollsOffALipInsteadOfBeingPushedBack(t *testing.T) {
	// A flat shelf at y = 0 for z > 0 with a sheer 20 m drop beyond it.
	cliff := func(x, z float32) float32 {
		if z > 0 {
			return 0
		}
		return -20
	}
	w := NewWorld()
	w.Add(NewHeightfield(cliff))
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{0, 0.5, 3}
	ball.Velocity = mathx.Vec3{0, 0, -6}
	ball.AngularVelocity = mathx.Vec3{-12, 0, 0} // already rolling
	w.LinearDamping, w.AngularDamping = 0, 0
	w.Add(ball)
	run(w, 1.2)
	if z := ball.Position[2]; z > -3.5 {
		t.Errorf("ball at z = %v: it should have carried on over the edge", z)
	}
	if vz := ball.Velocity[2]; vz > -5.5 {
		t.Errorf("forward speed %v: the edge should not throw the ball back", vz)
	}
}
