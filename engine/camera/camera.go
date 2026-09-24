// Package camera provides view and projection helpers (pure Go).
//
// Angles: yaw 0 looks down -Z and increases turning right (clockwise seen from
// above); pitch 0 is level and increases looking up.
package camera

import (
	"math"

	"vkgame/engine/mathx"
)

// MaxPitch keeps cameras just short of straight up/down, where LookAt degenerates.
const MaxPitch = 89 * math.Pi / 180

var up = mathx.Vec3{0, 1, 0}

// Direction returns the unit view direction for a yaw and pitch.
func Direction(yaw, pitch float32) mathx.Vec3 {
	sy, cy := math.Sincos(float64(yaw))
	sp, cp := math.Sincos(float64(pitch))
	return mathx.Vec3{float32(sy * cp), float32(sp), float32(-cy * cp)}
}

// Lens is a perspective projection.
type Lens struct {
	FovY      float32 // vertical field of view, radians
	Near, Far float32
}

func DefaultLens() Lens {
	return Lens{FovY: math.Pi / 4, Near: 0.1, Far: 200}
}

func (l Lens) Projection(aspect float32) mathx.Mat4 {
	return mathx.Perspective(l.FovY, aspect, l.Near, l.Far)
}

// Fly is a free camera: position plus yaw/pitch.
type Fly struct {
	Position   mathx.Vec3
	Yaw, Pitch float32
}

func (c *Fly) Forward() mathx.Vec3 { return Direction(c.Yaw, c.Pitch) }

// Right is horizontal, so strafing never changes height.
func (c *Fly) Right() mathx.Vec3 { return Direction(c.Yaw, 0).Cross(up).Normalize() }

func (c *Fly) View() mathx.Mat4 {
	return mathx.LookAt(c.Position, c.Position.Add(c.Forward()), up)
}

// Look turns the camera by the given angles (radians); positive dPitch looks up.
func (c *Fly) Look(dYaw, dPitch float32) {
	c.Yaw = wrapAngle(c.Yaw + dYaw)
	c.Pitch = clamp(c.Pitch+dPitch, -MaxPitch, MaxPitch)
}

// Move translates along the view direction, the horizontal right vector and world up.
func (c *Fly) Move(forward, right, upward float32) {
	c.Position = c.Position.
		Add(c.Forward().Scale(forward)).
		Add(c.Right().Scale(right)).
		Add(up.Scale(upward))
}

// Orbit circles a target point at a distance.
type Orbit struct {
	Target     mathx.Vec3
	Distance   float32
	Yaw, Pitch float32 // direction the camera looks in; negative pitch looks down on the target
	MinDist    float32
	MaxDist    float32
}

func (c *Orbit) Eye() mathx.Vec3 {
	return c.Target.Sub(Direction(c.Yaw, c.Pitch).Scale(c.Distance))
}

func (c *Orbit) View() mathx.Mat4 {
	return mathx.LookAt(c.Eye(), c.Target, up)
}

// Rotate turns around the target (radians); positive dPitch raises the view direction.
func (c *Orbit) Rotate(dYaw, dPitch float32) {
	c.Yaw = wrapAngle(c.Yaw + dYaw)
	c.Pitch = clamp(c.Pitch+dPitch, -MaxPitch, MaxPitch)
}

// Zoom scales the distance by factor, clamped to [MinDist, MaxDist] when set.
func (c *Orbit) Zoom(factor float32) {
	c.Distance *= factor
	if c.MinDist > 0 {
		c.Distance = max(c.Distance, c.MinDist)
	}
	if c.MaxDist > 0 {
		c.Distance = min(c.Distance, c.MaxDist)
	}
}

// ToFly returns a fly camera with the same position and view direction.
func (c *Orbit) ToFly() Fly {
	return Fly{Position: c.Eye(), Yaw: c.Yaw, Pitch: c.Pitch}
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

// wrapAngle keeps an angle in [-pi, pi) so it never loses float precision.
func wrapAngle(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a)+math.Pi, twoPi))
	if a < 0 {
		a += twoPi
	}
	return a - math.Pi
}
