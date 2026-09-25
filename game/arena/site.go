package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// Site is a generated arena: its indestructible shell (floor, walls,
// platforms, ramps and the launch bays), destructible cover, launch pads
// and spawn points.
type Site struct {
	Blocks     []Block
	Structures []*Structure
	Pads       []Pad
	Spawns     []Spawn
	Bounds     [2]float32 // half extents of the playable floor on X and Z
}

// Spawn is where a player starts a round, and which way they face.
type Spawn struct {
	At  mathx.Vec3
	Yaw float32
}

// Pad is a launcher: stand on it and it throws you with Launch velocity.
// Spawn pads sit in the launch bays and only fire once the round is live.
type Pad struct {
	Centre mathx.Vec3 // on its surface
	Radius float32
	Launch mathx.Vec3
	Spawn  bool
}

// The arena's shape, in the spirit of Halo 5's Breakout: a compact,
// symmetric box with an end for each side. South (+Z) is player 0's end.
const (
	arenaHalfX  = 20.0
	arenaHalfZ  = 30.0
	wallHeight  = 5.0
	endWallLow  = 3.2 // the end wall in front of each bay, below the bay floor
	bayFloor    = 4.0 // launch bays sit above and behind the end walls
	bayDepth    = 7.0
	bayHalfW    = 6.0
	PlatformH   = 2.5 // the centre platform
	ledgeH      = 2.0 // the side ledges
	launchSpeed = 12  // m/s towards the middle, out of the bay
	launchLift  = 7   // m/s up
)

// GenerateSite builds the arena for seed. The shell is always the same:
//   - a walled floor, 40 x 60 m
//   - a raised centre platform with ramps down to the east and west
//   - raised ledges along the side walls with ramps at their ends
//   - four tall pillars
//   - a jump pad at each end that throws you onto the centre platform
//   - a launch bay above and behind each end wall, whose pads fire the
//     players into the arena when the round starts
//
// The destructible cover (low and tall walls, L-shaped cover, crates and the
// odd bunker) is random, but mirrored through the centre so both ends play
// the same.
func GenerateSite(seed uint64) *Site {
	rng := rand.New(rand.NewPCG(seed, seed^0xd1b54a32d192ed03))
	s := &Site{Bounds: [2]float32{arenaHalfX, arenaHalfZ}}
	s.shell()
	s.cover(rng)
	return s
}

func (s *Site) add(k BlockKind, centre, half mathx.Vec3) {
	s.Blocks = append(s.Blocks, Block{Kind: k, Centre: centre, Half: half, Rotation: mathx.QuatIdentity()})
}

// shell adds the indestructible geometry, the pads and the spawns.
func (s *Site) shell() {
	const hx, hz = arenaHalfX, arenaHalfZ
	v := func(x, y, z float32) mathx.Vec3 { return mathx.Vec3{x, y, z} }

	s.add(Floor, v(0, -0.5, 0), v(hx+1, 0.5, hz+bayDepth+2))
	for _, sx := range []float32{-1, 1} {
		for _, sz := range []float32{-1, 1} { // the long walls, in halves (one per team colour)
			s.add(Wall, v(sx*(hx+0.5), wallHeight/2, sz*(hz+1)/2), v(0.5, wallHeight/2, (hz+1)/2))
		}
	}
	s.add(Platform, v(0, PlatformH/2, 0), v(5, PlatformH/2, 3.5))
	for _, sx := range []float32{-1, 1} {
		s.Blocks = append(s.Blocks, ramp(v(sx*12.5, 0, 0), v(sx*5, PlatformH, 0), 1.75))
		s.add(Platform, v(sx*(hx-3), ledgeH/2, 0), v(3, ledgeH/2, 9))
	}

	for i, sz := range []float32{1, -1} { // south (player 0's) end, then north
		z := func(d float32) float32 { return sz * d }
		// The end wall: full height at the sides, low in front of the bay.
		side := float32(hx-bayHalfW) / 2
		for _, sx := range []float32{-1, 1} {
			s.add(Wall, v(sx*(bayHalfW+side), wallHeight/2, z(hz+0.5)), v(side+0.5, wallHeight/2, 0.5))
		}
		s.add(Wall, v(0, endWallLow/2, z(hz+0.5)), v(bayHalfW, endWallLow/2, 0.5))

		// The launch bay: a floor at bayFloor behind the wall, walled in.
		bay := float32(hz + 1 + bayDepth/2)
		s.add(Bay, v(0, bayFloor/2, z(bay)), v(bayHalfW, bayFloor/2, bayDepth/2))
		s.add(Bay, v(0, bayFloor+1.5, z(hz+1.5+bayDepth)), v(bayHalfW+1, 1.5, 0.5))
		for _, sx := range []float32{-1, 1} {
			s.add(Bay, v(sx*(bayHalfW+0.5), bayFloor+1.5, z(bay)), v(0.5, 1.5, bayDepth/2))
		}

		// Ledge ramps, pillars and the end's jump pad onto the centre platform.
		for _, sx := range []float32{-1, 1} {
			s.Blocks = append(s.Blocks, ramp(v(sx*(hx-3), 0, z(16.5)), v(sx*(hx-3), ledgeH, z(9)), 1.5))
			s.add(Pillar, v(sx*9, wallHeight/2, z(15)), v(1, wallHeight/2, 1))
		}
		s.Pads = append(s.Pads, Pad{Centre: v(0, 0, z(9)), Radius: 1.1, Launch: v(0, 10, -sz*8)})

		// Two launch pads per bay (room for 2v2 later), facing the middle.
		yaw := float32(0)
		if sz < 0 {
			yaw = math.Pi
		}
		for j, sx := range []float32{-1, 1} {
			at := v(sx*2.5, bayFloor, z(bay-0.5))
			s.Pads = append(s.Pads, Pad{Centre: at, Radius: 1.2, Launch: v(0, launchLift, -sz*launchSpeed), Spawn: true})
			sp := Spawn{At: at.Add(v(0, PlayerRadius+0.02, 0)), Yaw: yaw}
			// Order: south 0, north 0, south 1, north 1, so players 0 and 1
			// start at opposite ends whichever way the sides are rotated.
			idx := j*2 + i
			for len(s.Spawns) <= idx {
				s.Spawns = append(s.Spawns, Spawn{})
			}
			s.Spawns[idx] = sp
		}
	}
}

