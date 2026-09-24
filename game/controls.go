package game

import (
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
)

const (
	mouseSensitivity = 0.0025 // radians per pixel
	flySpeed         = 5.0    // units per second; Shift triples it
	autoOrbitSpeed   = 0.15   // radians per second while idle
	autoOrbitDelay   = 3.0    // seconds without input before the orbit resumes
)

// cameraRig switches between an orbit camera (default) and a fly camera.
//
//	Orbit: drag with left/right mouse to rotate, scroll to zoom. Idles into a slow spin.
//	Fly:   Tab to toggle; hold right mouse to look, WASD to move, Q/E down/up, Shift faster.
type cameraRig struct {
	fly      bool
	orbit    camera.Orbit
	free     camera.Fly
	lens     camera.Lens
	idle     float32 // seconds since the last orbit input
	locked   bool    // the mouse cursor should be captured
	autoSpin bool    // orbit slowly when idle
}

func newCameraRig() cameraRig {
	return cameraRig{
		orbit: camera.Orbit{
			Target:   mathx.Vec3{0, 0.75, 0},
			Distance: 9.75,
			Yaw:      -math.Pi / 2,
			Pitch:    -0.395,
			MinDist:  1.5,
			MaxDist:  60,
		},
		lens:     camera.DefaultLens(),
		idle:     autoOrbitDelay, // start spinning straight away
		autoSpin: true,
	}
}

// update applies this frame's input. mouseFree is false while the debug UI is
// using the mouse; clicks, drags and scrolling then belong to the UI.
func (r *cameraRig) update(dt float32, in *input.State, mouseFree bool) {
	if in.Pressed(input.KeyTab) {
		r.fly = !r.fly
		if r.fly {
			r.free = r.orbit.ToFly()
		}
	}
	dx, dy := in.MouseDelta()
	yaw, pitch := float32(dx)*mouseSensitivity, float32(-dy)*mouseSensitivity

	if r.fly {
		r.locked = in.MouseDown(input.MouseRight) && (mouseFree || r.locked)
		if r.locked {
			r.free.Look(yaw, pitch)
		}
		speed := float32(flySpeed)
		if in.Down(input.KeyLeftShift) || in.Down(input.KeyRightShift) {
			speed *= 3
		}
		step := speed * dt
		r.free.Move(in.Axis(input.KeyS, input.KeyW)*step, in.Axis(input.KeyA, input.KeyD)*step,
			in.Axis(input.KeyQ, input.KeyE)*step)
		return
	}

	// A drag that started in the scene keeps going even if it crosses a UI window.
	dragging := (in.MouseDown(input.MouseLeft) || in.MouseDown(input.MouseRight)) && (mouseFree || r.locked)
	r.locked = dragging
	r.idle += dt
	if dragging {
		r.orbit.Rotate(yaw, pitch)
		r.idle = 0
	}
	if s := in.Scroll(); s != 0 && mouseFree {
		r.orbit.Zoom(float32(math.Pow(0.9, s)))
		r.idle = 0
	}
	if r.autoSpin && r.idle >= autoOrbitDelay {
		r.orbit.Rotate(autoOrbitSpeed*dt, 0)
	}
}

func (r *cameraRig) view() (view mathx.Mat4, eye mathx.Vec3) {
	if r.fly {
		return r.free.View(), r.free.Position
	}
	return r.orbit.View(), r.orbit.Eye()
}
