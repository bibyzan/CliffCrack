package game

// This package links the renderer DLL, so run its tests with renderer.dll on
// PATH (e.g. add build/bin). The tests themselves never touch the GPU.

import (
	"testing"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

func testArena() (*physics.World, *physics.Body) {
	w := physics.NewWorld()
	ground := physics.NewBox(mathx.Vec3{200, 0.5, 200}, physics.Static)
	ground.Position = mathx.Vec3{0, -0.5, 0}
	w.Add(ground)
	ball := physics.NewSphere(playerRadius, 1.5)
	ball.Friction = 0.9
	ball.Position = mathx.Vec3{0, playerRadius, 0}
	w.Add(ball)
	w.Update(0.2) // settle
	return w, ball
}

func TestRollControlRollsTheBall(t *testing.T) {
	w, ball := testArena()
	forward := mathx.Vec3{0, 0, -1}
	for i := 0; i < 120; i++ { // hold W for 2 s
		rollControl(ball, forward, false, 1.0/60)
		w.Update(1.0 / 60)
	}
	if z := ball.Position[2]; z > -3 {
		t.Errorf("after 2 s of forward input the ball is at z = %v, want well past -3", z)
	}
	if y := ball.Position[1]; y > playerRadius+0.05 {
		t.Errorf("ball should stay on the ground while rolling, y = %v", y)
	}
	// Rolling (not sliding): contact point speed ~ 0, i.e. v ~ w x (r * up).
	slip := ball.Velocity.Sub(ball.AngularVelocity.Cross(mathx.Vec3{0, playerRadius, 0})).Len()
	if slip > 0.5 {
		t.Errorf("ball is sliding, not rolling: slip speed %v m/s", slip)
	}
	if top := float32(maxSpin * playerRadius * 1.15); ball.Velocity.Len() > top {
		t.Errorf("speed %v exceeds the rolling cap %v", ball.Velocity.Len(), top)
	}

	// Letting go: the brake stops it within a few seconds.
	for i := 0; i < 240; i++ {
		rollControl(ball, mathx.Vec3{}, false, 1.0/60)
		w.Update(1.0 / 60)
	}
	if v := ball.Velocity.Len(); v > 0.2 {
		t.Errorf("ball still rolling at %v m/s 4 s after release", v)
	}
}

func TestRollControlJumpsOnlyFromTheGround(t *testing.T) {
	w, ball := testArena()
	if !rollControl(ball, mathx.Vec3{}, true, 1.0/60) {
		t.Fatal("jump from the ground should work")
	}
	w.Update(0.1)
	if ball.Position[1] < playerRadius+0.3 {
		t.Errorf("after jumping the ball should be in the air, y = %v", ball.Position[1])
	}
	if rollControl(ball, mathx.Vec3{}, true, 1.0/60) {
		t.Error("double jump in mid-air should be refused")
	}
}

func TestPlayerBouncesOffKinematicCube(t *testing.T) {
	w, ball := testArena()
	cube := physics.NewBox(mathx.Vec3{0.5, 0.5, 0.5}, physics.Kinematic)
	cube.Position = mathx.Vec3{0, 0.5, -3}
	w.Add(cube)

	hit := false
	for i := 0; i < 180 && !hit; i++ {
		rollControl(ball, mathx.Vec3{0, 0, -1}, false, 1.0/60)
		w.Update(1.0 / 60)
		for _, imp := range w.Impacts() {
			if imp.A == cube || imp.B == cube {
				hit = true
			}
		}
	}
	if !hit {
		t.Fatal("rolling at the cube should report an impact with it")
	}
	if ball.Position[2] < -2.5+playerRadius-0.05 {
		t.Errorf("ball passed into the cube: z = %v", ball.Position[2])
	}
	if cube.Position != (mathx.Vec3{0, 0.5, -3}) {
		t.Error("a kinematic cube must not be moved by the ball")
	}
}
