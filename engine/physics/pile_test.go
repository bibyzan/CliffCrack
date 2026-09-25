package physics

import (
	"math"
	"math/rand/v2"
	"testing"

	"CliffCrack/engine/mathx"
)

// energy is kinetic + rotational + potential energy of the dynamic bodies.
func energy(w *World) float64 {
	var e float64
	for _, b := range w.Bodies() {
		if b.Kind != Dynamic {
			continue
		}
		e += 0.5 * float64(b.Mass*b.Velocity.Dot(b.Velocity))
		e += 0.5 * float64(b.AngularVelocity.Dot(b.AngularVelocity)/b.invInertia)
		e += float64(b.Mass * -w.Gravity[1] * b.Position[1])
	}
	return e
}

// A walled pit with 60 mixed balls dropped into a heap: collisions must never
// create energy, and everything must settle inside the walls.
func TestPileConservesEnergyAndSettles(t *testing.T) {
	w := NewWorld()
	ground(w)
	for _, wall := range []struct{ pos, half mathx.Vec3 }{
		{mathx.Vec3{4.25, 0.5, 0}, mathx.Vec3{0.25, 1, 4.5}},
		{mathx.Vec3{-4.25, 0.5, 0}, mathx.Vec3{0.25, 1, 4.5}},
		{mathx.Vec3{0, 0.5, 4.25}, mathx.Vec3{4.5, 1, 0.25}},
		{mathx.Vec3{0, 0.5, -4.25}, mathx.Vec3{4.5, 1, 0.25}},
	} {
		b := NewBox(wall.half, Static)
		b.Position = wall.pos
		w.Add(b)
	}

	rng := rand.New(rand.NewPCG(1, 2))
	var balls []*Body
	for i := 0; i < 60; i++ {
		r := 0.2 + rng.Float32()*0.25
		a := rng.Float64() * 2 * math.Pi
		d := rng.Float64() * 2
		b := NewSphere(r, 4*r*r*r)
		b.Position = mathx.Vec3{float32(math.Cos(a) * d), 1 + 6*rng.Float32(), float32(math.Sin(a) * d)}
		b.Restitution = 0.25 + 0.3*rng.Float32()
		w.Add(b)
		balls = append(balls, b)
	}

	prev := energy(w)
	allowance := 0.01 * prev // depenetration may lift bodies slightly
	for step := 0; step < 900; step++ { // 15 s: a chaotic pile can leave a ball rolling for a while
		w.Update(1.0 / 60)
		e := energy(w)
		if e > prev+allowance {
			t.Fatalf("step %d: energy rose from %.3f to %.3f", step, prev, e)
		}
		prev = min(prev, e)
	}

	for i, b := range balls {
		p := b.Position
		if p[1] < b.Radius*0.9 || math.Abs(float64(p[0])) > 4 || math.Abs(float64(p[2])) > 4 {
			t.Errorf("ball %d escaped or sank: %v", i, p)
		}
		if v := b.Velocity.Len(); v > 0.3 {
			t.Errorf("ball %d (r %.2f) still moving at %.2f m/s after 15 s: pos %v vel %v ang %v", i, b.Radius, v, b.Position, b.Velocity, b.AngularVelocity)
		}
	}
}
