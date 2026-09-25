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

// Nlerp blends from a to b by t along the shorter arc (normalised lerp):
// cheap and smooth for the small angles between neighbouring frames.
func Nlerp(a, b Quat, t float32) Quat {
	if a.X*b.X+a.Y*b.Y+a.Z*b.Z+a.W*b.W < 0 {
		b = Quat{-b.X, -b.Y, -b.Z, -b.W}
	}
	s := 1 - t
	return Quat{a.X*s + b.X*t, a.Y*s + b.Y*t, a.Z*s + b.Z*t, a.W*s + b.W*t}.Normalize()
}

func (q Quat) Mat4() Mat4 { return FromQuat(q.X, q.Y, q.Z, q.W) }

// QuatFromBasis is the rotation taking the X, Y and Z axes to x, y and z,
// which must be orthonormal and right-handed (x = y cross z).
func QuatFromBasis(x, y, z Vec3) Quat {
	m := Identity()
	m[0], m[1], m[2] = x[0], x[1], x[2]
	m[4], m[5], m[6] = y[0], y[1], y[2]
	m[8], m[9], m[10] = z[0], z[1], z[2]
	_, q, _ := Decompose(m)
	return q
}

// LookRotation is a rotation taking +Z to dir (any length), keeping +Y as
// close to world up as it can.
func LookRotation(dir Vec3) Quat {
	z := dir.Normalize()
	up := Vec3{0, 1, 0}
	if math.Abs(float64(z.Dot(up))) > 0.999 {
		up = Vec3{1, 0, 0}
	}
	x := up.Cross(z).Normalize()
	return QuatFromBasis(x, z.Cross(x), z)
}

// Conjugate is the inverse rotation (for unit quaternions).
func (q Quat) Conjugate() Quat { return Quat{-q.X, -q.Y, -q.Z, q.W} }

// Decompose splits an affine translate*rotate*scale matrix (positive scale,
// no shear) into its parts.
func Decompose(m Mat4) (translation Vec3, rotation Quat, scale Vec3) {
	translation = Vec3{m[12], m[13], m[14]}
	cols := [3]Vec3{{m[0], m[1], m[2]}, {m[4], m[5], m[6]}, {m[8], m[9], m[10]}}
	for i, c := range cols {
		scale[i] = c.Len()
		if scale[i] != 0 {
			cols[i] = c.Scale(1 / scale[i])
		}
	}
	// Rotation matrix element (row r, col c) is cols[c][r]. Shepperd's method.
	m00, m11, m22 := cols[0][0], cols[1][1], cols[2][2]
	var q Quat
	switch trace := m00 + m11 + m22; {
	case trace > 0:
		s := float32(math.Sqrt(float64(trace+1))) * 2
		q = Quat{(cols[1][2] - cols[2][1]) / s, (cols[2][0] - cols[0][2]) / s, (cols[0][1] - cols[1][0]) / s, s / 4}
	case m00 > m11 && m00 > m22:
		s := float32(math.Sqrt(float64(1+m00-m11-m22))) * 2
		q = Quat{s / 4, (cols[1][0] + cols[0][1]) / s, (cols[2][0] + cols[0][2]) / s, (cols[1][2] - cols[2][1]) / s}
	case m11 > m22:
		s := float32(math.Sqrt(float64(1+m11-m00-m22))) * 2
		q = Quat{(cols[1][0] + cols[0][1]) / s, s / 4, (cols[2][1] + cols[1][2]) / s, (cols[2][0] - cols[0][2]) / s}
	default:
		s := float32(math.Sqrt(float64(1+m22-m00-m11))) * 2
		q = Quat{(cols[2][0] + cols[0][2]) / s, (cols[2][1] + cols[1][2]) / s, s / 4, (cols[0][1] - cols[1][0]) / s}
	}
	return translation, q.Normalize(), scale
}

// Rotate applies the rotation to a vector.
func (q Quat) Rotate(v Vec3) Vec3 { return q.Mat4().TransformPoint(v) }
