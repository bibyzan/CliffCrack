package course

import (
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// PowerKind is what a power-up does when the ball rolls through it.
type PowerKind int

const (
	// Boost: a burst of speed, then a few seconds of a much higher cruise.
	Boost PowerKind = iota
	// Shield: for a few seconds, rocks and trees shatter instead of ending the run.
	Shield
)

// PowerUp is a pickup floating over the path.
type PowerUp struct {
	Kind     PowerKind
	Pos      mathx.Vec3 // its centre, floating over the snow
	Distance float32    // s at the pickup
}

const (
	powerFrom   = StartClear // no pickups on the Drop: it's fast enough
	powerChance = 0.45       // of a chunk having one
	powerFloat  = 1.1        // metres its centre floats above the snow
	powerClear  = 3.0        // metres kept between it and any obstacle
)

// PowerUps returns chunk index's power-ups: about one every 100 m, on the
// path, clear of obstacles, kickers and landings. Shields turn up more often
// in the special sections, where there's more to hit.
func (c *Course) PowerUps(index int) []PowerUp {
	start := float32(index) * ChunkLength
	if start < powerFrom {
		return nil
	}
	rng := rand.New(rand.NewPCG(c.Seed^0x90e7, uint64(int64(index))))
	if rng.Float32() > powerChance {
		return nil
	}
	obstacles := c.Obstacles(index)
	for range 8 { // a few tries for a clear spot
		s := start + 4 + rng.Float32()*(ChunkLength-8)
		if c.inJumpZone(s) {
			continue
		}
		p, inSection := c.profileAt(s)
		if inSection && p.corridor < 0.99 {
			continue // a section's path is still joining the valley
		}
		hw := c.PathHalfWidth(s)
		x, z := c.PathCentre(s)+(rng.Float32()*2-1)*max(hw-2.5, 0), -s
		pos := mathx.Vec3{x, c.Height(x, z) + powerFloat, z}
		clear := true
		for _, o := range obstacles {
			if !o.Scenery && o.Centre.Sub(pos).Len() < o.Radius+powerClear {
				clear = false
				break
			}
		}
		if !clear {
			continue
		}
		kind := Boost
		shield := float32(0.3)
		if inSection {
			shield = 0.6
		}
		if rng.Float32() < shield {
			kind = Shield
		}
		return []PowerUp{{Kind: kind, Pos: pos, Distance: s}}
	}
	return nil
}
