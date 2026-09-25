package physics

import (
	"math"
	"testing"

	"CliffCrack/engine/mathx"
)

func nearV(a, b mathx.Vec3) bool { return a.Sub(b).Len() < 1e-3 }

func TestRaycastNearestSphere(t *testing.T) {
	w := NewWorld()
	far := NewSphere(1, 1)
	far.Position = mathx.Vec3{0, 0, -10}
	close := NewSphere(0.5, 1)
	close.Position = mathx.Vec3{0, 0, -5}
	w.Add(far)
	w.Add(close)

	hit, ok := w.Raycast(mathx.Vec3{}, mathx.Vec3{0, 0, -3}, 100, nil) // dir needn't be unit
	if !ok || hit.Body != close {
		t.Fatalf("hit %+v, want the nearer sphere", hit)
	}
	if math.Abs(float64(hit.Distance-4.5)) > 1e-4 || !nearV(hit.Normal, mathx.Vec3{0, 0, 1}) {
		t.Errorf("distance %v normal %v, want 4.5 and +Z", hit.Distance, hit.Normal)
	}

	// Skipping the near sphere hits the far one; a short ray hits nothing.
	hit, ok = w.Raycast(mathx.Vec3{}, mathx.Vec3{0, 0, -1}, 100, func(b *Body) bool { return b == close })
	if !ok || hit.Body != far || math.Abs(float64(hit.Distance-9)) > 1e-4 {
		t.Errorf("with skip: %+v, want far sphere at 9", hit)
	}
	if _, ok := w.Raycast(mathx.Vec3{}, mathx.Vec3{0, 0, -1}, 4, nil); ok {
		t.Error("a 4 m ray shouldn't reach a sphere 4.5 m away")
	}
	if _, ok := w.Raycast(mathx.Vec3{}, mathx.Vec3{0, 0, 1}, 100, nil); ok {
		t.Error("a ray pointing away shouldn't hit")
	}
}

func TestRaycastRotatedBoxAndInside(t *testing.T) {
	w := NewWorld()
	box := NewBox(mathx.Vec3{1, 1, 1}, Static)
	box.Position = mathx.Vec3{5, 0, 0}
	box.Rotation = mathx.AxisAngle(mathx.Vec3{0, 1, 0}, math.Pi/4) // a corner faces the ray
	w.Add(box)

	hit, ok := w.Raycast(mathx.Vec3{}, mathx.Vec3{1, 0, 0}, 100, nil)
	if !ok || hit.Body != box {
		t.Fatal("ray along +X should hit the box")
	}
	if want := 5 - float32(math.Sqrt2); math.Abs(float64(hit.Distance-want)) > 1e-3 {
		t.Errorf("distance %v, want %v (the corner)", hit.Distance, want)
	}
	if hit.Normal.Dot(mathx.Vec3{1, 0, 0}) >= 0 || math.Abs(float64(hit.Normal.Len()-1)) > 1e-4 {
		t.Errorf("normal %v should be unit and face back towards the ray", hit.Normal)
	}

	// Aimed past the box: miss. Starting inside: hit at 0.
	if _, ok := w.Raycast(mathx.Vec3{0, 3, 0}, mathx.Vec3{1, 0, 0}, 100, nil); ok {
		t.Error("ray above the box should miss")
	}
	if hit, ok := w.Raycast(mathx.Vec3{5, 0, 0}, mathx.Vec3{0, 1, 0}, 100, nil); !ok || hit.Distance != 0 {
		t.Errorf("ray from inside: %+v, want a hit at 0", hit)
	}
}

func TestRaycastHeightfield(t *testing.T) {
	w := NewWorld()
	w.Add(NewHeightfield(func(x, z float32) float32 { return 0.5 * x })) // slope rising along +X
	hit, ok := w.Raycast(mathx.Vec3{0, 5, 0}, mathx.Vec3{0, -1, 0}, 100, nil)
	if !ok || math.Abs(float64(hit.Distance-5)) > 1e-3 {
		t.Fatalf("straight down onto y=0 at x=0: %+v", hit)
	}
	// Horizontal ray at y = 2 meets the slope at x = 4.
	hit, ok = w.Raycast(mathx.Vec3{0, 2, 0}, mathx.Vec3{1, 0, 0}, 100, nil)
	if !ok || math.Abs(float64(hit.Point[0]-4)) > 1e-2 {
		t.Errorf("horizontal ray: %+v, want x = 4", hit)
	}
	if hit.Normal[1] <= 0 || hit.Normal[0] >= 0 {
		t.Errorf("slope normal %v should point up and back towards -X", hit.Normal)
	}
}

func TestFixedRotationHoldsOnRamp(t *testing.T) {
	ramp := func(fixed bool) *Body {
		w := NewWorld()
		r := NewBox(mathx.Vec3{10, 0.5, 10}, Static)
		r.Rotation = mathx.AxisAngle(mathx.Vec3{0, 0, 1}, 20*math.Pi/180)
		r.Friction = 1
		w.Add(r)
		b := NewSphere(0.4, 80)
		b.Friction = 1
		b.FixedRotation = fixed
		n := r.Rotation.Rotate(mathx.Vec3{0, 1, 0})
		b.Position = n.Scale(0.5 + 0.4)
		w.Add(b)
		for range 180 {
			w.Update(1.0 / 60)
		}
		return b
	}
	if b := ramp(true); b.Velocity.Len() > 0.05 || b.AngularVelocity.Len() != 0 {
		t.Errorf("fixed-rotation body should stand still on a 20 degree ramp: v %v w %v",
			b.Velocity.Len(), b.AngularVelocity)
	}
	if b := ramp(false); b.Velocity.Len() < 0.5 {
		t.Errorf("a normal ball should roll down the ramp, v %v", b.Velocity.Len())
	}
}
