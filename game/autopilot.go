package game

import (
	"CliffCrack/game/course"
)

// autopilot steers a ride down the course. It rides the menu backdrop and
// makes headless runs testable. It is decent, not perfect.
//
// Every frame it scores candidate lines across the channel: each line runs
// from the ball straight to an offset from the centre a little way ahead, and
// costs more the closer it passes to obstacles, the further it strays from
// the centre and the more it differs from the line chosen last frame (so it
// commits instead of dithering). It then steers towards the best line with
// velocity damping.
func autopilot(r *ride) rideInput {
	b := r.ball
	s := r.s()
	speed := max(r.speed(), 5)
	look := clampf(speed*1.4, 14, 40)
	x0, vx := b.Position[0], b.Velocity[0]

	var near []course.Obstacle
	for index := course.ChunkAt(s); index <= course.ChunkAt(s+look); index++ {
		for _, o := range r.chunkFn(index) {
			if ahead := o.Distance - s; ahead > -1 && ahead < look {
				near = append(near, o)
			}
		}
	}

	w := r.course.HalfWidth(s + look)
	centre := r.course.Centre(s + look)
	best, bestCost := r.lane, float32(1e9)
	for off := -w + 2; off <= w-2; off += 0.75 {
		end := centre + off
		cost := 0.02*abs32(off) + 0.05*abs32(off-r.lane)
		for _, o := range near {
			// Where this line crosses the obstacle's distance down the course.
			f := clampf((o.Distance-s)/look, 0, 1)
			x := x0 + (end-x0)*f
			clear := o.Radius + rideBallRadius + 1.2
			if gap := abs32(x - o.Centre[0]); gap < clear {
				cost += (clear - gap) * (2 - f) * 10 // near misses soon are the worst
			}
		}
		if cost < bestCost {
			best, bestCost = off, cost
		}
	}
	r.lane = best

	// Steer so the sideways velocity takes us onto the line a bit before we
	// get there, damped by the sideways velocity we already have.
	reach := max(look/speed*0.6, 0.4)
	wantVx := clampf((centre+best-x0)/reach, -12, 12)
	in := rideInput{
		steer:    clampf((wantVx-vx)/3, -1, 1),
		throttle: 0.4,
	}
	if speed > 45 {
		in.throttle = -0.4
	}
	// Hop off the kicker lip when a crack is just ahead.
	if k, ok := r.course.CrackAt(s + 1.5); ok && s < k.S-0.5 && s > k.S-3 {
		in.jump = true
	}
	return in
}
