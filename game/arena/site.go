package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// Site is a generated arena: its indestructible frame (the girders and the
// launch bays), its destructible structures (the layout's and the cover),
// launch pads and spawn points.
type Site struct {
	Blocks     []Block
	Structures []*Structure
	Pads       []Pad
	Spawns     []Spawn
	Spots      []PickupSpot // where weapons and grenades spawn
	Bounds     [2]float32   // half extents of the playable floor on X and Z
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
	// An aimed pad lands you on Target wherever you step on it, going up at
	// Launch's Y (see throwFrom).
	Target mathx.Vec3
	Aimed  bool
}

// The arena's shape, in the spirit of Halo 5's Breakout, hung in a chasm
// like THE FINALS: a compact, symmetric deck with an end for each side,
// laid on steel girders over a bottomless drop. South (+Z) is player 0's
// end. Almost all of it breaks.
const (
	arenaHalfX = 20.0
	arenaHalfZ = 30.0
	floorThick = 0.4
	floorTile  = 2.0
	parapetH   = 1.1 // the low wall round the edge
	bayGap     = 12  // open air between the arena's end and a launch bay
	bayFloor   = 6.0 // launch bays sit above the arena, jutting from the chasm walls
	bayDepth   = 8.0
	bayHalfW   = 6.0
	// The levels, 2.5 m apart.
	PlatformH = 2.5  // the keep's deck, the bridges, the side ledges and the mountains' saddles
	TopH      = 5.0  // the keep's top tier, the perch towers, the sky bridges and the mountains' lower terraces
	CrownH    = 7.5  // the mountains' upper terraces
	SummitH   = 10.0 // the mountains' summits
	PinnacleH = 12.5 // the pinnacles on the summits
	SpireH    = 29.0 // the top of the spire in the middle of the keep: its cap

	liftSpeed   = 13.5 // m/s up off a jump pad, enough to reach TopH from the floor
	hopSpeed    = 10   // ... and up a level (2.5 m) with room to spare
	launchSpeed = 16   // m/s towards the middle, out of the bay and over the gap
	launchLift  = 9    // m/s up

	// ChasmWall is how far from the middle (along Z) the chasm's rock walls
	// rise, just behind the launch bays.
	ChasmWall = arenaHalfZ + bayGap + bayDepth + 1
	// WaterLevel is where the drop ends, far below.
	WaterLevel = -70
)

// girderX are the long girders the floor rests on.
var girderX = []float32{-16, -8, 0, 8, 16}

// GenerateSite builds the arena for seed. The layout is always the same,
// on six levels 2.5 m apart, from the floor up to the pinnacles
// (PinnacleH).
//   - a 40 x 60 m floor of plates laid on girders over the chasm, with a
//     low parapet round the edge
//   - the keep in the middle: a mid-level deck on columns, grand stairs up
//     to it from each end, a top tier on it reached by stairs from the
//     bridges, and the spire rising from that to SpireH: a concrete core
//     with balconies every spireStep, turning a quarter each time, pads
//     hopping you up from one to the next, and jump pads on the floor that
//     throw you straight up to its third balcony
//   - bridges from the keep across to raised ledges along the sides, with
//     stairs down from the ledges' ends
//   - a perch tower at each corner of the middle, with a lift pad, and a
//     sky bridge from its top across to the mountain beside it
//   - a mountain spur rising out of the chasm along each side: a saddle off
//     the side ledge, rock ramps up two terraces to a summit, and a pad
//     from the summit onto a pinnacle
//   - a launch bay jutting from the chasm wall beyond each end, whose pads
//     fire the players across the gap into the arena when the round starts
//
// Only the girders, the bays and the mountains are indestructible. The floor, the keep,
// bridges, ledges, towers, stairs and parapets break like the cover does,
// and hold each other up: shoot out the keep's columns and its tiers come
// down; blow out the floor under something and it drops into the chasm. The cover (low and tall walls, L-shaped cover, crates, bunkers and
// the odd building) is random, but mirrored through the centre so both ends
// play the same.
func GenerateSite(seed uint64) *Site {
	rng := rand.New(rand.NewPCG(seed, seed^0xd1b54a32d192ed03))
	s := &Site{Bounds: [2]float32{arenaHalfX, arenaHalfZ}}
	s.shell()
	s.mountains()
	south, north := shellHalf(0), shellHalf(2)
	for i := range south {
		s.Structures = append(s.Structures, south[i], north[i])
	}
	s.cover(rng)
	s.pickupSpots(rng)
	return s
}

