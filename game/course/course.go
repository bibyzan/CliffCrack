// Package course generates the Run mode's downhill level from a seed (pure
// Go, no GPU): a meandering snow channel dropping down a mountainside, walled
// by rising peaks, cut by cracks to jump and strewn with rocks and pines, and
// broken up by special sections (sections.go): ridges above a bottomless pit
// and narrows between sheer walls.
//
// The level is endless and generated in chunks along the run. Every query is
// a pure function of the seed, so chunks can be built in any order, the
// renderer and the physics sample the same surface, and a seed replays the
// same level.
//
// Coordinates: the run heads down -Z; s = -z is the distance from the start
// in metres. x is across the run and y is up.
package course

import (
	"math"
	"math/rand/v2"
	"sort"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/noise"
)

const (
	// The slope steepens from startGrade to endGrade (drop per metre travelled,
	// about 21 to 33 degrees) over the first steepenOver metres, so the ride
	// keeps getting faster.
	startGrade  = 0.38
	endGrade    = 0.65
	steepenOver = 3000
	// The Drop: off the cliff the slope starts dropGrade steeper (about 47
	// degrees in all), easing into the run over dropLength metres, to get the
	// ball going fast from the first second.
	dropGrade  = 0.7
	dropLength = 160

	// ChunkLength is the length of one generated piece of the level (metres).
	ChunkLength = 48
	// rowStep is the spacing of mesh rows along the run. Cracks are aligned
	// to rows, so the mesh reproduces their sheer walls exactly.
	rowStep = 1
	// Mesh columns span the course centre +-halfSpan, packed tightly in the
	// channel and spreading out over the mountains.
	columns  = 73
	halfSpan = 130

	// StartClear is how far down the run the first obstacles may appear.
	StartClear = 70
	// cliffHeight is the wall the ball is dropped from, behind the start.
	cliffHeight = 30

	crackDepth = 30
	landing    = 45 // metres kept clear after a crack: you can't steer much in the air
	rampLength = 8  // metres of kicker before each crack
	// The kicker lip rises this far above the slope, leaving it at ~10 degrees
	// to the surface: enough for a slow ball to clear a wide crack, gentle
	// enough that a fast one doesn't sail off down the mountain for seconds.
	rampHeight = 0.9
)

// Crack is a crevasse across the run between S and S+Width.
type Crack struct {
	S, Width float32
}

// ObstacleKind is what stands on the course.
type ObstacleKind int

const (
	Rock ObstacleKind = iota
	Tree
)

// Obstacle is one thing to avoid. The run ends when the ball hits one.
type Obstacle struct {
	Kind     ObstacleKind
	Base     mathx.Vec3 // where it meets the ground
	Radius   float32    // collision sphere radius
	Centre   mathx.Vec3 // collision sphere centre
	Scale    float32    // visual size multiplier
	Yaw      float32    // visual rotation about +Y
	Variant  int        // which mesh variant to draw (any non-negative number)
	Squash   float32    // rocks: vertical scale factor
	Distance float32    // s at the obstacle
}

// Course is one generated level.
type Course struct {
	Seed uint64

	shape  *noise.Field // centre line, width, rolls
	detail *noise.Field // moguls
	peaks  *noise.Field // mountains beside the run
	cracks []Crack      // sorted by S
	// Special sections, sorted by Start. Cracks only appear in plain valley.
	sections []Section
}

// New generates the level for seed.
func New(seed uint64) *Course {
	c := &Course{
		Seed:   seed,
		shape:  noise.New(seed),
		detail: noise.New(seed + 1),
		peaks:  noise.New(seed + 2),
	}
	c.generateSections()
	// Cracks: the first after a warm-up stretch, then closer together and
	// wider the further you get, only in plain valley. Aligned to mesh rows.
	rng := rand.New(rand.NewPCG(seed, 0xc4ac5))
	for s := float32(220 + rng.IntN(60)); s < 200000; {
		d := Difficulty(s)
		width := float32(math.Round(float64(3 + 5*d + float32(rng.IntN(2)))))
		at := float32(math.Round(float64(s)))
		if c.overlapsSection(at-rampLength-30, at+width+landing+30) {
			// No cracks in sections: try again just after the one in the way.
			k, ok := c.SectionAt(at + width + landing + 30)
			if !ok {
				k, _ = c.NextSection(at - rampLength - 30)
			}
			s = k.End() + rampLength + 40 + float32(rng.IntN(60))
			continue
		}
		c.cracks = append(c.cracks, Crack{S: at, Width: width})
		s += 130 + 130*(1-d) + float32(rng.IntN(100))
	}
	return c
}

