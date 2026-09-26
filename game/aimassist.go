package game

import (
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/mathx"
	"CliffCrack/game/arena"
)

// Aim assist, for gamepads and touch (never the mouse), and light:
//
//   - Slowdown: with the crosshair over (or just beside) an enemy you can
//     see, small, careful turns go slower, so it's easier to stay on them.
//     A flick isn't slowed (see flickFade): snapping to a head is yours.
//   - Pull: while you're moving (the stick, or yourself), the aim drifts
//     gently onto them, stronger with the sights up. Up and down it only
//     keeps you on their body, chest to head, so it never drags a headshot
//     down. It never snaps, and it doesn't aim for you when you're still.
const (
	assistRange    = 60   // m
	assistBase     = 0.02 // rad of cone round a target, on top of its size
	assistMaxCone  = 0.09 // rad
	assistSlowdown = 0.25 // careful turns slow by up to this much
	assistPull     = 0.4  // rad/s of pull at the hip, on target
	assistPullADS  = 0.7  // ... with the sights up
	assistBody     = 0.45 // m: how wide a target counts as, for the cone
	assistFlick    = 1.5  // rad/s of turning at which the slowdown has gone
)

// flickFade eases a slowdown (from aimAssist) off as the turn gets quicker,
// rate in rad/s: full for fine tracking, none for a flick.
func flickFade(slow, rate float32) float32 {
	return 1 - (1-slow)*clampf(1-rate/assistFlick, 0, 1)
}

// aimAssist is the stick's speed factor (1: none) and the look to add this
// frame (yaw, pitch) towards the best target near the crosshair. moving is
// whether the player's moving the stick or themself (the pull's only then).
func (m *Arena) aimAssist(moving bool, dt float32) (slow float32, pull [2]float32) {
	me, a := m.me(), m.sim()
	slow = 1
	if me.Dead || m.match.Phase == arena.PhaseCountdown {
		return
	}
	eye := me.Eye(1)
	yaw, pitch := me.ViewYaw(), me.ViewPitch()
	fwd := camera.Direction(yaw, pitch)
	best, bestStrength := (*arena.Player)(nil), float32(0)
	var bestYaw, lowPitch, highPitch float32
	for _, p := range a.Players {
		if p == me || p.Dead {
			continue
		}
		to := p.Chest().Sub(eye)
		dist := to.Len()
		if dist > assistRange || dist < 0.5 {
			continue
		}
		dir := to.Scale(1 / dist)
		angle := float32(math.Acos(float64(clampf(fwd.Dot(dir), -1, 1))))
		cone := min(assistBase+float32(math.Atan(float64(assistBody/dist))), assistMaxCone)
		if angle >= cone || !a.CanSee(eye, p.Chest()) {
			continue
		}
		if strength := 1 - angle/cone; strength > bestStrength {
			best, bestStrength = p, strength
			bestYaw = float32(math.Atan2(float64(dir[0]), float64(-dir[2])))
			pitchTo := func(at mathx.Vec3) float32 {
				return float32(math.Asin(float64(clampf(at.Sub(eye).Normalize()[1], -1, 1))))
			}
			lowPitch, highPitch = pitchTo(p.Chest()), pitchTo(p.Head())
		}
	}
	if best == nil {
		return
	}
	slow = 1 - assistSlowdown*bestStrength
	if !moving {
		return
	}
	rate := assistPull + (assistPullADS-assistPull)*me.ADS
	step := rate * bestStrength * dt
	dy := float32(math.Remainder(float64(bestYaw-yaw), 2*math.Pi))
	dp := float32(0) // anywhere from the chest to the head is on them
	switch {
	case pitch < lowPitch:
		dp = lowPitch - pitch
	case pitch > highPitch:
		dp = highPitch - pitch
	}
	pull[0] = clampf(dy, -step, step)
	pull[1] = clampf(dp, -step, step)
	return
}
