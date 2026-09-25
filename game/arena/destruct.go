package arena

import (
	"container/heap"
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Material is what a structure chunk is made of.
type Material int

const (
	Wood Material = iota
	Brick
	Concrete
	Glass
	Metal
	Panel // the arena's white clean-sim panels: the keep, bridges, ledges, stairs and parapets
	Plate // the arena's floor plates, laid over the girders
	MaterialCount
)

// MaterialInfo is how tough a material is, how far it can reach out from
// whatever holds it up, and how its pieces behave.
type MaterialInfo struct {
	Name        string
	HP          float32 // per chunk
	Density     float32 // kg per m^3 of debris
	Restitution float32
	Friction    float32
	DebrisLife  float32 // seconds broken pieces stay before clearing away
	// Span is how far (in metres, sideways) a piece can be from the nearest
	// thing bearing it up before it gives way: knock out what holds up a
	// floor and the part further out than this falls, the rest hangs on.
	Span float32
}

var Materials = [MaterialCount]MaterialInfo{
	Wood:     {"wood", 45, 500, 0.3, 0.7, 10, 4.5},
	Brick:    {"brick", 90, 1800, 0.15, 0.8, 12, 3.5},
	Concrete: {"concrete", 160, 2300, 0.1, 0.8, 12, 7},
	Glass:    {"glass", 4, 2500, 0.2, 0.3, 3, 2.5},
	Metal:    {"metal", 600, 7800, 0.35, 0.5, 12, 10},
	Panel:    {"panel", 110, 2000, 0.15, 0.8, 12, 4.5},
	Plate:    {"plate", 120, 2400, 0.1, 0.8, 12, 7},
}

// Chunk is one breakable piece of a structure: an axis-aligned box.
type Chunk struct {
	Structure *Structure
	Index     int
	Centre    mathx.Vec3
	Half      mathx.Vec3
	Mat       Material
	HP, MaxHP float32
	Alive     bool
	Trim      bool          // part of the arena's glowing outline, along its top edge
	Slope     *Slope        // if set, its collider instead of the box: a stair tread walked as a ramp
	Body      *physics.Body // static collider while alive

	links    []link  // the chunks it touches, in any structure
	anchored bool    // rests on the ground, or on something indestructible
	base     bool    // anchored, or resting on another structure: its footing
	reach    float32 // scratch for unsupported
}

// Slope is a tilted box standing in for a chunk in the physics.
type Slope struct {
	Centre, Half mathx.Vec3
	Rotation     mathx.Quat
}

// link is a chunk touching another. cost is how much of the other's span
// it takes to carry load across, side by side: the distance between them.
// One resting on top of the other (onTop) starts afresh: a column on a
// floor carries its load straight down, whatever the floor's own span.
// Nothing hangs from what's above it (under).
type link struct {
	c            *Chunk
	cost         float32
	onTop, under bool
}

// Health is 0..1.
func (c *Chunk) Health() float32 { return max(c.HP, 0) / c.MaxHP }

// Top is the height of the chunk's upper face.
func (c *Chunk) Top() float32 { return c.Centre[1] + c.Half[1] }

// Structure is a building (or wall, or floor, or tower) made of chunks.
// Chunks hold up the chunks they touch; anything too far from a chunk
// resting on the ground (or on something indestructible) collapses.
type Structure struct {
	Name    string
	Chunks  []*Chunk
	Shell   bool // part of the arena's fixed layout rather than random cover
	Spanned bool // held up by its spans alone (a floor on girders): no footing rule
	alive   int
	footing int  // chunks in its footing when built
	dirty   bool // chunks were removed: support needs rechecking
}

// minFooting is the fraction of its original footing a structure needs to
// stay up: knock out more of the ground floor than that and the rest comes
// down, even if a column or two still connects it to the ground.
const minFooting = 0.35

// Remaining is the fraction of the structure still standing.
func (s *Structure) Remaining() float32 {
	if len(s.Chunks) == 0 {
		return 0
	}
	return float32(s.alive) / float32(len(s.Chunks))
}

// Alive counts the standing chunks.
func (s *Structure) Alive() int { return s.alive }

// touchEps is how close faces must be to count as touching.
const touchEps = 0.02

// maxHPScale caps how much tougher than a wall panel a big piece gets, so a
// floor plate or a crate still breaks.
const maxHPScale = 2.5

// link sets up a structure on its own: its chunks' health, which touch, and
// which rest on the ground (y = 0). Call once after building. An Arena
// relinks every structure together (see Arena.linkAll).
func (s *Structure) link() {
	for i, c := range s.Chunks {
		c.Structure, c.Index, c.Alive = s, i, true
		c.MaxHP = Materials[c.Mat].HP * clamp(volume(c.Half)/0.2, 0.35, maxHPScale) // bigger pieces are tougher
		c.HP = c.MaxHP
		c.links = c.links[:0]
		c.anchored = c.Centre[1]-c.Half[1] <= touchEps
		c.base = c.anchored
	}
	s.alive = len(s.Chunks)
	for i, a := range s.Chunks {
		for _, b := range s.Chunks[i+1:] {
			connect(a, b)
		}
	}
	s.countFooting()
}

func (s *Structure) countFooting() {
	s.footing = 0
	for _, c := range s.Chunks {
		if c.base {
			s.footing++
		}
	}
}

// connect links a and b if they touch.
func connect(a, b *Chunk) {
	axis, ok := contact(a, b)
	if !ok {
		return
	}
	cost := float32(0)
	if axis != 1 {
		cost = flat(a.Centre.Sub(b.Centre)).Len()
	}
	aboveB := axis == 1 && a.Centre[1] > b.Centre[1]
	aboveA := axis == 1 && !aboveB
	a.links = append(a.links, link{b, cost, aboveA, aboveB})
	b.links = append(b.links, link{a, cost, aboveB, aboveA})
}

func volume(h mathx.Vec3) float32 { return 8 * h[0] * h[1] * h[2] }

// touching reports whether two boxes share face area.
func touching(a, b *Chunk) bool {
	_, ok := contact(a, b)
	return ok
}

// contact reports whether two boxes share face area: they meet (within
// touchEps) along one axis, returned, and overlap by a margin on the other
// two.
func contact(a, b *Chunk) (axis int, ok bool) {
	meet := 0
	for k := 0; k < 3; k++ {
		overlap := min(a.Centre[k]+a.Half[k], b.Centre[k]+b.Half[k]) -
			max(a.Centre[k]-a.Half[k], b.Centre[k]-b.Half[k])
		switch {
		case overlap < -touchEps:
			return 0, false // apart
		case overlap <= touchEps:
			meet++ // faces meet on this axis
			axis = k
		case overlap < 0.05:
			return 0, false // only an edge or a sliver in common
		}
	}
	return axis, meet == 1
}

// restsOn reports whether chunk c sits on top of the static box b.
func restsOn(c *Chunk, b Block) bool {
	if r := b.Rotation; abs(r.X)+abs(r.Y)+abs(r.Z) > 1e-4 {
		return false // a tilted block: a brace, a stair's slope
	}
	if abs(c.Centre[1]-c.Half[1]-(b.Centre[1]+b.Half[1])) > touchEps {
		return false
	}
	for _, k := range []int{0, 2} {
		overlap := min(c.Centre[k]+c.Half[k], b.Centre[k]+b.Half[k]) - max(c.Centre[k]-c.Half[k], b.Centre[k]-b.Half[k])
		if overlap < 0.05 {
			return false
		}
	}
	return true
}

// linkAll links every chunk of every structure to every chunk it touches,
// so a wall standing on a floor is held up by the floor (and comes down with
// it). Chunks resting on the level's blocks are anchored.
func linkAll(structures []*Structure, level []Block) []*Chunk {
	var all []*Chunk
	for _, s := range structures {
		for _, c := range s.Chunks {
			c.links = c.links[:0]
			c.anchored = false
			for _, b := range level {
				if restsOn(c, b) {
					c.anchored = true
					break
				}
			}
			c.base = c.anchored
			all = append(all, c)
		}
	}
	// A spatial hash finds the neighbours without testing every pair.
	const cell = 2.0
	key := func(v float32) int32 { return int32(math.Floor(float64(v / cell))) }
	grid := map[[3]int32][]int32{}
	span := func(c *Chunk, f func(k [3]int32)) {
		lo := c.Centre.Sub(c.Half).Sub(mathx.Vec3{touchEps, touchEps, touchEps})
		hi := c.Centre.Add(c.Half).Add(mathx.Vec3{touchEps, touchEps, touchEps})
		for x := key(lo[0]); x <= key(hi[0]); x++ {
			for y := key(lo[1]); y <= key(hi[1]); y++ {
				for z := key(lo[2]); z <= key(hi[2]); z++ {
					f([3]int32{x, y, z})
				}
			}
		}
	}
	for i, c := range all {
		span(c, func(k [3]int32) { grid[k] = append(grid[k], int32(i)) })
	}
	seen := make([]int32, len(all))
	for i := range seen {
		seen[i] = -1
	}
	for i, a := range all {
		span(a, func(k [3]int32) {
			for _, j := range grid[k] {
				if int(j) <= i || seen[j] == int32(i) {
					continue
				}
				seen[j] = int32(i)
				b := all[j]
				connect(a, b)
				if b.Structure != a.Structure {
					if _, below := contactBelow(a, b); below {
						a.base = true
					}
					if _, below := contactBelow(b, a); below {
						b.base = true
					}
				}
			}
		})
	}
	for _, s := range structures {
		s.countFooting()
	}
	return all
}

// contactBelow reports whether a rests on top of b.
func contactBelow(a, b *Chunk) (int, bool) {
	axis, ok := contact(a, b)
	return axis, ok && axis == 1 && a.Centre[1] > b.Centre[1]
}

// unsupported returns the standing chunks among chunks that must fall. Load
// spreads out from the anchored chunks: up into anything resting on them
// (which then spans afresh) and sideways at the cost of the distance
// covered, but never down: nothing hangs from what it holds up. A chunk further (by that measure) from an anchor than its
// material can span falls. So does everything above the footing of a
// structure that has lost too much of it.
func unsupported(chunks []*Chunk) []*Chunk {
	counts := map[*Structure]int{}
	for _, c := range chunks {
		if c.Alive && c.base {
			counts[c.Structure]++
		}
	}
	overloaded := func(s *Structure) bool {
		return !s.Spanned && s.footing > 0 && float32(counts[s]) < minFooting*float32(s.footing)
	}
	heavy := map[*Structure]bool{}
	for _, c := range chunks {
		if _, done := heavy[c.Structure]; !done {
			heavy[c.Structure] = overloaded(c.Structure)
		}
	}

	inf := float32(math.Inf(1))
	var q reachQueue
	for _, c := range chunks {
		c.reach = inf
		if c.Alive && c.anchored {
			c.reach = 0
			heap.Push(&q, c)
		}
	}
	for q.Len() > 0 {
		c := heap.Pop(&q).(*Chunk)
		for _, l := range c.links {
			n := l.c
			if !n.Alive || l.under || (heavy[n.Structure] && !n.base) {
				continue
			}
			d := c.reach + l.cost
			if l.onTop {
				d = 0
			}
			if d > Materials[n.Mat].Span || d >= n.reach {
				continue
			}
			n.reach = d
			heap.Push(&q, n)
		}
	}
	var out []*Chunk
	for _, c := range chunks {
		if c.Alive && c.reach == inf {
			out = append(out, c)
		}
	}
	return out
}

// unsupported is the structure's own chunks that must fall, judging it on
// its own.
func (s *Structure) unsupported() []*Chunk { return unsupported(s.Chunks) }

// reachQueue is a min-heap of chunks by reach. A chunk may be pushed more
// than once as its reach improves; stale entries pop harmlessly later.
type reachQueue []*Chunk

func (q reachQueue) Len() int           { return len(q) }
func (q reachQueue) Less(i, j int) bool { return q[i].reach < q[j].reach }
func (q reachQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *reachQueue) Push(x any)        { *q = append(*q, x.(*Chunk)) }
func (q *reachQueue) Pop() any {
	old := *q
	c := old[len(old)-1]
	*q = old[:len(old)-1]
	return c
}

// distToBox is the distance from p to the chunk's box (0 inside).
func (c *Chunk) distTo(p mathx.Vec3) float32 {
	var d mathx.Vec3
	for k := 0; k < 3; k++ {
		d[k] = max(abs(p[k]-c.Centre[k])-c.Half[k], 0)
	}
	return d.Len()
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