// Difficulty rises from 0 at the start towards 1 and never stops rising:
// about 0.46 at 1 km, 0.71 at 2 km, 0.85 at 3 km.
func Difficulty(s float32) float32 {
	return float32(1 - math.Exp(-float64(max(s, 0))/1600))
}

// GradeAt is the slope's average drop per metre at s.
func GradeAt(s float32) float32 {
	g := startGrade + (endGrade-startGrade)*clamp(s/steepenOver, 0, 1)
	if s > 0 && s < dropLength {
		g += dropGrade * (1 - s/dropLength)
	}
	return g
}

// drop is how far the slope has fallen by s: the integral of GradeAt.
func drop(s float32) float32 {
	const g0, g1, l = startGrade, endGrade, steepenOver
	var d float32
	switch {
	case s <= 0:
		return g0 * s
	case s < l:
		d = g0*s + (g1-g0)*s*s/(2*l)
	default:
		d = g0*l + (g1-g0)*l/2 + g1*(s-l)
	}
	t := min(s, dropLength)
	return d + dropGrade*(t-t*t/(2*dropLength))
}

// Centre is the x of the valley's centre line at distance s. The path follows
// it except in ridge sections (see PathCentre).
func (c *Course) Centre(s float32) float32 {
	fs := float64(s)
	x := 34*c.shape.Line(fs/260) + 9*c.shape.Line(fs/75+40)
	// Ease in from a straight start so the drop lands on a predictable line.
	return float32(x) * smoothstep(0, 120, s)
}

// HalfWidth is half the width of the valley channel's flat-ish floor at s.
// It tightens as you go. (Where the path runs elsewhere, see PathHalfWidth.)
func (c *Course) HalfWidth(s float32) float32 { return c.valleyHalfWidth(s) }

func (c *Course) valleyHalfWidth(s float32) float32 {
	d := Difficulty(s)
	return 15 - 5*d + (4-1.5*d)*float32(c.shape.Line(float64(s)/140+90))
}

// PathCentre is the x of the rideable path's centre line: the valley's, or
// the ridge's crest in a ridge section.
func (c *Course) PathCentre(s float32) float32 {
	p, _ := c.profileAt(s)
	return c.Centre(s) + p.shift
}

// PathHalfWidth is half the width of the rideable path's floor at s.
func (c *Course) PathHalfWidth(s float32) float32 {
	w := c.valleyHalfWidth(s)
	if p, ok := c.profileAt(s); ok {
		return w + (p.halfWidth-w)*p.corridor
	}
	return w
}

// ValleyWeight is 1 in plain valley and 0 where a section's corridor has
// fully replaced it.
func (c *Course) ValleyWeight(s float32) float32 {
	if p, ok := c.profileAt(s); ok {
		return 1 - p.corridor
	}
	return 1
}

// Cracks returns the cracks overlapping [from, to).
func (c *Course) Cracks(from, to float32) []Crack {
	i := sort.Search(len(c.cracks), func(i int) bool { return c.cracks[i].S+c.cracks[i].Width+1 > from })
	j := i
	for j < len(c.cracks) && c.cracks[j].S-rampLength < to {
		j++
	}
	return c.cracks[i:j]
}

// CrackAt returns the crack (if any) whose opening or kicker covers s.
func (c *Course) CrackAt(s float32) (Crack, bool) {
	for _, k := range c.Cracks(s-1, s+rampLength+1) {
		if s > k.S-rampLength-1 && s < k.S+k.Width+1 {
			return k, true
		}
	}
	return Crack{}, false
}