// keepClear are the parts of the south half's floor cover mustn't block:
// the shell's platforms, ramps and pillars, the jump pad, and where players
// land from the launch bays. (The north half mirrors it.)
var keepClear = [][2]mathx.Vec3{
	{{-13, 0, -1}, {13, 0, 5}},                             // centre platform and ramps
	{{-arenaHalfX, 0, -1}, {-arenaHalfX + 7.5, 0, 18}},     // west ledge and ramp
	{{arenaHalfX - 7.5, 0, -1}, {arenaHalfX, 0, 18}},       // east ledge and ramp
	{{-11, 0, 13}, {-7, 0, 17}}, {{7, 0, 13}, {11, 0, 17}}, // pillars
	{{-2.5, 0, 6.5}, {2.5, 0, 11.5}},                    // jump pad
	{{-6, 0, 14}, {6, 0, 23}},                           // landing zone
	{{-arenaHalfX, 0, 27}, {arenaHalfX, 0, arenaHalfZ}}, // along the end wall
}

// cover places random destructible cover in the south half and a copy
// turned half round in the north half.
func (s *Site) cover(rng *rand.Rand) {
	type option struct {
		weight int
		build  func(*rand.Rand, mathx.Vec3, int) *Structure
	}
	options := []option{{5, LowWall}, {3, TallWall}, {3, LCover}, {2, Crates}, {1, Bunker}}
	total := 0
	for _, o := range options {
		total += o.weight
	}
	var placed [][2]mathx.Vec3 // footprints so far (south half), grown by a margin
	want := 7 + rng.IntN(3)
	for tries := 0; tries < 400 && want > 0; tries++ {
		pick := rng.IntN(total)
		var build func(*rand.Rand, mathx.Vec3, int) *Structure
		for _, o := range options {
			if pick < o.weight {
				build = o.build
				break
			}
			pick -= o.weight
		}
		at := mathx.Vec3{(rng.Float32()*2 - 1) * (arenaHalfX - 3), 0, 3 + rng.Float32()*(arenaHalfZ-6)}
		turns, seed := rng.IntN(4), rng.Uint64()
		south := build(rand.New(rand.NewPCG(seed, 1)), at, turns)
		lo, hi := footprint(south)
		if !fits(lo, hi, keepClear, 0.4) || !fits(lo, hi, placed, 1.6) {
			continue
		}
		mirror := mathx.Vec3{-at[0], 0, -at[2]}
		north := build(rand.New(rand.NewPCG(seed, 1)), mirror, turns+2)
		s.Structures = append(s.Structures, south, north)
		placed = append(placed, [2]mathx.Vec3{lo, hi})
		want--
	}
}

// footprint is a structure's extent on the floor (Y ignored).
func footprint(s *Structure) (lo, hi mathx.Vec3) {
	lo = mathx.Vec3{math.MaxFloat32, 0, math.MaxFloat32}
	hi = mathx.Vec3{-math.MaxFloat32, 0, -math.MaxFloat32}
	for _, c := range s.Chunks {
		for _, k := range []int{0, 2} {
			lo[k] = min(lo[k], c.Centre[k]-c.Half[k])
			hi[k] = max(hi[k], c.Centre[k]+c.Half[k])
		}
	}
	return lo, hi
}

// fits reports whether the box lo-hi keeps margin clear of every area.
func fits(lo, hi mathx.Vec3, areas [][2]mathx.Vec3, margin float32) bool {
	if lo[0] < -arenaHalfX+0.5 || hi[0] > arenaHalfX-0.5 || lo[2] < 1.5 || hi[2] > arenaHalfZ-0.5 {
		return false
	}
	for _, a := range areas {
		if lo[0] < a[1][0]+margin && hi[0] > a[0][0]-margin && lo[2] < a[1][2]+margin && hi[2] > a[0][2]-margin {
			return false
		}
	}
	return true
}

// ramp is a slab of the given half-width running from foot (on the ground)
// up to top, tilted to match. The slab is 0.4 m thick with its top surface on
// that line, and extends a little past both ends so there's no lip.
func ramp(foot, top mathx.Vec3, halfWidth float32) Block {
	const thick = 0.2 // half thickness
	run := top.Sub(foot)
	length := run.Len() + 0.6
	up := run.Normalize() // along the slope, uphill
	flat := mathx.Vec3{up[0], 0, up[2]}.Normalize()
	// Perpendicular to the slope, pointing up: cos(a)*Y - sin(a)*flat.
	normal := mathx.Vec3{0, 1, 0}.Scale(flat.Dot(up)).Sub(flat.Scale(up[1]))
	// Local axes: +Y the surface normal, +Z down the slope, +X across it
	// (X = Y x Z keeps the basis right-handed).
	down := up.Scale(-1)
	rot := mathx.QuatFromBasis(normal.Cross(down), normal, down)
	mid := foot.Add(top).Scale(0.5).Sub(normal.Scale(thick))
	return Block{Kind: Ramp, Centre: mid, Half: mathx.Vec3{halfWidth, thick, length / 2}, Rotation: rot}
}
