package mathx

import (
	"math"
	"testing"
)

func TestQuat(t *testing.T) {
	yaw := AxisAngle(Vec3{0, 1, 0}, math.Pi/2)
	if got := yaw.Rotate(Vec3{0, 0, -1}); !nearVec(got, Vec3{-1, 0, 0}) {
		t.Errorf("90 deg about +Y turns -Z into %v, want -X (counter-clockwise from above)", got)
	}

	// Mul applies the right-hand rotation first.
	pitch := AxisAngle(Vec3{1, 0, 0}, math.Pi/2)
	v := Vec3{0, 0, -1}
	if got, want := yaw.Mul(pitch).Rotate(v), yaw.Rotate(pitch.Rotate(v)); !nearVec(got, want) {
		t.Errorf("(yaw*pitch)(v) = %v, want yaw(pitch(v)) = %v", got, want)
	}

	if got := AxisAngle(Vec3{0, 0, 5}, 0.3).Mat4(); got != RotateZ(0.3) {
		for i := range got {
			if !near(got[i], RotateZ(0.3)[i]) {
				t.Fatalf("AxisAngle Z matrix = %v, want RotateZ = %v", got, RotateZ(0.3))
			}
		}
	}

	if q := (Quat{0, 0, 0, 2}).Normalize(); q != QuatIdentity() {
		t.Errorf("Normalize = %v, want identity", q)
	}
}

func TestDecompose(t *testing.T) {
	// Cover all four branches of the quaternion extraction.
	rotations := []Quat{
		QuatIdentity(),
		AxisAngle(Vec3{1, 0, 0}, 3),         // large X rotation
		AxisAngle(Vec3{0, 1, 0}, 3),         // large Y rotation
		AxisAngle(Vec3{0, 0, 1}, 3),         // large Z rotation
		AxisAngle(Vec3{1, 2, 3}, 0.7),       // general
		AxisAngle(Vec3{-1, 0.5, 0.2}, -2.5), // general, negative angle
	}
	for _, r := range rotations {
		m := Translate(1, -2, 3).Mul(r.Mat4()).Mul(Scale(2, 0.5, 3))
		pos, rot, scale := Decompose(m)
		if !nearVec(pos, Vec3{1, -2, 3}) || !nearVec(scale, Vec3{2, 0.5, 3}) {
			t.Errorf("Decompose(%v): pos %v scale %v", r, pos, scale)
		}
		// q and -q are the same rotation; compare by effect.
		for _, v := range []Vec3{{1, 0, 0}, {0, 1, 0}, {0.3, -0.4, 0.8}} {
			if got, want := rot.Rotate(v), r.Rotate(v); !nearVec(got, want) {
				t.Errorf("Decompose(%v) rotation maps %v to %v, want %v", r, v, got, want)
			}
		}
	}
	if c := AxisAngle(Vec3{0, 1, 0}, 1).Conjugate().Mul(AxisAngle(Vec3{0, 1, 0}, 1)); !nearVec(c.Rotate(Vec3{1, 0, 0}), Vec3{1, 0, 0}) {
		t.Error("q* q should be identity")
	}
}