// kicker is how far crack k's take-off ramp lifts the slope at s: rising
// along a parabola to rampHeight at the lip, one row before the opening.
func (k Crack) kicker(s float32) float32 {
	if t := (s - (k.S - rampLength)) / (rampLength - 1); t > 0 && t <= 1 {
		return rampHeight * t * t
	}
	return 0
}

// inJumpZone reports whether s is on a crack's run-in, opening or landing.
func (c *Course) inJumpZone(s float32) bool {
	for _, k := range c.Cracks(s-landing-8, s+rampLength+4) {
		if s > k.S-rampLength-4 && s < k.S+k.Width+landing {
			return true
		}
	}
	return false
}

// Height is the ground height at world (x, z), cracks included. It is what
// the chunk meshes are built from and what the physics collides with.
func (c *Course) Height(x, z float32) float32 {
	h, cut := c.surface(x, z)
	return h - cut
}

// Rim is the ground height at (x, z) as if no crack were there; a ball well
// below it has fallen into a crack.
func (c *Course) Rim(x, z float32) float32 {
	h, _ := c.surface(x, z)
	return h
}

// surface returns the ground height without cracks and how deep a crack cuts
// into it at (x, z).
func (c *Course) surface(x, z float32) (height, cut float32) {
	s := -z
	u := x - c.Centre(s)
	au := abs(u)
	w := c.valleyHalfWidth(s)

	// The slope itself, with long gentle rolls.
	base := -drop(s) + 2.5*float32(c.shape.Line(float64(s)/60+20))
	h := base

	// Channel: a shallow bowl, then snow berms, then peaks further out.
	bowl := min(au, w)
	h += 0.012 * bowl * bowl
	if e := au - w; e > 0 {
		// Tall banks: a berm that keeps climbing into a mountain wall.
		h += 30*(1-float32(math.Exp(-float64(e)/11))) + e*0.4
		ridges := float32(c.peaks.Ridged(float64(x)/90, float64(z)/90, 5, 0.5))
		h += ridges * 90 * smoothstep(14, 90, e)
	}
	// Moguls on the floor, fading out up the banks; bumpier further down.
	floor := 1 - smoothstep(w-2, w+6, au)
	h += (0.5 + 0.25*Difficulty(s)) * float32(c.detail.FBM(float64(x)/6, float64(z)/6, 3, 0.5)) * floor

	// Behind the start: the cliff the ball is dropped from.
	h += cliffHeight * smoothstep(-2, -10, s)

	if p, ok := c.profileAt(s); ok && p.corridor > 0 {
		h = c.corridor(x, z, base, h, p)
	}

	// Cracks and their kickers span the channel and fade out on the banks.
	across := 1 - smoothstep(w+4, w+18, au)
	if across > 0 {
		for _, k := range c.Cracks(s-1, s+rampLength+1) {
			h += k.kicker(s) * across
			// Full depth on the rows inside the crack, linear to zero one row
			// either side, exactly as the mesh interpolates it.
			open := clamp(s-(k.S-rowStep), 0, 1) * clamp(k.S+k.Width+rowStep-s, 0, 1)
			cut = max(cut, crackDepth*open*across)
		}
	}
	return h, cut
}

// corridor reshapes the valley height hv at (x, z) for a special section.
// The valley sinks towards the pit, and the path's own surface replaces it:
// a ridge is the summit of the same ridged mountains that wall the valley,
// a narrows is a gorge cut through them.
func (c *Course) corridor(x, z, base, hv float32, p profile) float32 {
	sunk := hv + (base-pitDepth-hv)*p.pit
	var local float32
	if p.kind == Narrows {
		local = c.gorge(x, z, base, sunk, p)
	} else {
		local = c.summit(x, z, base, sunk, p)
	}
	return sunk + (local-sunk)*p.corridor
}

