package online

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/game/arena"
)

// A guest sees the match a round trip late: its inputs reach the host,
// which steps the match and sends back the result. Two things hide that:
//
//   - Prediction: the guest moves its own player at once with its own
//     input (Predictor). When a snapshot arrives it resets that player to
//     where the host had it, as of the last input the host had used, and
//     replays the inputs since. What's left of any difference is eased out
//     on screen rather than snapped.
//   - Interpolation: everyone else is drawn a little in the past, smoothly
//     between the host's snapshots (Interp), rather than jumping to each
//     snapshot as it lands, late or early.

// Predictor is a guest's record of its own inputs, for replaying.
type Predictor struct {
	history []predicted
	// Offset is how far the predicted player has jumped on corrections,
	// still to ease out on screen: draw the player at Body.Position + Offset.
	Offset mathx.Vec3
}

type predicted struct {
	seq        uint32
	in         arena.Input
	yaw, pitch float32
	dt         float32
}

const (
	maxHistory = 180 // inputs (3 s at 60 Hz): beyond that, the host's too far behind to matter
	snapFar    = 2.0 // m: a correction bigger than this snaps (a respawn, a launch)
	easeRate   = 10  // 1/s: how fast a correction eases out
)

// Step records this frame's input (sent to the host as seq) and moves the
// player by it.
func (pr *Predictor) Step(a *arena.Arena, p *arena.Player, seq uint32, in arena.Input, dt float32) {
	if len(pr.history) == maxHistory {
		pr.history = pr.history[1:]
	}
	pr.history = append(pr.history, predicted{seq: seq, in: in, yaw: p.Yaw, pitch: p.Pitch, dt: dt})
	a.Predict(p, in, dt)
}

// Reconcile is called just after a snapshot has put the player where the
// host had it, as of input ack: the inputs since are replayed on top, and
// the difference from where the player had been predicted is kept in
// Offset to ease out. before is where the player was before the snapshot.
func (pr *Predictor) Reconcile(a *arena.Arena, p *arena.Player, ack uint32, before mathx.Vec3) {
	i := 0
	for i < len(pr.history) && pr.history[i].seq <= ack {
		i++
	}
	pr.history = pr.history[i:]
	yaw, pitch := p.Yaw, p.Pitch
	for _, h := range pr.history {
		p.Yaw, p.Pitch = h.yaw, h.pitch // as it was aimed then
		a.Predict(p, h.in, h.dt)
	}
	p.Yaw, p.Pitch = yaw, pitch
	pr.Offset = pr.Offset.Add(before.Sub(p.Body.Position))
	if pr.Offset.Len() > snapFar || p.Dead {
		pr.Offset = mathx.Vec3{}
	}
}

// Ease shrinks the correction still to show.
func (pr *Predictor) Ease(dt float32) {
	pr.Offset = pr.Offset.Scale(float32(math.Exp(-easeRate * float64(dt))))
}

// Reset forgets everything (a new round).
func (pr *Predictor) Reset() { pr.history, pr.Offset = pr.history[:0], mathx.Vec3{} }

// Interp keeps the recent snapshots of the other players and places them
// smoothly between them, interpDelay behind the host.
type Interp struct {
	clock   float32 // our time
	offset  float32 // our time minus the host's, as best we can tell
	started bool
	samples [][]sample // by player
}

type sample struct {
	t          float32 // host time
	pos, vel   mathx.Vec3
	yaw, pitch float32
}

const (
	interpDelay    = 0.1  // s behind the newest snapshot other players are drawn
	maxExtrapolate = 0.15 // s past the newest snapshot they may carry on along their velocity
	maxSamples     = 32
)

// Tick advances our clock.
func (ip *Interp) Tick(dt float32) { ip.clock += dt }

// Add takes a snapshot's players.
func (ip *Interp) Add(s *arena.Snapshot) {
	off := ip.clock - s.Time
	switch {
	case !ip.started:
		ip.offset, ip.started = off, true
	case off < ip.offset:
		ip.offset = off // arrived sooner than we thought possible: the delay is less
	default:
		ip.offset += (off - ip.offset) * 0.02 // drift up slowly, riding out jitter
	}
	for len(ip.samples) < len(s.Players) {
		ip.samples = append(ip.samples, nil)
	}
	for i, np := range s.Players {
		list := ip.samples[i]
		if n := len(list); n > 0 && s.Time <= list[n-1].t {
			continue // out of order
		}
		if len(list) == maxSamples {
			list = list[1:]
		}
		ip.samples[i] = append(list, sample{t: s.Time, pos: mathx.Vec3(np.Pos), vel: mathx.Vec3(np.Vel), yaw: np.Yaw, pitch: np.Pitch})
	}
}

// Place puts player i where it should be drawn now. It reports false with
// nothing to go on yet.
func (ip *Interp) Place(i int, p *arena.Player) bool {
	if i >= len(ip.samples) || len(ip.samples[i]) == 0 {
		return false
	}
	list := ip.samples[i]
	t := ip.clock - ip.offset - interpDelay
	var s sample
	switch last := list[len(list)-1]; {
	case t >= last.t:
		s = last
		ahead := min(t-last.t, maxExtrapolate)
		s.pos = s.pos.Add(s.vel.Scale(ahead))
	case t <= list[0].t:
		s = list[0]
	default:
		k := len(list) - 1
		for list[k-1].t > t {
			k--
		}
		a, b := list[k-1], list[k]
		f := (t - a.t) / (b.t - a.t)
		s = sample{pos: a.pos.Add(b.pos.Sub(a.pos).Scale(f)), vel: a.vel.Add(b.vel.Sub(a.vel).Scale(f)),
			yaw:   a.yaw + float32(math.Remainder(float64(b.yaw-a.yaw), 2*math.Pi))*f,
			pitch: a.pitch + (b.pitch-a.pitch)*f}
	}
	p.Body.Position, p.Body.Velocity = s.pos, s.vel
	p.Body.Teleported()
	p.Yaw, p.Pitch = s.yaw, s.pitch
	return true
}

// ViewTime is the host's time everyone else is drawn at now (for lag
// compensation), or zero before any snapshot.
func (ip *Interp) ViewTime() float32 {
	if !ip.started {
		return 0
	}
	return max(ip.clock-ip.offset-interpDelay, 0)
}

// Reset forgets everything (a new round).
func (ip *Interp) Reset() { ip.samples, ip.started = nil, false } // (a new round's clock starts again)
