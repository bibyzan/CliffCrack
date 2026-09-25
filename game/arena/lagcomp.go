package arena

import "CliffCrack/engine/mathx"

// Lag compensation. A guest online sees everyone else a little in the past
// (the host's world, a trip over the network ago, drawn smoothly a touch
// further back still). So the host judges a guest's shots against the world
// the guest saw: each input says which moment of the host's time the guest
// was looking at (Input.ViewTime), and while that player's weapons act, the
// other players are put back where they were then. Only players move back;
// the site is as it is now.

const (
	historyLen = 36   // steps of player positions kept (0.6 s at 60 Hz)
	maxRewind  = 0.35 // s: no further back than this, however laggy the shooter
)

// pastFrame is where everyone was at a moment.
type pastFrame struct {
	t   float32
	pos []mathx.Vec3
}

// recordPast notes where everyone is now (called at the end of each step).
func (a *Arena) recordPast() {
	f := pastFrame{t: a.Time, pos: make([]mathx.Vec3, len(a.Players))}
	for i, p := range a.Players {
		f.pos[i] = p.Body.Position
	}
	if len(a.past) == historyLen {
		copy(a.past, a.past[1:])
		a.past = a.past[:historyLen-1]
	}
	a.past = append(a.past, f)
}

// rewind puts every player but shooter where they were at time t (as far
// back as maxRewind, between recorded steps), and returns what undoes it.
// With t zero, or nothing recorded, it does nothing.
func (a *Arena) rewind(shooter *Player, t float32) (restore func()) {
	restore = func() {}
	if t <= 0 || len(a.past) == 0 {
		return
	}
	t = max(t, a.Time-maxRewind)
	if t >= a.past[len(a.past)-1].t {
		return // (the present)
	}
	k := 0
	for k < len(a.past)-1 && a.past[k+1].t <= t {
		k++
	}
	at := func(i int) mathx.Vec3 {
		f := a.past[k]
		if k+1 >= len(a.past) || i >= len(f.pos) {
			return f.pos[min(i, len(f.pos)-1)]
		}
		g := a.past[k+1]
		if i >= len(g.pos) || g.t <= f.t {
			return f.pos[i]
		}
		u := clamp((t-f.t)/(g.t-f.t), 0, 1)
		return f.pos[i].Add(g.pos[i].Sub(f.pos[i]).Scale(u))
	}
	saved := make([]mathx.Vec3, len(a.Players))
	for i, p := range a.Players {
		saved[i] = p.Body.Position
		if p != shooter && !p.Dead && i < len(a.past[k].pos) {
			p.Body.Position = at(i)
		}
	}
	return func() {
		for i, p := range a.Players {
			p.Body.Position = saved[i]
		}
	}
}