// summit is the mountain top the ridge rides. The crest is a narrow crown.
// Past it, the ground is the same ridged peaks that wall the valley: sharp
// crests and rock faces, tens of metres of relief, sloping away on both
// sides and then into the pit. Those peaks stay at or below the line you ride.
func (c *Course) summit(x, z, base, hv float32, p profile) float32 {
	s := -z
	path := c.Centre(s) + p.shift
	au := abs(x - path)
	sharp := float32(0)
	if ridgeLift > 0 {
		sharp = p.lift / ridgeLift
	}
	// Long and shallow, so the crest never climbs against the mountain's grade.
	on := float32(c.peaks.Ridged(float64(-s)/220, 1.3, 2, 0.5))
	spine := base + p.lift + (on-0.4)*14*sharp

	floor := max(p.halfWidth, 4)
	u := min(au/floor, 1)
	// A little of the mountain's own grain on the crown, so the snow isn't a
	// graded ramp. Small enough that the line stays downhill and rideable.
	grain := float32(c.detail.FBM(float64(x)/7, float64(z)/7, 2, 0.5)) * 0.35 * sharp
	crown := spine - 0.7*u*u*sharp + grain*(1-u)

	// Same field as the side mountains, zero on the racing line and full a
	// few metres off it, which is where the camera sees them.
	off := float32(c.peaks.Ridged(float64(x)/64, float64(z)/64, 5, 0.5))
	along := float32(c.peaks.Ridged(float64(path)/64, float64(-s)/64, 5, 0.5))
	grow := smoothstep(floor-0.5, floor+3, au)
	peaks := (off - along) * 70 * grow * sharp
	// Upward ridges stand beside the crest. Further out they aren't allowed
	// to rebuild a bench, or a ball that leaves the line can land and ride.
	if upCap := 9 * (1 - smoothstep(floor+1, floor+9, au)); peaks > upCap {
		peaks = upCap
	}
	d := max(au-floor, 0)
	// A mountainside, not a wall: steep enough for rock, shallow enough that
	// the ridges read as peaks instead of wrinkles on a cliff.
	slope := (0.38*d + 0.008*d*d) * sharp
	local := crown - slope + peaks
	roof := spine - 0.08*d*sharp
	if local > roof {
		local = roof + (local-roof)*0.35
	}
	// The pit takes over only once the shoulders have had room to be mountains.
	if hv < local {
		t := smoothstep(floor+14, floor+36, au)
		local += (hv - local) * t
	}
	return local
}

// gorge is the narrows: a snaking floor between walls built from the same
// ridged peaks as the side mountains, pinching and opening along the way.
func (c *Course) gorge(x, z, base, hv float32, p profile) float32 {
	s := -z
	u := x - (c.Centre(s) + p.shift)
	au := abs(u)
	jag := float32(c.peaks.Ridged(float64(x)/16, float64(z)/16, 4, 0.55))
	big := float32(c.peaks.Ridged(float64(x)/48, float64(z)/48, 3, 0.5))
	roll := float32(c.shape.Line(float64(s)/34+6)) * 2 * p.walls
	floor := base + roll - 0.012*u*u
	e := au - p.halfWidth
	if e <= 0 {
		return floor
	}
	// Leaning, not vertical. A vertical face hides the ridges: height variation
	// only wrinkles it. On a slope, the same ridges step the wall in and out.
	fold := (0.55*big + jag - 0.75) * 26 * smoothstep(1.5, 9, e)
	rise := max(2.0*e, 2.8*e+fold)
	face := floor + rise*p.walls
	// The first stretch of wall stays the face, so the gorge doesn't melt
	// back into the snow bank. Further out it joins the surrounding mountains.
	if e < 8 {
		return face
	}
	t := smoothstep(8, 34, e)
	return face + (hv-face)*t
}

// StartPosition is where the ball is dropped: over the cliff edge behind the start.
func (c *Course) StartPosition() mathx.Vec3 {
	const s = -9
	return mathx.Vec3{c.Centre(s), c.Rim(c.Centre(s), -s) + 1.5, -s}
}

// Chunk is one generated piece of the level.
type Chunk struct {
	Index     int
	Start     float32 // s at the chunk's first row
	Mesh      geom.MeshData
	Obstacles []Obstacle
}

