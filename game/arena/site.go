package arena

import (
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// Site is a generated demolition environment: indestructible ground and
// boundary, destructible structures on a grid of lots, and a spawn point.
type Site struct {
	Blocks     []Block
	Structures []*Structure
	Spawn      mathx.Vec3
	HalfSize   float32
}

const (
	siteHalf = 34.0 // metres; the playable square is -siteHalf..siteHalf
	lotPitch = 22.0 // distance between lot centres (3 x 3 lots)
)

// lotKind is what goes on a lot.
type lotKind int

const (
	lotEmpty lotKind = iota
	lotHouse
	lotTower
	lotBunker
	lotGlass
	lotWalls
)

// GenerateSite builds a random site from seed: a 3 x 3 grid of lots, each
// with a house, tower, bunker, glasshouse, freestanding walls or nothing, at
// a random quarter turn, plus scattered crates. The south-middle lot is the
// spawn and stays open.
func GenerateSite(seed uint64) *Site {
	rng := rand.New(rand.NewPCG(seed, seed^0xd1b54a32d192ed03))
	s := &Site{HalfSize: siteHalf}

	id := mathx.QuatIdentity()
	const wall = 3.0
	s.Blocks = []Block{
		{Kind: Floor, Centre: mathx.Vec3{0, -0.5, 0}, Half: mathx.Vec3{siteHalf + 1, 0.5, siteHalf + 1}, Rotation: id},
		{Kind: Wall, Centre: mathx.Vec3{0, wall / 2, -siteHalf - 0.5}, Half: mathx.Vec3{siteHalf + 1, wall / 2, 0.5}, Rotation: id},
		{Kind: Wall, Centre: mathx.Vec3{0, wall / 2, siteHalf + 0.5}, Half: mathx.Vec3{siteHalf + 1, wall / 2, 0.5}, Rotation: id},
		{Kind: Wall, Centre: mathx.Vec3{-siteHalf - 0.5, wall / 2, 0}, Half: mathx.Vec3{0.5, wall / 2, siteHalf + 1}, Rotation: id},
		{Kind: Wall, Centre: mathx.Vec3{siteHalf + 0.5, wall / 2, 0}, Half: mathx.Vec3{0.5, wall / 2, siteHalf + 1}, Rotation: id},
	}

	// Weighted lot types: mostly buildings.
	pick := []lotKind{lotHouse, lotHouse, lotHouse, lotTower, lotTower, lotBunker, lotGlass, lotWalls, lotWalls, lotEmpty}
	build := map[lotKind]func(*rand.Rand, mathx.Vec3, int) *Structure{
		lotHouse: House, lotTower: Tower, lotBunker: Bunker, lotGlass: Glasshouse, lotWalls: Walls,
	}
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			centre := mathx.Vec3{float32(i) * lotPitch, 0, float32(j) * lotPitch}
			if i == 0 && j == 1 {
				s.Spawn = centre.Add(mathx.Vec3{0, PlayerRadius + 0.02, 4})
				s.Structures = append(s.Structures, Crates(rng, centre.Add(mathx.Vec3{3, 0, -2}), rng.IntN(4)))
				continue
			}
			// Jitter within the lot so the grid doesn't read as a grid.
			at := centre.Add(mathx.Vec3{(rng.Float32() - 0.5) * 4, 0, (rng.Float32() - 0.5) * 4})
			kind := pick[rng.IntN(len(pick))]
			if kind == lotEmpty {
				s.Structures = append(s.Structures, Crates(rng, at, rng.IntN(4)))
				continue
			}
			s.Structures = append(s.Structures, build[kind](rng, at, rng.IntN(4)))
			if rng.IntN(2) == 0 { // some lots get crates beside the building
				side := mathx.Vec3{float32(1-2*rng.IntN(2)) * 8, 0, float32(1-2*rng.IntN(2)) * 8}
				s.Structures = append(s.Structures, Crates(rng, at.Add(side), rng.IntN(4)))
			}
		}
	}
	return s
}
