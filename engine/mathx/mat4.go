// Package mathx holds the small amount of math the engine needs, using
// Vulkan conventions: column-major matrices, clip-space Y pointing down,
// depth in [0, 1].
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

func RotateZ(radians float32) Mat4 {
	s, c := math.Sincos(float64(radians))
	m := Identity()
	m[0], m[1] = float32(c), float32(s)
	m[4], m[5] = float32(-s), float32(c)
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