// ChunkAt returns the index of the chunk containing distance s.
func ChunkAt(s float32) int { return int(math.Floor(float64(s / ChunkLength))) }

// Chunk builds chunk index (index 0 starts at s = 0; negative chunks lie
// behind the start). Neighbouring chunks share their boundary row exactly.
func (c *Course) Chunk(index int) Chunk {
	start := float32(index) * ChunkLength
	rows := ChunkLength/rowStep + 1
	offsets := columnOffsets()
	mesh := geom.Grid(columns, rows, func(i, j int) mathx.Vec3 {
		s := start + float32(j*rowStep)
		x := c.PathCentre(s) + offsets[i] // columns packed tightly around the path
		return mathx.Vec3{x, c.Height(x, -s), -s}
	})
	return Chunk{Index: index, Start: start, Mesh: mesh, Obstacles: c.obstacles(index, start)}
}

// Obstacles returns chunk index's obstacles without building its mesh.
func (c *Course) Obstacles(index int) []Obstacle {
	return c.obstacles(index, float32(index)*ChunkLength)
}

// columnOffsets spaces the mesh columns across the course: about a metre
// apart on the path, widening to ~9 m over the far mountains.
func columnOffsets() []float32 {
	const a = 0.25 // share of linear spacing; the rest is cubic
	out := make([]float32, columns)
	for i := range out {
		t := 2*float32(i)/(columns-1) - 1
		out[i] = halfSpan * (a*t + (1-a)*t*t*t)
	}
	return out
}

// obstacles places a chunk's rocks and trees. Each chunk has its own random
// stream, so it doesn't matter which chunks were generated before.
func (c *Course) obstacles(index int, start float32) []Obstacle {
	if start+ChunkLength <= StartClear {
		return nil
	}
	rng := rand.New(rand.NewPCG(c.Seed^0x0b57ac1e, uint64(int64(index))))
	mid := start + ChunkLength/2
	d := Difficulty(mid)
	rocks := 2 + int(9*d) + rng.IntN(3)
	trees := 5 + int(6*d) + rng.IntN(4)
	if _, ok := c.profileAt(mid); ok {
		return c.sectionObstacles(rng, start, d)
	}

	var out []Obstacle
	// place adds an obstacle at distance s, u across from the centre line.
	// Gate rocks are packed into a wall, so they skip the spacing check.
	place := func(kind ObstacleKind, s, u float32, gate bool) {
		if s < StartClear || s < start || s >= start+ChunkLength {
			return
		}
		if c.inJumpZone(s) {
			return // keep kickers, cracks and landings clear
		}
		if c.overlapsSection(s-20, s+20) {
			return // sections place their own (see sectionObstacles)
		}
		x, z := c.Centre(s)+u, -s
		base := mathx.Vec3{x, c.Height(x, z), z}
		o := Obstacle{Kind: kind, Base: base, Distance: s, Yaw: rng.Float32() * 2 * math.Pi, Variant: rng.IntN(1 << 16)}
		switch {
		case gate:
			o.Scale = 0.95 + 0.25*rng.Float32()
			o.Squash = 0.75 + 0.2*rng.Float32()
			o.Radius = o.Scale * 0.85
			o.Centre = base.Add(mathx.Vec3{0, o.Scale * o.Squash * 0.35, 0})
			out = append(out, o)
			return
		case kind == Rock:
			o.Scale = 0.7 + rng.Float32()*1.1 + 0.4*d
			o.Squash = 0.6 + 0.3*rng.Float32()
			o.Radius = o.Scale * 0.85
			o.Centre = base.Add(mathx.Vec3{0, o.Scale * o.Squash * 0.35, 0})
		default:
			o.Scale = 0.8 + rng.Float32()*0.6
			o.Radius = 0.8 * o.Scale
			o.Centre = base.Add(mathx.Vec3{0, 0.9 * o.Scale, 0})
		}
		for _, p := range out {
			if p.Centre.Sub(o.Centre).Len() < p.Radius+o.Radius+1.5 {
				return // too crowded
			}
		}
		out = append(out, o)
	}
	// Gates: a wall of rocks across the channel with one gap, more often and
	// tighter the further you get.
	if d > 0.2 && rng.Float32() < d {
		s := start + 6 + rng.Float32()*(ChunkLength-12)
		w := c.HalfWidth(s)
		gap := 7 - 3*d // metres
		at := (rng.Float32()*2 - 1) * (w - gap/2 - 1)
		for u := -w - 1; u <= w+1; u += 2.3 {
			if abs(u-at) > gap/2 {
				place(Rock, s, u, true)
			}
		}
	}
	for range rocks {
		s := start + rng.Float32()*ChunkLength
		w := c.HalfWidth(s)
		place(Rock, s, (rng.Float32()*2-1)*(w-1), false)
	}
	for range trees {
		s := start + rng.Float32()*ChunkLength
		w := c.HalfWidth(s)
		if rng.Float32() < 0.25+0.35*d { // some in the channel, more as it gets harder
			place(Tree, s, (rng.Float32()*2-1)*(w-2), false)
		} else { // most line the banks
			side := float32(1)
			if rng.IntN(2) == 0 {
				side = -1
			}
			place(Tree, s, side*(w-1+rng.Float32()*16), false)
		}
	}
	return out
}

