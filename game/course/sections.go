package course

import (
	"math/rand/v2"
	"sort"
)

// SectionKind is a stretch of the course with its own character.
type SectionKind int

const (
	// Valley is the default: the snow channel between rising banks.
	Valley SectionKind = iota
	// Ridge: the path climbs out of the valley onto one side's mountain — the
	// same ridged peaks that wall the run — and rides the summit. Both sides
	// fall away, into the valley and then into a bottomless pit. Further on
	// the pit fills back in and the path drops into the valley.
	Ridge
	// Narrows: the channel becomes a gorge through those mountains, snaking
	// and pinching between ridged walls.
	Narrows
)

// Section is a special stretch of the course between Start and Start+Length.
type Section struct {
	Kind          SectionKind
	Start, Length float32
	Side          float32 // Ridge: which bank the path climbs, +1 for +x (right when heading down), -1 for -x
}

func (k Section) End() float32 { return k.Start + k.Length }

const (
	sectionsFrom = 600 // metres of plain valley before the first special section

	ridgeClimb     = 170 // metres from the section start to the top of the ridge (and back down)
	ridgeOffset    = 50  // how far sideways the ridge path sits from the valley's centre line
	ridgeLift      = 28  // how high the summit rises above the valley floor, before its own rolls
	ridgeHalfWidth = 5   // half the rideable crest; the mountain face starts outside it
	pitDepth       = 260 // how far the valley sinks below the floor under a ridge

	narrowsIn   = 90 // metres over which the gorge closes in (and opens again)
	narrowsWall = 42 // how high the gorge walls rise, before the ridged skyline
)

// generateSections lays out the special sections: after a warm-up of plain
// valley, a ridge or narrows every few hundred metres, with valley between,
// the gaps shortening as the run gets harder.
func (c *Course) generateSections() {
	rng := rand.New(rand.NewPCG(c.Seed, 0x5ec7))
	for s := float32(sectionsFrom + rng.IntN(150)); s < 200000; {
		k := Section{Kind: Ridge, Start: s, Side: 1}
		if rng.IntN(2) == 0 {
			k.Side = -1
		}
		if rng.IntN(2) == 0 {
			k.Kind = Narrows
			k.Length = float32(300 + rng.IntN(150))
		} else {
			k.Length = float32(2*ridgeClimb + 180 + rng.IntN(180))
		}
		c.sections = append(c.sections, k)
		s = k.End() + (180+float32(rng.IntN(220)))*(1-0.3*Difficulty(s))
	}
}

// SectionAt returns the special section covering s, if any.
func (c *Course) SectionAt(s float32) (Section, bool) {
	i := sort.Search(len(c.sections), func(i int) bool { return c.sections[i].End() > s })
	if i < len(c.sections) && c.sections[i].Start <= s {
		return c.sections[i], true
	}
	return Section{}, false
}

// NextSection returns the first special section starting after s.
func (c *Course) NextSection(s float32) (Section, bool) {
	i := sort.Search(len(c.sections), func(i int) bool { return c.sections[i].Start > s })
	if i < len(c.sections) {
		return c.sections[i], true
	}
	return Section{}, false
}

// overlapsSection reports whether [from, to) touches any special section.
func (c *Course) overlapsSection(from, to float32) bool {
	i := sort.Search(len(c.sections), func(i int) bool { return c.sections[i].End() > from })
	return i < len(c.sections) && c.sections[i].Start < to
}

// profile is how a special section reshapes the course at one distance: the
// path's corridor blended in over the valley, which may sink away into a pit.
// On a ridge the corridor is the mountain's summit; in the narrows it is a gorge.
type profile struct {
	kind      SectionKind
	shift     float32 // path centre's offset from the valley centre line
	lift      float32 // path surface's height above the valley floor
	pit       float32 // 0..1: how far the valley has sunk into the pit
	corridor  float32 // 0..1: how much the path corridor replaces the valley
	halfWidth float32 // half the width of the path corridor's floor
	walls     float32 // 0..1: sheer walls rise beside the corridor (narrows)
	stable    bool    // well inside the section, past its transitions
}

// profileAt returns the section profile at s; ok is false in plain valley.
func (c *Course) profileAt(s float32) (p profile, ok bool) {
	k, ok := c.SectionAt(s)
	if !ok {
		return profile{}, false
	}
	t, l := s-k.Start, k.Length
	w := c.valleyHalfWidth(s)
	p.kind = k.Kind
	switch k.Kind {
	case Ridge:
		up := smoothstep(0, ridgeClimb, t) * (1 - smoothstep(l-ridgeClimb, l, t))
		// Out onto the mountain, and snaking along its crest rather than a straight cut.
		p.shift = (k.Side*ridgeOffset + c.crestWeave(s)) * up
		p.lift = ridgeLift * up
		// The pit opens once the path is up on the crest and closes before it comes down.
		p.pit = smoothstep(ridgeClimb-30, ridgeClimb+60, t) * (1 - smoothstep(l-ridgeClimb-90, l-ridgeClimb+30, t))
		p.corridor = smoothstep(0, 30, t) * (1 - smoothstep(l-30, l, t))
		p.halfWidth = w + (ridgeHalfWidth-w)*up
		p.stable = t > ridgeClimb+60 && t < l-ridgeClimb-90
	case Narrows:
		in := smoothstep(0, narrowsIn, t) * (1 - smoothstep(l-narrowsIn, l, t))
		p.corridor = smoothstep(0, 20, t) * (1 - smoothstep(l-20, l, t))
		p.shift = c.gorgeWeave(s) * in
		p.halfWidth = w + (c.gorgeHalfWidth(s)-w)*in
		p.walls = in
		p.stable = t > narrowsIn && t < l-narrowsIn
	}
	return p, true
}

// crestWeave is how far the summit snakes off the mountain's midline (metres,
// either side). Long and gentle: the bend has to stay inside what steering
// can follow at speed.
func (c *Course) crestWeave(s float32) float32 {
	return float32(c.shape.Line(float64(s)/140+4)) * 8
}

// gorgeWeave is the gorge centre's wander, metres either side of the valley line.
func (c *Course) gorgeWeave(s float32) float32 {
	return float32(c.shape.Line(float64(s)/100+9)) * 10
}

// gorgeHalfWidth is half the gorge floor at s: it pinches and opens, about
// 4.1 m to 8.9 m, instead of one constant slot.
func (c *Course) gorgeHalfWidth(s float32) float32 {
	n := clamp(float32(c.shape.Line(float64(s)/46+2)), -1, 1)
	return 6.5 + 2.4*n
}