// pickupSpotsSouth are places a weapon or grenades can spawn, in the south
// half (each is mirrored into the north): up the spire, on the ledges,
// bridges, keep and towers, and out on the mountains.
var pickupSpotsSouth = []mathx.Vec3{
	{0, SpireH, 1.5},       // the spire's cap
	{17, PlatformH, 6},     // a side ledge
	{towerX, TopH, 19.6},   // a tower top
	{28, SummitH, 0.8},     // a mountain's summit
	{0, PlatformH, 5.2},    // the keep's deck
	{11.5, PlatformH, 0.6}, // a bridge
	{27.5, TopH, 18},       // a mountain's lower terrace
	{28, CrownH, 8.5},      // ... and upper terrace
	{0, 0, 16.5},           // the floor, where you land from the bay
	{towerX, 0, 22.2},      // the floor behind a tower
}

// pickupSpots puts the power weapons and grenades at random spots, the same
// at both ends. Everyone starts with the rifle and pistol; these are what's
// worth going out for.
func (s *Site) pickupSpots(rng *rand.Rand) {
	pool := []Pickup{
		weaponPickup(WeaponSniper, mathx.Vec3{}), weaponPickup(WeaponLauncher, mathx.Vec3{}),
		weaponPickup(WeaponShotgun, mathx.Vec3{}), weaponPickup(WeaponShotgun, mathx.Vec3{}),
		grenadePickup(Frag, 2, mathx.Vec3{}), grenadePickup(Sticky, 2, mathx.Vec3{}),
	}
	spots := rng.Perm(len(pickupSpotsSouth))
	for i, p := range pool {
		at := pickupSpotsSouth[spots[i]]
		p.Yaw = rng.Float32() * 2 * math.Pi
		for _, mirror := range []float32{1, -1} {
			q := p
			q.At = mathx.Vec3{mirror * at[0], at[1], mirror * at[2]}
			q.Yaw += (1 - mirror) * math.Pi / 2
			s.Spots = append(s.Spots, PickupSpot{Pickup: q, Respawn: respawnFor(q)})
		}
	}
}

// link links the site's structures together as an Arena does, returning
// every chunk.
func (s *Site) link() []*Chunk { return linkAll(s.Structures, s.Blocks) }

func (s *Site) add(k BlockKind, centre, half mathx.Vec3) {
	s.Blocks = append(s.Blocks, Block{Kind: k, Centre: centre, Half: half, Rotation: mathx.QuatIdentity()})
}

