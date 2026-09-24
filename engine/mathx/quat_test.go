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
