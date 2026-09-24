package mathx

import (
	"math"
	"testing"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func nearVec(a, b Vec3) bool { return near(a[0], b[0]) && near(a[1], b[1]) && near(a[2], b[2]) }

// clip applies m to a point and returns normalized device coordinates.
func clip(m Mat4, v Vec3) Vec3 {
	x := m[0]*v[0] + m[4]*v[1] + m[8]*v[2] + m[12]
	y := m[1]*v[0] + m[5]*v[1] + m[9]*v[2] + m[13]
	z := m[2]*v[0] + m[6]*v[1] + m[10]*v[2] + m[14]
	w := m[3]*v[0] + m[7]*v[1] + m[11]*v[2] + m[15]
	return Vec3{x / w, y / w, z / w}
}

func TestPerspectiveDepthAndYFlip(t *testing.T) {
	p := Perspective(math.Pi/2, 1, 0.1, 100)
	if got := clip(p, Vec3{0, 0, -0.1}); !near(got[2], 0) {
		t.Errorf("near plane depth = %v, want 0", got[2])
	}
	if got := clip(p, Vec3{0, 0, -100}); !near(got[2], 1) {
		t.Errorf("far plane depth = %v, want 1", got[2])
	}
	// A point above the view axis must land in the top half (Vulkan NDC y < 0).
	if got := clip(p, Vec3{0, 1, -5}); got[1] >= 0 {
		t.Errorf("world +Y maps to NDC y = %v, want negative (top of screen)", got[1])
	}
}

func TestLookAt(t *testing.T) {
	v := LookAt(Vec3{0, 0, 5}, Vec3{}, Vec3{0, 1, 0})
	if got := v.TransformPoint(Vec3{}); !nearVec(got, Vec3{0, 0, -5}) {
		t.Errorf("target in view space = %v, want (0,0,-5)", got)
	}
	if got := v.TransformPoint(Vec3{1, 0, 0}); !nearVec(got, Vec3{1, 0, -5}) {
		t.Errorf("world +X in view space = %v, want (1,0,-5): right should stay right", got)
	}
}

func TestNormalMatrix(t *testing.T) {
	// Pure rotation: normal matrix equals the rotation.
	r := RotateY(0.7)
	n := NormalMatrix(r)
	for _, v := range []Vec3{{1, 0, 0}, {0, 1, 0}, {0.3, -0.2, 0.9}} {
		if got, want := TransformDir(n, v), r.TransformPoint(v); !nearVec(got, want) {
			t.Errorf("rotation normal of %v = %v, want %v", v, got, want)
		}
	}

	// Non-uniform scale: normals must stay perpendicular to transformed surfaces.
	s := Scale(4, 1, 1)
	surfaceDir := Vec3{1, 1, 0} // lies in a surface whose normal is (1,-1,0)
	normal := TransformDir(NormalMatrix(s), Vec3{1, -1, 0})
	if d := s.TransformPoint(surfaceDir).Dot(normal); !near(d, 0) {
		t.Errorf("scaled normal not perpendicular to scaled surface: dot = %v", d)
	}
}

func TestFromQuatMatchesAxisRotation(t *testing.T) {
	angle := float32(1.1)
	s, c := sincos(angle / 2)
	q := FromQuat(0, s, 0, c) // rotation about +Y
	r := RotateY(angle)
	for i := range q {
		if !near(q[i], r[i]) {
			t.Fatalf("FromQuat = %v, want %v", q, r)
		}
	}
}