// shell adds the indestructible frame (girders and the launch bays), the
// pads and the spawns.
func (s *Site) shell() {
	const hx, hz = arenaHalfX, arenaHalfZ
	v := func(x, y, z float32) mathx.Vec3 { return mathx.Vec3{x, y, z} }

	// Long girders under the floor, running into the chasm walls, on cross
	// beams.
	const top = -floorThick
	for _, x := range girderX {
		s.add(Girder, v(x, top-0.6, 0), v(0.3, 0.6, ChasmWall+4))
	}
	for _, z := range []float32{-25, -15, -5, 5, 15, 25} {
		s.add(Girder, v(0, top-1.6, z), v(hx+1.5, 0.4, 0.3))
	}

	for i, sz := range []float32{1, -1} { // south (player 0's) end, then north
		z := func(d float32) float32 { return sz * d }
		// The launch bay: a deck jutting from the rock, walled at the sides
		// and back, braced underneath.
		bay := float32(hz + bayGap + bayDepth/2)
		s.add(Deck, v(0, bayFloor-0.75, z(bay)), v(bayHalfW, 0.75, bayDepth/2))
		s.add(Bay, v(0, bayFloor+1.5, z(hz+bayGap+bayDepth+0.5)), v(bayHalfW+1, 1.5, 0.5))
		for _, sx := range []float32{-1, 1} {
			s.add(Bay, v(sx*(bayHalfW+0.5), bayFloor+1.5, z(bay)), v(0.5, 1.5, bayDepth/2))
			s.Blocks = append(s.Blocks, beam(v(sx*4, bayFloor-1.6, z(hz+bayGap+1)), v(sx*4, bayFloor-12, z(ChasmWall+2)), 0.35, 0.5))
		}

		// The spire's pads (the floor's jump pads among them), and a lift
		// pad at the foot of each perch tower.
		s.spirePads(sz)
		for _, sx := range []float32{-1, 1} {
			s.Pads = append(s.Pads, aimed(v(sx*towerX, 0, z(towerZ-liftPadOut)), v(sx*towerX, TopH, z(towerZ-0.3)), liftSpeed+1, 1))
		}

		// Two launch pads per bay (room for 2v2 later), facing the middle.
		yaw := float32(0)
		if sz < 0 {
			yaw = math.Pi
		}
		for j, sx := range []float32{-1, 1} {
			at := v(sx*2.5, bayFloor, z(bay+0.5))
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

// The keep's and the spire's shape. The spire's balconies stick out east
// and west, or north and south, never into the corners between: the
// corners are clear all the way up, for pads to throw you up through.
const (
	keepDeckX, keepDeckZ = 9, 6   // half size of the mid-level deck
	keepTopX, keepTopZ   = 6.5, 4 // ... and of the top tier
	spireCore            = 2.0    // half width of the core: its posts are this wide
	balconyOut           = 6.5    // balconies reach out this far from the middle
	balconyHalf          = 2.5    // ... and are twice this wide
	capHalf              = 3.0    // half width of the cap on top
	spireStep            = 3.0    // m between the spire's levels
	firstBalcony         = TopH + spireStep
	spireHop             = 11   // m/s up off a spire pad: a level up, and under the balcony above
	floorPadX            = 4.2  // the floor's jump pads, either side of the middle ...
	floorPadZ            = 10   // ... in front of the keep
	floorPadHop          = 21.5 // m/s up: from the floor to the spire's third balcony
)

// eastWest reports whether the spire's balconies at height h stick out east
// and west (else north and south).
func eastWest(h float32) bool { return int(math.Round(float64((h-firstBalcony)/spireStep)))%2 == 0 }

// spirePads adds the pads up the spire at end sz: from the floor, up
// through a corner onto the third balcony; from the top tier onto the
// first; from each balcony onto the next, a quarter turn round; and from
// the last onto the cap. Each sits clear of the balcony above, and lands
// you clear of the next pad.
func (s *Site) spirePads(sz float32) {
	const r = 0.45
	v := func(x, y, z float32) mathx.Vec3 { return mathx.Vec3{x, y, z} }
	add := func(at, target mathx.Vec3) {
		s.Pads = append(s.Pads, aimed(at, target, spireHop, r))
	}
	for _, sx := range []float32{-1, 1} {
		s.Pads = append(s.Pads, aimed(v(sx*floorPadX, 0, sz*floorPadZ), v(sx*4.5, firstBalcony+2*spireStep, sz*1.5), floorPadHop, 0.9))
		add(v(sx*4.5, TopH, sz*3.65), v(sx*4.5, firstBalcony, sz*1.5)) // out at the tier's edge: clear of the balcony's edge on the way up
		for h := float32(firstBalcony); h < SpireH; h += spireStep {
			up := h + spireStep
			switch {
			case up >= SpireH: // onto the cap
				add(v(sx*5.8, h, sz*1), v(sx*2, up, sz*1))
			case eastWest(h): // east or west, round to north or south
				add(v(sx*5.8, h, sz*1), v(sx*1.2, up, sz*4.5))
			default: // north or south, round to east or west
				add(v(sx*1, h, sz*5.8), v(sx*4.5, up, sz*1.2))
			}
		}
	}
}

// mountains adds the rock spurs beside the arena. Each is the same seen
// from either end (mirrored through z = 0), so the pair is too.
func (s *Site) mountains() {
	const low = WaterLevel - 5 // the rock goes all the way down
	for _, sx := range []float32{-1, 1} {
		// rock is a block of rock from low up to top over x0..x1, z0..z1 (x
		// on the +X side, mirrored for sx; z mirrored for sz).
		rock := func(x0, x1, z0, z1, top, sz float32) {
			lo := mathx.Vec3{min(sx*x0, sx*x1), low, min(sz*z0, sz*z1)}
			hi := mathx.Vec3{max(sx*x0, sx*x1), top, max(sz*z0, sz*z1)}
			s.add(Rock, lo.Add(hi).Scale(0.5), hi.Sub(lo).Scale(0.5))
		}
		up := func(x, fromZ, fromY, toZ, toY, halfWidth, sz float32) {
			s.Blocks = append(s.Blocks, ramp(mathx.Vec3{sx * x, fromY, sz * fromZ}, mathx.Vec3{sx * x, toY, sz * toZ}, halfWidth))
		}
		for _, sz := range []float32{1, -1} {
			rock(20, 25, 5, 10.5, PlatformH, sz)      // the saddle, off the side ledge
			rock(21, 24.5, 10.5, 15.5, PlatformH, sz) // under the first ramp
			up(22.75, 10.5, PlatformH, 15.5, TopH, 1.75, sz)
			rock(20.5, 30, 15.5, 20, TopH, sz)     // the lower terrace, where the sky bridge lands
			rock(24.5, 25.5, 10.5, 15.5, TopH, sz) // a rib between the ramps
			rock(25.5, 30, 10.5, 15.5, TopH, sz)   // under the second ramp
			up(27.75, 15.5, TopH, 10.5, CrownH, 2.25, sz)
			rock(25, 31, 6, 10.5, CrownH, sz) // the upper terrace
			rock(25, 31, 1.5, 6, CrownH, sz)  // under the last ramp
			up(28, 6, CrownH, 1.5, SummitH, 3, sz)
		}
		rock(25, 31, -1.5, 1.5, SummitH, 1)
		rock(29.5, 32, -1, 1, PinnacleH, 1)
		s.Pads = append(s.Pads, aimed(mathx.Vec3{sx * 26.8, SummitH, 0}, mathx.Vec3{sx * 30.7, PinnacleH, 0}, hopSpeed+1, 0.9))
	}
}

// shellHalf builds the destructible layout of the south half (turns 0) or,
// turned half round, the north half (turns 2): the floor, the parapets, the
// halves of the keep, the bridges and the ledges, and the towers.
func shellHalf(turns int) []*Structure {
	const hx, hz = arenaHalfX, arenaHalfZ
	var out []*Structure
	part := func(name string, build func(b *builder)) *Structure {
		b := newBuilder(name, mathx.Vec3{}, turns)
		build(b)
		s := b.finish()
		s.Shell = true
		out = append(out, s)
		return s
	}

	floor := part("floor", func(b *builder) {
		b.tiles(-hx, 0, hx, hz, 0, floorThick, floorTile, Plate)
	})
	floor.Spanned = true // held up by the girders under it, however much is gone

	part("parapet", func(b *builder) {
		b.trim = true
		const t = 0.3
		b.wall(-hx+t/2, 0, -hx+t/2, hz-t, 0, parapetH, t, Panel)
		b.wall(hx-t/2, 0, hx-t/2, hz-t, 0, parapetH, t, Panel)
		b.wall(-hx, hz-t/2, hx, hz-t/2, 0, parapetH, t, Panel)
	})

	part("keep", func(b *builder) {
		// The mid-level deck, on columns.
		b.trim = true
		b.tiles(-keepDeckX, 0, keepDeckX, keepDeckZ, PlatformH, 0.3, slabTile, Panel)
		b.trim = false
		for _, x := range []float32{-8.4, -4.5, 0, 4.5, 8.4} {
			for _, z := range []float32{1.2, 5.4} {
				b.column(x, z, 0, PlatformH-0.3, 0.6, Panel)
			}
		}
		// Grand stairs up from the floor, either side of the jump pads.
		for _, sx := range []float32{-1, 1} {
			b.stairs(false, 13.5, keepDeckZ, min(sx*5.6, sx*7.8), max(sx*5.6, sx*7.8), 0, PlatformH, Panel)
		}
		// The top tier on the deck, and stairs up to it from the bridges.
		b.trim = true
		b.tiles(-keepTopX, 0, keepTopX, keepTopZ, TopH, 0.3, slabTile, Panel)
		b.trim = false
		for _, x := range []float32{-6.1, -3, 0, 3, 6.1} {
			for _, z := range []float32{1.2, 3.6} {
				b.column(x, z, PlatformH, TopH-PlatformH-0.3, 0.6, Panel)
			}
		}
		for _, sx := range []float32{-1, 1} {
			b.stairs(true, sx*11, sx*keepTopX, 0, 1.25, PlatformH, TopH, Panel) // starting out on the bridge
		}
	})

	part("spire", func(b *builder) {
		// The core: four concrete posts from the top tier to the top.
		for _, x := range []float32{-spireCore / 2, spireCore / 2} {
			b.column(x, spireCore/2, TopH, SpireH-TopH-0.3, spireCore, Concrete) // the cap sits on top
		}
		// Concrete balconies off the core (panels couldn't reach this far),
		// east and west then north and south by turns.
		b.trim = true
		for h := float32(firstBalcony); h < SpireH; h += spireStep {
			if eastWest(h) {
				for _, sx := range []float32{-1, 1} {
					b.tiles(min(sx*spireCore, sx*balconyOut), 0, max(sx*spireCore, sx*balconyOut), balconyHalf, h, 0.3, slabTile, Concrete)
				}
			} else {
				b.tiles(-balconyHalf, spireCore, balconyHalf, balconyOut, h, 0.3, slabTile, Concrete)
			}
		}
		// The cap on the core, walled at the ends (pads land from the sides).
		b.tiles(-capHalf, 0, capHalf, capHalf, SpireH, 0.3, slabTile, Concrete)
		b.wall(-capHalf, capHalf-0.15, capHalf, capHalf-0.15, SpireH, 1.0, 0.3, Concrete)
		b.trim = false
	})

	part("bridges", func(b *builder) {
		for _, sx := range []float32{-1, 1} {
			b.trim = true
			b.tiles(min(sx*keepDeckX, sx*14), 0, max(sx*keepDeckX, sx*14), 1.25, PlatformH, 0.3, 1.25, Panel)
			b.trim = false
			b.column(sx*11.5, 0.6, 0, PlatformH-0.3, 0.6, Panel)
		}
	})

	part("ledges", func(b *builder) {
		for _, sx := range []float32{-1, 1} {
			x0, x1 := sx*14, sx*(hx-0.3)
			b.trim = true
			b.tiles(min(x0, x1), 0, max(x0, x1), 9, PlatformH, 0.3, slabTile, Panel)
			b.wall(sx*(hx-0.45), 0, sx*(hx-0.45), 5, PlatformH, 1.0, 0.3, Panel) // open beyond, onto the mountain's saddle
			b.trim = false
			for _, x := range []float32{sx * 14.4, sx * (hx - 0.75)} {
				for _, z := range []float32{1, 4.5, 8.4} {
					b.column(x, z, 0, PlatformH-0.3, 0.6, Panel)
				}
			}
			b.stairs(false, 16.2, 9, min(sx*15.5, sx*18.5), max(sx*15.5, sx*18.5), 0, PlatformH, Panel)
		}
	})

	part("towers", func(b *builder) {
		const h = towerHalf
		for _, sx := range []float32{-1, 1} {
			x, z := sx*towerX, float32(towerZ)
			for _, c := range [][2]float32{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				b.column(x+c[0]*(h-0.3), z+c[1]*(h-0.3), 0, TopH-0.3, 0.6, Concrete)
			}
			// A wall on the outer side below for cover, and a parapet up top
			// on every side but the one the lift pad throws you over.
			b.wall(x+sx*(h-0.15), z-h+0.6, x+sx*(h-0.15), z+h-0.6, 0, 2.25, 0.3, Concrete)
			b.trim = true
			b.tiles(x-h, z-h, x+h, z+h, TopH, 0.3, 1.35, Concrete)
			b.wall(x-h, z+h-0.15, x+h, z+h-0.15, TopH, 1.1, 0.3, Concrete)
			b.wall(x+sx*(h-0.15), z-h, x+sx*(h-0.15), skyZ0, TopH, 1.1, 0.3, Concrete) // either side of the sky bridge
			b.wall(x+sx*(h-0.15), skyZ1, x+sx*(h-0.15), z+h-0.3, TopH, 1.1, 0.3, Concrete)
			b.trim = false
		}
	})

	part("sky bridges", func(b *builder) {
		for _, sx := range []float32{-1, 1} {
			// From the tower's top out over the ledge's stairs to the
			// mountain's lower terrace, resting on the rock at the far end.
			b.trim = true
			b.tiles(min(sx*(towerX+towerHalf), sx*21.5), skyZ0, max(sx*(towerX+towerHalf), sx*21.5), skyZ1, TopH, 0.3, slabTile, Panel)
			b.trim = false
			b.column(sx*16.5, (skyZ0+skyZ1)/2, 0, TopH-0.3, 0.6, Panel)
		}
	})
	return out
}

// The perch towers: one each side of the middle at each end.
const (
	towerX    = 9.0
	towerZ    = 18.5
	towerHalf = 2.0
	// liftPadOut is how far in front of a tower its lift pad is: far enough
	// that someone steering forward off it still clears the tower's edge.
	liftPadOut = 4.7
	// skyZ0..skyZ1 is the sky bridge from each tower to the mountain.
	skyZ0 = towerZ - 1
	skyZ1 = towerZ + 1
)

// keepClear are the parts of the south half's floor cover mustn't block:
// the keep, bridges, ledges, towers, stairs and pads, and where players
// land from the launch bays. (The north half mirrors it.)
var keepClear = [][2]mathx.Vec3{
	{{-14.5, 0, -1}, {14.5, 0, 2}},                      // bridges
	{{-9.5, 0, -1}, {9.5, 0, 14}},                       // the keep, its stairs and the jump pads
	{{-arenaHalfX, 0, -1}, {-arenaHalfX + 7.5, 0, 21}},  // west ledge, its stairs and the sky bridge's column
	{{arenaHalfX - 7.5, 0, -1}, {arenaHalfX, 0, 21}},    // east ...
	{{-towerX - 2.5, 0, 11}, {-towerX + 2.5, 0, 23}},    // towers, their lift pads and the pickup behind them
	{{towerX - 2.5, 0, 11}, {towerX + 2.5, 0, 23}},      //
	{{-6, 0, 14}, {6, 0, 23}},                           // landing zone
	{{-arenaHalfX, 0, 27}, {arenaHalfX, 0, arenaHalfZ}}, // along the end parapet
}

// cover places random destructible cover in the south half and a copy
// turned half round in the north half.
func (s *Site) cover(rng *rand.Rand) {
	type option struct {
		weight int
		build  func(*rand.Rand, mathx.Vec3, int) *Structure
	}
	options := []option{{5, LowWall}, {3, TallWall}, {3, LCover}, {2, Crates}, {1, Bunker}, {1, Glasshouse}}
	total := 0
	for _, o := range options {
		total += o.weight
	}
	var placed [][2]mathx.Vec3 // footprints so far (south half), grown by a margin
	want := 6 + rng.IntN(3)
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

// ramp is a slab of rock of the given half-width running from foot up to
// top, tilted to match. The slab is 0.4 m thick with its top surface on
// that line, and runs on a little past the foot, into the ground, so there's
// no lip there. (Past the top it would stick up above the level it meets.)
func ramp(foot, top mathx.Vec3, halfWidth float32) Block {
	const thick = 0.2 // half thickness
	run := top.Sub(foot)
	const past = 0.4
	length := run.Len() + past
	up := run.Normalize() // along the slope, uphill
	flat := mathx.Vec3{up[0], 0, up[2]}.Normalize()
	// Perpendicular to the slope, pointing up: cos(a)*Y - sin(a)*flat.
	normal := mathx.Vec3{0, 1, 0}.Scale(flat.Dot(up)).Sub(flat.Scale(up[1]))
	// Local axes: +Y the surface normal, +Z down the slope, +X across it
	// (X = Y x Z keeps the basis right-handed).
	down := up.Scale(-1)
	rot := mathx.QuatFromBasis(normal.Cross(down), normal, down)
	mid := foot.Add(top).Scale(0.5).Sub(normal.Scale(thick)).Sub(up.Scale(past / 2))
	return Block{Kind: Rock, Centre: mid, Half: mathx.Vec3{halfWidth, thick, length / 2}, Rotation: rot}
}

// beam is a girder of the given half width and height running straight
// from one point to another.
func beam(from, to mathx.Vec3, halfWidth, halfHeight float32) Block {
	along := to.Sub(from)
	fwd := along.Normalize()
	side := fwd.Cross(mathx.Vec3{0, 1, 0}).Normalize()
	up := side.Cross(fwd)
	// Local axes: +X across, +Y up the beam's depth, +Z along it (X = Y x Z
	// keeps the basis right-handed).
	rot := mathx.QuatFromBasis(up.Cross(fwd), up, fwd)
	return Block{Kind: Girder, Centre: from.Add(to).Scale(0.5), Half: mathx.Vec3{halfWidth, halfHeight, along.Len() / 2}, Rotation: rot}
}
