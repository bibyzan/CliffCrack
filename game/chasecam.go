package game

import (
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/game/course"
)

const (
	camMouseSens     = 0.0025 // radians per pixel of mouse (or finger) movement
	camStickYaw      = 2.6    // radians per second at full right-stick tilt
	camStickPitch    = 1.6
	camRecenterAfter = 1.2 // seconds without look input before the camera swings back behind
	camRecenterRate  = 2.5 // 1/s: how quickly it swings back
	camMinElev       = 0.05
	camMaxElev       = 1.35 // radians above the horizon, seen from the ball
	camFollowRate    = 15   // 1/s: how tightly the camera tracks the ball
	camTurnRate      = 4    // 1/s: how quickly it swings round to a new direction of travel
)

// lookInput turns the camera this frame (radians): yaw to the right, and
// elevation up (a negative elev looks further up from lower down).
type lookInput struct {
	yaw, elev float32
}

// chaseCam orbits the ball: by default behind and above it, looking down the
// slope, widening its field of view with speed. The player can turn it
// (mouse, right stick, touch drag); after a moment without input it swings
// back behind the ball. Looking around never changes the steering, which
// follows the direction of travel.
type chaseCam struct {
	pivot    mathx.Vec3 // the ball, smoothed
	yaw      float32    // direction of travel, smoothed (camera.Direction's convention)
	lookYaw  float32    // the player's turn around the ball
	lookElev float32    // the player's change of height angle
	idle     float32    // seconds since the last look input
	fov      float32

	eye, target mathx.Vec3 // this frame's result
}

func newChaseCam(rd *ride) chaseCam {
	c := chaseCam{fov: 50 * math.Pi / 180, idle: camRecenterAfter}
	c.pivot, _ = rd.pose(rd.ball)
	c.yaw = headingYaw(rd.heading)
	c.place(rd, rd.course)
	return c
}

// headingYaw is the yaw that looks along a horizontal direction.
func headingYaw(h mathx.Vec3) float32 {
	return float32(math.Atan2(float64(h[0]), float64(-h[2])))
}

func (c *chaseCam) update(dt float32, rd *ride, crs *course.Course, look lookInput) {
	p, _ := rd.pose(rd.ball)
	if rd.crashed {
		p = rd.crashPos // stay by the wreck; the player can still look around it
	}
	c.pivot = c.pivot.Add(p.Sub(c.pivot).Scale(smoothing(camFollowRate, dt)))
	if !rd.crashed {
		c.yaw = wrapAngle(c.yaw + wrapAngle(headingYaw(rd.heading)-c.yaw)*smoothing(camTurnRate, dt))
	}

	if look.yaw != 0 || look.elev != 0 {
		c.lookYaw = wrapAngle(c.lookYaw + look.yaw)
		c.lookElev = clampf(c.lookElev+look.elev, -math.Pi/2, math.Pi/2)
		c.idle = 0
	} else {
		c.idle += dt
	}
	if c.idle > camRecenterAfter && !rd.crashed {
		k := 1 - smoothing(camRecenterRate, dt)
		c.lookYaw *= k
		c.lookElev *= k
	}

	c.place(rd, crs)
	fov := (50 + min(rd.speed(), 55)*0.45) * math.Pi / 180
	c.fov += (float32(fov) - c.fov) * smoothing(6, dt)
}

// place puts the camera on its orbit: the distance and default height grow
// with speed, the player's look offsets turn and tilt it around the ball.
func (c *chaseCam) place(rd *ride, crs *course.Course) {
	speed := rd.speed()
	back := 8 + speed*0.08
	up := 3.6 + speed*0.03
	dist := float32(math.Hypot(float64(back), float64(up)))
	base := float32(math.Atan2(float64(up), float64(back)))
	c.lookElev = clampf(c.lookElev, camMinElev-base, camMaxElev-base)
	elev := float64(base + c.lookElev)

	forward := camera.Direction(c.yaw+c.lookYaw, 0)
	c.eye = c.pivot.
		Sub(forward.Scale(dist * float32(math.Cos(elev)))).
		Add(mathx.Vec3{0, dist * float32(math.Sin(elev)), 0})
	if ground := crs.Height(c.eye[0], c.eye[2]) + 1.2; c.eye[1] < ground {
		c.eye[1] = ground // never inside the mountain
	}
	// Look a little ahead of the ball when facing downhill; turned away, aim
	// at the ball itself so it stays in the middle of the picture.
	ahead := 5 * max(0, float32(math.Cos(float64(c.lookYaw))))
	c.target = c.pivot.Add(forward.Scale(ahead)).Add(mathx.Vec3{0, 0.4, 0})
}

func (c *chaseCam) view() (mathx.Mat4, mathx.Vec3) {
	return mathx.LookAt(c.eye, c.target, mathx.Vec3{0, 1, 0}), c.eye
}

// smoothing is the fraction of the way to close this frame when easing
// towards a target at rate (1/s), independent of frame rate.
func smoothing(rate, dt float32) float32 {
	return float32(1 - math.Exp(-float64(rate*dt)))
}

// wrapAngle keeps an angle in [-pi, pi).
func wrapAngle(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a)+math.Pi, twoPi))
	if a < 0 {
		a += twoPi
	}
	return a - math.Pi
}
