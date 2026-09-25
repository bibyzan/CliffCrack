package game

import (
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/game/arena"
)

// Aim assist, for gamepads only (never the mouse), and light:
//
//   - Slowdown: with the crosshair over (or just beside) an enemy you can
//     see, the right stick turns slower, so it's easier to stay on them.
//   - Pull: while you're moving (the stick, or yourself), the aim drifts
//     gently onto their chest, stronger with the sights up. It never snaps,
//     and it doesn't aim for you when you're still.
const (
	assistRange    = 60   // m
	assistBase     = 0.03 // rad of cone round a target, on top of its size
	assistMaxCone  = 0.14 // rad
	assistSlowdown = 0.45 // the stick's speed drops by up to this much
	assistPull     = 0.9  // rad/s of pull at the hip, on target
	assistPullADS  = 1.6  // ... with the sights up
	assistBody     = 0.55 // m: how wide a target counts as, for the cone
)

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
	var bestYaw, bestPitch float32
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
			bestPitch = float32(math.Asin(float64(clampf(dir[1], -1, 1))))
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
	dp := bestPitch - pitch
	pull[0] = clampf(dy, -step, step)
	pull[1] = clampf(dp, -step, step)
	return
}
