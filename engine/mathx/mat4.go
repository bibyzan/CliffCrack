// Package mathx holds the small amount of math the engine needs, using
// Vulkan conventions: column-major matrices, right-handed world space,
// clip-space Y pointing down (projections flip it so world +Y is up on
// screen), depth in [0, 1].
package mathx

import "math"

// Mat4 is column-major: element (row r, column c) is m[c*4+r].
type Mat4 [16]float32

func Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// Mul returns m * o (o is applied first).
func (m Mat4) Mul(o Mat4) Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += m[k*4+row] * o[c*4+k]
			}
			r[c*4+row] = sum
		}
	}
	return r
}

// TransformPoint applies m to the point v (w = 1).
func (m Mat4) TransformPoint(v Vec3) Vec3 {
	return Vec3{
		m[0]*v[0] + m[4]*v[1] + m[8]*v[2] + m[12],
		m[1]*v[0] + m[5]*v[1] + m[9]*v[2] + m[13],
		m[2]*v[0] + m[6]*v[1] + m[10]*v[2] + m[14],
	}
}

func Translate(x, y, z float32) Mat4 {
	m := Identity()
	m[12], m[13], m[14] = x, y, z
	return m
}

func Scale(x, y, z float32) Mat4 {
	m := Identity()
	m[0], m[5], m[10] = x, y, z
	return m
}

func RotateX(radians float32) Mat4 {
	s, c := sincos(radians)
	m := Identity()
	m[5], m[6] = c, s
	m[9], m[10] = -s, c
	return m
}

func RotateY(radians float32) Mat4 {
	s, c := sincos(radians)
	m := Identity()
	m[0], m[2] = c, -s
	m[8], m[10] = s, c
	return m
}

func RotateZ(radians float32) Mat4 {
	s, c := sincos(radians)
	m := Identity()
	m[0], m[1] = c, s
	m[4], m[5] = -s, c
	return m
}

// FromQuat builds a rotation from a unit quaternion (x, y, z, w).
func FromQuat(x, y, z, w float32) Mat4 {
	m := Identity()
	m[0] = 1 - 2*(y*y+z*z)
	m[1] = 2 * (x*y + z*w)
	m[2] = 2 * (x*z - y*w)
	m[4] = 2 * (x*y - z*w)
	m[5] = 1 - 2*(x*x+z*z)
	m[6] = 2 * (y*z + x*w)
	m[8] = 2 * (x*z + y*w)
	m[9] = 2 * (y*z - x*w)
	m[10] = 1 - 2*(x*x+y*y)
	return m
}

// Ortho maps the box [left,right]x[bottom,top]x[near,far] to Vulkan clip space,
// with world +Y pointing up on screen.
func Ortho(left, right, bottom, top, near, far float32) Mat4 {
	var m Mat4
	m[0] = 2 / (right - left)
	m[5] = 2 / (bottom - top)
	m[10] = 1 / (far - near)
	m[12] = -(right + left) / (right - left)
	m[13] = -(bottom + top) / (bottom - top)
	m[14] = -near / (far - near)
	m[15] = 1
	return m
}

// Perspective is a right-handed projection (camera looks down -Z) mapping
// view depth near..far to 0..1, with world +Y up on screen.
func Perspective(fovYRadians, aspect, near, far float32) Mat4 {
	f := float32(1 / math.Tan(float64(fovYRadians)/2))
	var m Mat4
	m[0] = f / aspect
	m[5] = -f
	m[10] = far / (near - far)
	m[11] = -1
	m[14] = near * far / (near - far)
	return m
}

// LookAt builds a right-handed view matrix.
func LookAt(eye, target, up Vec3) Mat4 {
	f := target.Sub(eye).Normalize()
	s := f.Cross(up).Normalize()
	u := s.Cross(f)
	return Mat4{
		s[0], u[0], -f[0], 0,
		s[1], u[1], -f[1], 0,
		s[2], u[2], -f[2], 0,
		-s.Dot(eye), -u.Dot(eye), f.Dot(eye), 1,
	}
}

// NormalMatrix returns the inverse-transpose of m's upper 3x3 (correct for
// non-uniform scale), laid out as 3 columns padded to vec4 like the shader expects.
func NormalMatrix(m Mat4) [12]float32 {
	a, b, c := m[0], m[4], m[8]
	d, e, f := m[1], m[5], m[9]
	g, h, i := m[2], m[6], m[10]

	// Cofactors; inverse-transpose = cofactor matrix / det.
	c00, c01, c02 := e*i-f*h, -(d*i - f*g), d*h-e*g
	c10, c11, c12 := -(b*i - c*h), a*i-c*g, -(a*h - b*g)
	c20, c21, c22 := b*f-c*e, -(a*f - c*d), a*e-b*d

	det := a*c00 + b*c01 + c*c02
	if det == 0 {
		return [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}
	}
	inv := 1 / det
	return [12]float32{
		c00 * inv, c10 * inv, c20 * inv, 0, // column 0
		c01 * inv, c11 * inv, c21 * inv, 0, // column 1
		c02 * inv, c12 * inv, c22 * inv, 0, // column 2
	}
}

func sincos(radians float32) (float32, float32) {
	s, c := math.Sincos(float64(radians))
	return float32(s), float32(c)
}
