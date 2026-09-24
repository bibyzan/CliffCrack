package mathx

import "math"

type Vec3 [3]float32

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a Vec3) Scale(s float32) Vec3 { return Vec3{a[0] * s, a[1] * s, a[2] * s} }
func (a Vec3) Dot(b Vec3) float32   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a Vec3) Len() float32         { return float32(math.Sqrt(float64(a.Dot(a)))) }
func (a Vec3) Min(b Vec3) Vec3      { return Vec3{min(a[0], b[0]), min(a[1], b[1]), min(a[2], b[2])} }
func (a Vec3) Max(b Vec3) Vec3      { return Vec3{max(a[0], b[0]), max(a[1], b[1]), max(a[2], b[2])} }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

// Normalize returns a unit vector, or the zero vector if a has no length.
func (a Vec3) Normalize() Vec3 {
	l := a.Len()
	if l == 0 {
		return Vec3{}
	}
	return a.Scale(1 / l)
}

// TransformDir applies a NormalMatrix-style padded 3x3 to a direction.
func TransformDir(n [12]float32, v Vec3) Vec3 {
	return Vec3{
		n[0]*v[0] + n[4]*v[1] + n[8]*v[2],
		n[1]*v[0] + n[5]*v[1] + n[9]*v[2],
		n[2]*v[0] + n[6]*v[1] + n[10]*v[2],
	}
}
