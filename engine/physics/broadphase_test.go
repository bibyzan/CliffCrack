package physics

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

// tiledFloor lays n x n static 1 m tiles with their tops at y = 0.
func tiledFloor(w *World, n int) {
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			b := NewBox(mathx.Vec3{0.5, 0.25, 0.5}, Static)
			b.Position = mathx.Vec3{float32(i-n/2) + 0.5, -0.25, float32(j-n/2) + 0.5}
			w.Add(b)
		}
	}
}

func TestBroadphaseFindsNearbyStatics(t *testing.T) {
	w := NewWorld()
	tiledFloor(w, 50) // 2500 static tiles
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{3.2, 3, -7.9}
	w.Add(ball)
	for range 240 {
		w.Update(1.0 / 60)
	}
	if y := ball.Position[1]; math.Abs(float64(y-0.5)) > 0.03 {
		t.Errorf("ball on the tiled floor rests at y = %v, want 0.5", y)
	}
}

func TestBroadphaseSeesStaticsAddedAndMovedLater(t *testing.T) {
	w := NewWorld()
	w.Gravity = mathx.Vec3{}
	ball := NewSphere(0.5, 1)
	ball.Velocity = mathx.Vec3{4, 0, 0}
	w.Add(ball)
	w.Update(0.1) // builds the grid with no statics

	wall := NewBox(mathx.Vec3{0.5, 2, 2}, Static)
	wall.Position = mathx.Vec3{3, 0, 0}
	w.Add(wall) // must be picked up without any extra call
	for range 120 {
		w.Update(1.0 / 60)
	}
	if ball.Position[0] > 2.51 {
		t.Errorf("ball passed a wall added after the first step: x = %v", ball.Position[0])
	}

	// Moving a static needs StaticsChanged.
	wall.Position = mathx.Vec3{-10, 0, 0}
	w.StaticsChanged()
	ball.Position, ball.Velocity = mathx.Vec3{-7, 0, 0}, mathx.Vec3{-4, 0, 0}
	ball.Teleported()
	for range 120 {
		w.Update(1.0 / 60)
	}
	if ball.Position[0] < -9.51 {
		t.Errorf("ball passed a moved wall: x = %v", ball.Position[0])
	}
}

func TestBroadphaseRemovedStaticsStopColliding(t *testing.T) {
	w := NewWorld()
	floor := NewBox(mathx.Vec3{5, 0.5, 5}, Static)
	floor.Position = mathx.Vec3{0, -0.5, 0}
	w.Add(floor)
	ball := NewSphere(0.5, 1)
	ball.Position = mathx.Vec3{0, 0.5, 0}
	w.Add(ball)
	for range 30 {
		w.Update(1.0 / 60)
	}
	w.Remove(floor)
	for range 30 { // Update catches up at most MaxSteps per call, so step per frame
		w.Update(1.0 / 60)
	}
	if ball.Position[1] > 0 {
		t.Errorf("ball should fall once the floor is removed, y = %v", ball.Position[1])
	}
}

func TestIgnoredPairsPassThrough(t *testing.T) {
	w := NewWorld()
	w.Gravity = mathx.Vec3{}
	a, b := NewSphere(0.5, 1), NewSphere(0.5, 1)
	b.Position = mathx.Vec3{3, 0, 0}
	a.Velocity = mathx.Vec3{6, 0, 0}
	a.Ignore = b
	w.Add(a)
	w.Add(b)
	for range 60 {
		w.Update(1.0 / 60)
	}
	if a.Position[0] < 5 || b.Velocity.Len() != 0 {
		t.Errorf("ignored pair collided: a at %v, b moving %v", a.Position, b.Velocity)
	}
}

func BenchmarkStepThousandsOfStatics(b *testing.B) {
	w := NewWorld()
	tiledFloor(w, 60) // 3600 statics
	for i := 0; i < 100; i++ {
		s := NewSphere(0.2, 1)
		s.Position = mathx.Vec3{float32(i%10) - 5, 1 + float32(i/10)*0.5, 0}
		w.Add(s)
	}
	b.ResetTimer()
	for range b.N {
		w.step(w.FixedStep)
	}
}
