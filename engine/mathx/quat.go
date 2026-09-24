package mathx

import "math"

// Quat is a rotation quaternion (X, Y, Z vector part, W scalar part).
// The zero value is not a valid rotation; start from QuatIdentity.
type Quat struct{ X, Y, Z, W float32 }

func QuatIdentity() Quat { return Quat{W: 1} }

// AxisAngle returns a rotation of radians around axis (need not be unit length).
func AxisAngle(axis Vec3, radians float32) Quat {
	a := axis.Normalize()
	s, c := math.Sincos(float64(radians) / 2)
	return Quat{a[0] * float32(s), a[1] * float32(s), a[2] * float32(s), float32(c)}
}

// Mul returns q * r: the rotation that applies r first, then q.
func (q Quat) Mul(r Quat) Quat {
	return Quat{
		X: q.W*r.X + q.X*r.W + q.Y*r.Z - q.Z*r.Y,
		Y: q.W*r.Y - q.X*r.Z + q.Y*r.W + q.Z*r.X,
		Z: q.W*r.Z + q.X*r.Y - q.Y*r.X + q.Z*r.W,
		W: q.W*r.W - q.X*r.X - q.Y*r.Y - q.Z*r.Z,
	}
}

// Normalize returns q scaled to unit length (identity if q is zero).
func (q Quat) Normalize() Quat {
	l := float32(math.Sqrt(float64(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)))
	if l == 0 {
		return QuatIdentity()
	}
	return Quat{q.X / l, q.Y / l, q.Z / l, q.W / l}
}

func (q Quat) Mat4() Mat4 { return FromQuat(q.X, q.Y, q.Z, q.W) }

// Rotate applies the rotation to a vector.
func (q Quat) Rotate(v Vec3) Vec3 { return q.Mat4().TransformPoint(v) }
