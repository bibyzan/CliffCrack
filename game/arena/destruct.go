package arena

import (
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
	Scrap // drone parts (debris only)
	materialCount
)

// MaterialInfo is how tough a material is and how its pieces behave.
type MaterialInfo struct {
	Name        string
	HP          float32 // per chunk
	Density     float32 // kg per m^3 of debris
	Restitution float32
	Friction    float32
	DebrisLife  float32 // seconds broken pieces stay before clearing away
}

var Materials = [materialCount]MaterialInfo{
	Wood:     {"wood", 45, 500, 0.3, 0.7, 10},
	Brick:    {"brick", 90, 1800, 0.15, 0.8, 12},
	Concrete: {"concrete", 160, 2300, 0.1, 0.8, 12},
	Glass:    {"glass", 4, 2500, 0.2, 0.3, 3},
	Metal:    {"metal", 600, 7800, 0.35, 0.5, 12},
	Scrap:    {"scrap", 1, 1200, 0.4, 0.5, 4},
}

// Chunk is one breakable piece of a structure: an axis-aligned box.
type Chunk struct {
	Structure  *Structure
	Index      int
	Centre     mathx.Vec3
	Half       mathx.Vec3
	Mat        Material
	HP, MaxHP  float32
	Alive      bool
	Body       *physics.Body // static collider while alive
	neighbours []int32       // indices of chunks it touches (in Structure.Chunks)
	anchored   bool          // rests on the ground
}

// Health is 0..1.
func (c *Chunk) Health() float32 { return max(c.HP, 0) / c.MaxHP }

// Structure is a building (or wall, or tower) made of chunks. Chunks hold up
// the chunks they touch; anything no longer connected to a grounded chunk
// collapses.
type Structure struct {
	Name    string
	Chunks  []*Chunk
	alive   int
	footing int  // chunks resting on the ground when built
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

// link computes which chunks touch (sharing some face area) and which rest
// on the ground (y = 0). Call once after building.
func (s *Structure) link() {
	for i, c := range s.Chunks {
		c.Structure, c.Index, c.Alive = s, i, true
		c.MaxHP = Materials[c.Mat].HP * max(volume(c.Half)/0.2, 0.35) // bigger pieces are tougher
		c.HP = c.MaxHP
		c.anchored = c.Centre[1]-c.Half[1] <= touchEps
		if c.anchored {
			s.footing++
		}
	}
	s.alive = len(s.Chunks)
	for i, a := range s.Chunks {
		for j := i + 1; j < len(s.Chunks); j++ {
			if touching(a, s.Chunks[j]) {
				a.neighbours = append(a.neighbours, int32(j))
				s.Chunks[j].neighbours = append(s.Chunks[j].neighbours, int32(i))
			}
		}
	}
}

func volume(h mathx.Vec3) float32 { return 8 * h[0] * h[1] * h[2] }

// touching reports whether two boxes share face area: they meet (within
// touchEps) along one axis and overlap by a margin on the other two.
func touching(a, b *Chunk) bool {
	meet := 0
	for k := 0; k < 3; k++ {
		overlap := min(a.Centre[k]+a.Half[k], b.Centre[k]+b.Half[k]) -
			max(a.Centre[k]-a.Half[k], b.Centre[k]-b.Half[k])
		switch {
		case overlap < -touchEps:
			return false // apart
		case overlap <= touchEps:
			meet++ // faces meet on this axis
		case overlap < 0.05:
			return false // only an edge or a sliver in common
		}
	}
	return meet == 1
}

// unsupported returns the standing chunks that must fall: those no longer
// connected, through other standing chunks, to one resting on the ground; or
// everything off the ground once too little of the footing is left to carry
// it.
func (s *Structure) unsupported() []*Chunk {
	seen := make([]bool, len(s.Chunks))
	var queue []int32
	footing := 0
	for i, c := range s.Chunks {
		if c.Alive && c.anchored {
			seen[i] = true
			queue = append(queue, int32(i))
			footing++
		}
	}
	if float32(footing) < minFooting*float32(s.footing) {
		queue = queue[:0] // overloaded: only the grounded stubs stay
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, j := range s.Chunks[i].neighbours {
			if !seen[j] && s.Chunks[j].Alive {
				seen[j] = true
				queue = append(queue, j)
			}
		}
	}
	var out []*Chunk
	for i, c := range s.Chunks {
		if c.Alive && !seen[i] {
			out = append(out, c)
		}
	}
	return out
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