// sectionObstacles places a chunk's obstacles inside a special section: none
// on its transitions (the climb, the pinch). On a ridge's crest, rocks
// across it and pines clinging to its edges; in the narrows, rocks and pines
// hugging alternate walls with boulders out on the floor, so you weave
// between them. One at a time along the run, with room to swerve.
func (c *Course) sectionObstacles(rng *rand.Rand, start, d float32) []Obstacle {
	var out []Obstacle
	count := 5 + int(6*d) + rng.IntN(3)
	for range count {
		s := start + rng.Float32()*ChunkLength
		q, ok := c.profileAt(s)
		if !ok || !q.stable {
			continue
		}
		hw := c.PathHalfWidth(s)
		o := Obstacle{Kind: Rock, Distance: s, Yaw: rng.Float32() * 2 * math.Pi, Variant: rng.IntN(1 << 16)}
		side := float32(1)
		if rng.IntN(2) == 0 {
			side = -1
		}
		var u float32
		switch pick := rng.Float32(); {
		case pick < 0.3: // a pine on the edge
			o.Kind = Tree
			o.Scale = 0.7 + 0.4*rng.Float32()
			u = side * (hw - 0.8*o.Scale)
		case q.kind == Narrows && pick < 0.7: // a rock against the wall
			o.Scale = 0.7 + 0.5*rng.Float32()
			u = side * (hw - o.Scale*0.6)
		case q.kind == Narrows: // a boulder out on the floor
			o.Scale = 0.6 + 0.4*rng.Float32()
			u = (rng.Float32()*2 - 1) * (hw - 3)
		default: // a rock on the crest
			o.Scale = 0.7 + 0.7*rng.Float32()
			u = (rng.Float32()*2 - 1) * (hw - 2)
		}
		x, z := c.PathCentre(s)+u, -s
		o.Base = mathx.Vec3{x, c.Height(x, z), z}
		if o.Kind == Tree {
			o.Radius = 0.8 * o.Scale
			o.Centre = o.Base.Add(mathx.Vec3{0, 0.9 * o.Scale, 0})
		} else {
			o.Squash = 0.6 + 0.3*rng.Float32()
			o.Radius = o.Scale * 0.85
			o.Centre = o.Base.Add(mathx.Vec3{0, o.Scale * o.Squash * 0.35, 0})
		}
		crowded := false
		for _, q := range out {
			if abs(q.Distance-s) < 9-2*d { // room to swerve between them
				crowded = true
			}
		}
		if !crowded {
			out = append(out, o)
		}
	}
	return out
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

// smoothstep is 0 at edge0, 1 at edge1 (either order) and smooth between.
func smoothstep(edge0, edge1, v float32) float32 {
	t := clamp((v-edge0)/(edge1-edge0), 0, 1)
	return t * t * (3 - 2*t)
}
