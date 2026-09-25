// Package arena is the FPS arena mode's simulation: the level, the player's
// movement, the rifle and the drones. It is pure Go (no GPU), so the whole
// game can be stepped and tested headlessly; package game draws it.
package arena

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// BlockKind says what a block is, so the renderer can colour and trim it.
type BlockKind int

const (
	Floor BlockKind = iota
	Wall
	Pillar
	Cover
	Platform
	Ramp
)

// Block is a static box in the level.
type Block struct {
	Kind     BlockKind
	Centre   mathx.Vec3
	Half     mathx.Vec3
	Rotation mathx.Quat
}

// Arena dimensions (metres).
const (
	HalfSize   = 24  // the floor spans -HalfSize..HalfSize on X and Z
	WallHeight = 5.0 // perimeter walls
	PlatformH  = 2.5 // top of the centre platform
	LedgeH     = 2.0 // top of the side ledges
)

// Level is the arena's fixed layout: a walled square with a raised centre
// platform reached by two ramps, raised ledges on the east and west sides with
// ramps up from the middle, four tall pillars and scattered cover.
func Level() []Block {
	id := mathx.QuatIdentity()
	box := func(k BlockKind, c, h mathx.Vec3) Block { return Block{Kind: k, Centre: c, Half: h, Rotation: id} }
	yawed := func(k BlockKind, c, h mathx.Vec3, deg float32) Block {
		return Block{Kind: k, Centre: c, Half: h, Rotation: mathx.AxisAngle(mathx.Vec3{0, 1, 0}, deg*math.Pi/180)}
	}

	const hs, wh = HalfSize, WallHeight
	blocks := []Block{
		box(Floor, mathx.Vec3{0, -0.5, 0}, mathx.Vec3{hs + 1, 0.5, hs + 1}),
		box(Wall, mathx.Vec3{0, wh / 2, -hs - 0.5}, mathx.Vec3{hs + 1, wh / 2, 0.5}),
		box(Wall, mathx.Vec3{0, wh / 2, hs + 0.5}, mathx.Vec3{hs + 1, wh / 2, 0.5}),
		box(Wall, mathx.Vec3{-hs - 0.5, wh / 2, 0}, mathx.Vec3{0.5, wh / 2, hs + 1}),
		box(Wall, mathx.Vec3{hs + 0.5, wh / 2, 0}, mathx.Vec3{0.5, wh / 2, hs + 1}),

		// Centre platform, ramps up from the north and south.
		box(Platform, mathx.Vec3{0, PlatformH / 2, 0}, mathx.Vec3{4, PlatformH / 2, 4}),
		ramp(mathx.Vec3{0, 0, 11}, mathx.Vec3{0, PlatformH, 4}, 1.75),
		ramp(mathx.Vec3{0, 0, -11}, mathx.Vec3{0, PlatformH, -4}, 1.75),

		// East and west ledges along the walls, ramps up from the middle.
		box(Platform, mathx.Vec3{19, LedgeH / 2, 0}, mathx.Vec3{5, LedgeH / 2, 7}),
		box(Platform, mathx.Vec3{-19, LedgeH / 2, 0}, mathx.Vec3{5, LedgeH / 2, 7}),
		ramp(mathx.Vec3{9, 0, 0}, mathx.Vec3{14, LedgeH, 0}, 1.5),
		ramp(mathx.Vec3{-9, 0, 0}, mathx.Vec3{-14, LedgeH, 0}, 1.5),

		// Tall pillars break the long sight lines.
		box(Pillar, mathx.Vec3{10, wh / 2, 12}, mathx.Vec3{1, wh / 2, 1}),
		box(Pillar, mathx.Vec3{-10, wh / 2, 12}, mathx.Vec3{1, wh / 2, 1}),
		box(Pillar, mathx.Vec3{10, wh / 2, -12}, mathx.Vec3{1, wh / 2, 1}),
		box(Pillar, mathx.Vec3{-10, wh / 2, -12}, mathx.Vec3{1, wh / 2, 1}),

		// Waist-high cover to duck behind.
		yawed(Cover, mathx.Vec3{5, 0.6, 16}, mathx.Vec3{1.2, 0.6, 1.2}, 20),
		yawed(Cover, mathx.Vec3{-6, 0.6, 18}, mathx.Vec3{2.5, 0.6, 0.4}, 0),
		yawed(Cover, mathx.Vec3{-5, 0.6, -16}, mathx.Vec3{1.2, 0.6, 1.2}, -25),
		yawed(Cover, mathx.Vec3{6, 0.6, -18}, mathx.Vec3{2.5, 0.6, 0.4}, 0),
		yawed(Cover, mathx.Vec3{17, 0.6, 14}, mathx.Vec3{0.4, 0.6, 2.5}, 0),
		yawed(Cover, mathx.Vec3{-17, 0.6, -14}, mathx.Vec3{0.4, 0.6, 2.5}, 0),
		yawed(Cover, mathx.Vec3{16, 0.9, -17}, mathx.Vec3{1, 0.9, 1}, 35),
		yawed(Cover, mathx.Vec3{-16, 0.9, 17}, mathx.Vec3{1, 0.9, 1}, -35),
	}
	return blocks
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

// addLevel puts every block into the physics world as a static box.
func addLevel(w *physics.World, blocks []Block) {
	for _, b := range blocks {
		body := physics.NewBox(b.Half, physics.Static)
		body.Position, body.Rotation = b.Centre, b.Rotation
		body.Friction = 1
		body.Restitution = 0 // contacts use the bouncier body: the player lands dead, debris still bounces
		body.UserData = b.Kind
		if err := w.Add(body); err != nil {
			panic(err) // static boxes are always valid
		}
	}
}
