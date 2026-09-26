package arena

import (
	"container/heap"
	"math"
	"sort"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Structural failure, in two parts.
//
// Stress: every standing chunk carries a load, its own weight and whatever
// it holds up, passed down (and sideways) through the chunks that hold it
// up to the ground. Each remembers the load it had when the site was built:
// its design load. Knock out supports and their load moves onto the ones
// left; a chunk carrying more than stressSafety times its design load is
// overstressed, and it creaks and slowly gives way (faster the worse it is)
// until it breaks, passing its load on in turn. So a building with two of
// its four corners gone may stand a few seconds, groaning, then fall.
//
// Toppling: a big piece of a structure left with nothing under it (a tower
// with its base shot out) doesn't crumble where it stands. It falls as one:
// dropping onto whatever's below, tipping over (slowly, then faster) away
// from where it was broken, sweeping players aside, and shattering where it
// lands.

const (
	stressSafety = 1.6  // a chunk holds up to this many times its design load
	stressRate   = 0.45 // of its max HP per s, per unit of overload past that
	stressChip   = 0.25 // s between a straining chunk's reports (its creaks)

	toppleMinChunks = 4    // pieces this small just crumble
	toppleMinHeight = 2.5  // m tall, at least, to fall over rather than crumble
	toppleMaxAngle  = 1.5  // rad: past this it's down, and shatters
	toppleStartSpin = 0.18 // rad/s it starts tipping at
	topplePush      = 1.1  // a player it sweeps is thrown this much faster than it moves
)

// mass is a chunk's weight, in kg.
func (c *Chunk) mass() float32 { return Materials[c.Mat].Density * volume(c.Half) }

// Strain is how overstressed the chunk is: its load over what it can take
// (stressSafety times its design load). Over 1 it's giving way.
func (c *Chunk) Strain() float32 {
	if c.designLoad <= 0 {
		return 0
	}
	return c.load / (c.designLoad * stressSafety)
}

// computeLoads works out every standing chunk's load. With design set it
// also takes those as the design loads (once, when the site is built).
//
// The support is found as in unsupported (from the anchors outwards: up
// into what rests on a chunk, sideways within its material's span, never
// down), settling chunks in order of reach and then height. A chunk's
// supporters are its neighbours settled before it that could reach it; its
// load (its weight plus what it was handed) is shared out among them evenly,
// farthest chunks first, down to the anchors.
func computeLoads(chunks []*Chunk, design bool) {
	for _, c := range chunks {
		c.reach, c.load, c.order = float32(math.Inf(1)), 0, -1
	}
	var q loadQueue
	for _, c := range chunks {
		if c.Alive && c.anchored {
			c.reach = 0
			heap.Push(&q, c)
		}
	}
	var settled []*Chunk
	for q.Len() > 0 {
		c := heap.Pop(&q).(*Chunk)
		if c.order >= 0 {
			continue // an older, worse entry
		}
		c.order = len(settled)
		settled = append(settled, c)
		for _, l := range c.links {
			n := l.c
			if !n.Alive || l.under || n.order >= 0 {
				continue
			}
			d := c.reach + l.cost
			if l.onTop {
				d = 0
			}
			if d > Materials[n.Mat].Span || d > n.reach {
				continue
			}
			n.reach = d
			heap.Push(&q, n)
		}
	}
	// Loads, from the last settled (farthest out, highest) back to the anchors.
	for i := len(settled) - 1; i >= 0; i-- {
		c := settled[i]
		c.load += c.mass()
		var holders []*Chunk
		for _, l := range c.links {
			// n holds c up if c could be reached from n: n settled first,
			// and the link from n's side isn't "under" (n isn't above c).
			n := l.c
			if !n.Alive || n.order < 0 || n.order >= c.order || !l.under && !sideways(l) {
				continue
			}
			holders = append(holders, n)
		}
		if len(holders) == 0 {
			continue // an anchor: it goes into the ground
		}
		share := c.load / float32(len(holders))
		for _, n := range holders {
			n.load += share
		}
	}
	if design {
		for _, c := range chunks {
			c.designLoad = max(c.load, c.mass())
		}
	}
}

// sideways reports whether a link is side by side (neither resting on the
// other).
func sideways(l link) bool { return !l.onTop && !l.under }

// strain wears down overstressed chunks (after computeLoads).
func (a *Arena) strain(dt float32, ev *Events) {
	for _, c := range a.chunks {
		if !c.Alive {
			continue
		}
		over := c.Strain() - 1
		if over <= 0 {
			continue
		}
		c.HP -= c.MaxHP * stressRate * min(over, 3) * dt
		if c.HP <= 0 {
			a.breakChunk(c, mathx.Vec3{0, -1, 0}, a.lastBreaker, ev)
			continue
		}
		ev.Chipped = append(ev.Chipped, c)     // (online, its HP goes out)
		if a.Time-c.strainedAt >= stressChip { // it creaks
			c.strainedAt = a.Time
			ev.Strained = append(ev.Strained, c)
		}
	}
}

// Topple is a piece of a structure falling over as one: it drops onto what
// is below it, then turns about Pivot's axis (dropped by Drop) by Angle,
// its chunks as they were. It shatters where it lands.
type Topple struct {
	ID          int
	Chunks      []*Chunk
	Pivot, Axis mathx.Vec3
	Support     float32 // the height it drops to before it tips
	Drop        float32 // how far it has dropped
	DropVel     float32
	Angle, Spin float32
	lever       float32             // pivot to its centre of mass
	far         float32             // pivot to its farthest piece: only pieces well out from the pivot can land it
	own         map[*Structure]bool // the structures it's part of
	By          *Player
	Down        bool // shattered
}

// Frame is the transform from where its chunks stood to where they are.
func (t *Topple) Frame() mathx.Mat4 {
	p := t.Pivot
	return mathx.Translate(p[0], p[1]-t.Drop, p[2]).Mul(mathx.AxisAngle(t.Axis, t.Angle).Mat4()).
		Mul(mathx.Translate(-p[0], -p[1], -p[2]))
}

// velocityAt is how fast a point (where it stood) is moving now.
func (t *Topple) velocityAt(p mathx.Vec3) mathx.Vec3 {
	f := t.Frame()
	w := f.TransformPoint(p)
	pivot := t.Pivot.Sub(mathx.Vec3{0, t.Drop, 0})
	v := t.Axis.Scale(t.Spin).Cross(w.Sub(pivot))
	if t.Drop < t.Pivot[1]-t.Support {
		v[1] -= t.DropVel
	}
	return v
}

// collapse brings down what's lost its support (from settle): each
// connected piece of it big enough topples, the rest crumbles.
func (a *Arena) collapse(falling []*Chunk, ev *Events) {
	in := map[*Chunk]bool{}
	for _, c := range falling {
		in[c] = true
	}
	done := map[*Chunk]bool{}
	for _, c := range falling {
		if done[c] {
			continue
		}
		// The connected piece c is part of.
		group := []*Chunk{c}
		done[c] = true
		for i := 0; i < len(group); i++ {
			for _, l := range group[i].links {
				if n := l.c; in[n] && !done[n] {
					done[n] = true
					group = append(group, n)
				}
			}
		}
		lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
		for _, g := range group {
			lo, hi = min(lo, g.Centre[1]-g.Half[1]), max(hi, g.Centre[1]+g.Half[1])
		}
		if len(group) >= toppleMinChunks && hi-lo >= toppleMinHeight {
			a.topple(group, lo, ev)
			continue
		}
		for _, g := range group {
			a.crumble(g, ev)
		}
	}
}

// crumble drops a chunk out of where it stood as one piece of rubble.
func (a *Arena) crumble(c *Chunk, ev *Events) {
	a.removeChunk(c, nil)
	drift := mathx.Vec3{a.rng.Float32() - 0.5, -0.5, a.rng.Float32() - 0.5}.Scale(0.8)
	d := a.addDebris(c.Centre, c.Half, c.Mat, drift)
	d.By = a.lastBreaker
	d.Collapsed = true
	ev.Breaks = append(ev.Breaks, Break{At: c.Centre, Half: c.Half, Mat: c.Mat, Collapsed: true, Chunk: c})
}

// topple starts a piece (bottom at lo) falling over, away from its middle
// towards where it was last broken.
func (a *Arena) topple(group []*Chunk, lo float32, ev *Events) {
	sort.Slice(group, func(i, j int) bool { return group[i].ID < group[j].ID }) // the same order everywhere
	var centroid, com mathx.Vec3
	var base, mass float32
	for _, c := range group {
		m := c.mass()
		com = com.Add(c.Centre.Scale(m))
		mass += m
		if c.Centre[1]-c.Half[1] < lo+0.3 {
			centroid = centroid.Add(c.Centre)
			base++
		}
	}
	com = com.Scale(1 / mass)
	centroid = centroid.Scale(1 / base)
	dir := a.lastBreakAt.Sub(centroid)
	dir[1] = 0
	if dir.Len() < 0.3 {
		dir = com.Sub(centroid)
		dir[1] = 0
	}
	if dir.Len() < 0.05 {
		ang := a.rng.Float64() * 2 * math.Pi
		dir = mathx.Vec3{float32(math.Cos(ang)), 0, float32(math.Sin(ang))}
	}
	dir = dir.Normalize()
	// The pivot: the base's edge on that side, where it stood.
	edge := float32(0)
	for _, c := range group {
		if c.Centre[1]-c.Half[1] < lo+0.3 {
			off := c.Centre.Sub(centroid).Dot(dir) + abs(c.Half[0]*dir[0]) + abs(c.Half[2]*dir[2])
			edge = max(edge, off)
		}
	}
	pivot := centroid.Add(dir.Scale(edge))
	pivot[1] = lo
	t := &Topple{ID: a.nextTopple, Chunks: group, Pivot: pivot, Axis: mathx.Vec3{0, 1, 0}.Cross(dir).Normalize(),
		Spin: toppleStartSpin, By: a.lastBreaker, lever: max(com.Sub(pivot).Len(), 0.5)}
	a.nextTopple++
	t.own = map[*Structure]bool{}
	for _, c := range group {
		a.removeChunk(c, nil)
		t.far = max(t.far, c.Centre.Sub(pivot).Len())
		t.own[c.Structure] = true
	}
	t.Support = a.floorBelow(pivot)
	a.Topples = append(a.Topples, t)
	ev.Topples = append(ev.Topples, t)
}

// floorBelow is the height of what's under p (it can be far down: the pit).
func (a *Arena) floorBelow(p mathx.Vec3) float32 {
	if hit, ok := a.Phys.Raycast(p.Add(mathx.Vec3{0, 0.05, 0}), mathx.Vec3{0, -1, 0}, 60, a.ignoreForAim); ok {
		return hit.Point[1]
	}
	return pitDepth
}

// stepTopples moves the falling pieces on, sweeping players (the host only:
// push) and shattering them where they land.
func (a *Arena) stepTopples(dt float32, push bool, ev *Events) {
	kept := a.Topples[:0]
	for _, t := range a.Topples {
		before := t.Frame()
		if t.Drop < t.Pivot[1]-t.Support {
			t.DropVel += gravity * dt
			t.Drop = min(t.Drop+t.DropVel*dt, t.Pivot[1]-t.Support)
			if t.Drop >= t.Pivot[1]-t.Support {
				t.Spin += t.DropVel * 0.06 // landing on its edge kicks it over
				t.DropVel = 0
			}
		} else {
			// Tipping about its edge: an inverted pendulum, slowly and then fast.
			t.Spin += 0.75 * gravity / t.lever * float32(math.Sin(float64(max(t.Angle, 0.05)))) * dt
		}
		t.Angle += t.Spin * dt
		after := t.Frame()
		if push {
			a.sweep(t, after)
		}
		if t.Pivot[1]-t.Drop < fallDeath-10 {
			t.Down = true // gone into the pit, out of sight
			continue
		}
		if t.Angle > toppleMaxAngle || (t.Angle > 0.15 && a.toppleHits(t, before, after)) {
			a.shatter(t, ev)
			continue
		}
		kept = append(kept, t)
	}
	clear(a.Topples[len(kept):])
	a.Topples = kept
}

// toppleHits reports whether any of a topple's pieces ran into the level or
// a standing structure this step.
func (a *Arena) toppleHits(t *Topple, before, after mathx.Mat4) bool {
	for _, c := range t.Chunks {
		if c.Centre.Sub(t.Pivot).Len() < 0.4*t.far {
			continue // near the pivot: it grinds against its base as it turns, but that's not landing
		}
		from, to := before.TransformPoint(c.Centre), after.TransformPoint(c.Centre)
		step := to.Sub(from)
		dist := step.Len()
		if dist < 1e-4 {
			continue
		}
		reach := dist + min(c.Half[0], c.Half[1], c.Half[2])
		passes := func(b *physics.Body) bool {
			// It passes through what's left of its own structures (bits
			// anchored elsewhere: its rubble will crush them).
			if c, ok := b.UserData.(*Chunk); ok && t.own[c.Structure] {
				return true
			}
			return a.ignoreForAim(b)
		}
		if _, ok := a.Phys.Raycast(from, step.Scale(1/dist), reach, passes); ok {
			return true
		}
	}
	return false
}

// sweep throws players out of a topple's way, with how fast it's moving
// where it meets them (no damage, but it can knock them off an edge: the
// fall is credited to whoever brought it down).
func (a *Arena) sweep(t *Topple, frame mathx.Mat4) {
	back := mathx.AxisAngle(t.Axis, -t.Angle).Mat4()
	pivot := t.Pivot.Sub(mathx.Vec3{0, t.Drop, 0})
	for _, p := range a.Players {
		if p.Dead {
			continue
		}
		// The player's point in where the chunks stood.
		local := back.TransformPoint(p.Body.Position.Sub(pivot)).Add(t.Pivot)
		for _, c := range t.Chunks {
			if c.distTo(local) > PlayerRadius+0.1 {
				continue
			}
			v := t.velocityAt(c.Centre).Scale(topplePush)
			v[1] = max(v[1], 1.5)
			if v.Len() > p.Body.Velocity.Len() {
				p.Body.Velocity = v
			}
			if t.By != nil && t.By != p {
				p.lastHitBy, p.lastHitAt = t.By, a.Time
			}
			break
		}
		_ = frame
	}
}

// shatter breaks a topple into rubble where it is, each piece flying on
// with the speed it had.
func (a *Arena) shatter(t *Topple, ev *Events) {
	t.Down = true
	f := t.Frame()
	for _, c := range t.Chunks {
		at := f.TransformPoint(c.Centre)
		v := t.velocityAt(c.Centre)
		n := 3
		big := c.Half.Scale(0.55)
		for i := range n {
			half := big
			if i > 0 {
				half = big.Scale(0.6 + 0.3*a.rng.Float32())
			}
			spray := mathx.Vec3{a.rng.Float32()*2 - 1, a.rng.Float32(), a.rng.Float32()*2 - 1}.Scale(1.5)
			d := a.addDebris(at.Add(spray.Scale(0.15)), half, c.Mat, v.Scale(0.7).Add(spray))
			d.By = t.By
		}
		ev.Breaks = append(ev.Breaks, Break{At: at, Half: c.Half, Mat: c.Mat, Chunk: c, Push: v, Toppled: true})
	}
}

// loadQueue is a min-heap of chunks by reach, then height (so a stack of
// pieces at the same reach settles bottom first).
type loadQueue []*Chunk

func (q loadQueue) Len() int { return len(q) }
func (q loadQueue) Less(i, j int) bool {
	if q[i].reach != q[j].reach {
		return q[i].reach < q[j].reach
	}
	return q[i].Centre[1] < q[j].Centre[1]
}
func (q loadQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *loadQueue) Push(x any)   { *q = append(*q, x.(*Chunk)) }
func (q *loadQueue) Pop() any {
	old := *q
	c := old[len(old)-1]
	*q = old[:len(old)-1]
	return c
}

// Demolish knocks out the bottom layer of every structure named name (as a
// player might, shooting out its base), for testing. It reports how many
// pieces it broke.
func (a *Arena) Demolish(name string, ev *Events) int {
	lo := float32(math.Inf(1))
	for _, s := range a.Structures {
		if s.Name == name {
			for _, c := range s.Chunks {
				lo = min(lo, c.Centre[1])
			}
		}
	}
	n := 0
	for _, s := range a.Structures {
		if s.Name != name {
			continue
		}
		for _, c := range s.Chunks {
			if c.Alive && c.Centre[1] < lo+0.1 {
				a.breakChunk(c, mathx.Vec3{}, a.lastBreaker, ev)
				n++
			}
		}
	}
	return n
}
