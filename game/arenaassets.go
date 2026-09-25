package game

import (
	"image"
	"image/color"
	"math/rand/v2"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/noise"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// Arena's look, after Halo 5's Breakout: a clean white simulation space
// outlined in glowing trim, each end in its team's colour, with the
// structures in their materials' colours, on a dark steel frame.
var (
	arenaFloorColor    = mathx.SRGB(0.74, 0.76, 0.80, 1)
	arenaWallColor     = mathx.SRGB(0.90, 0.91, 0.93, 1)
	arenaPlatformColor = mathx.SRGB(0.95, 0.95, 0.96, 1)
	arenaBayColor      = mathx.SRGB(0.50, 0.53, 0.58, 1)
	arenaDeckColor     = mathx.SRGB(0.66, 0.69, 0.74, 1)
	girderColor        = mathx.SRGB(0.28, 0.30, 0.34, 1)
	arenaTrimCyan      = mathx.SRGB(0.25, 0.92, 1.00, 1)

	grenadeGlow    = mathx.SRGB(1.00, 0.18, 0.12, 1)
	hurtColor      = mathx.SRGB(0.85, 0.04, 0.04, 1)
	gunMetal       = mathx.SRGB(0.18, 0.19, 0.22, 1)
	gunBlack       = mathx.SRGB(0.08, 0.08, 0.09, 1)
	launcherGreen  = mathx.SRGB(0.27, 0.34, 0.23, 1)
	hammerHandle   = mathx.SRGB(0.55, 0.40, 0.24, 1)
	tracerColor    = mathx.SRGB(1.00, 0.86, 0.45, 1)
	flashColor     = mathx.SRGB(1.00, 0.70, 0.30, 1)
	holeColor      = mathx.SRGB(0.05, 0.05, 0.06, 1)
	explosionColor = mathx.SRGB(1.00, 0.55, 0.20, 1)
	smokeColor     = mathx.SRGB(0.35, 0.33, 0.32, 1)
)

// materialColor is each structure material's base tint (linear RGBA; glass
// is translucent).
var materialColor = [...][4]float32{
	arena.Wood:     mathx.SRGB(0.66, 0.47, 0.30, 1),
	arena.Brick:    mathx.SRGB(0.70, 0.36, 0.27, 1),
	arena.Concrete: mathx.SRGB(0.66, 0.66, 0.64, 1),
	arena.Glass:    mathx.SRGB(0.62, 0.84, 0.95, 0.32),
	arena.Metal:    mathx.SRGB(0.48, 0.50, 0.56, 1),
	arena.Panel:    arenaPlatformColor,
	arena.Plate:    arenaFloorColor,
}

// dustColor is the puff a material gives off when it breaks.
var dustColor = [...][4]float32{
	arena.Wood:     mathx.SRGB(0.60, 0.48, 0.35, 1),
	arena.Brick:    mathx.SRGB(0.72, 0.52, 0.45, 1),
	arena.Concrete: mathx.SRGB(0.78, 0.77, 0.74, 1),
	arena.Glass:    mathx.SRGB(0.85, 0.95, 1.00, 1),
	arena.Metal:    mathx.SRGB(0.95, 0.80, 0.50, 1),
	arena.Panel:    mathx.SRGB(0.92, 0.93, 0.95, 1),
	arena.Plate:    mathx.SRGB(0.80, 0.81, 0.84, 1),
}

// arenaAssets are the Arena's GPU resources, built once.
type arenaAssets struct {
	cube      render.Mesh // unit cube, 2 across (half extents 1)
	floor     render.Texture
	panel     render.Texture
	materials [len(materialColor)]render.Texture
	chasm     *chasm
}

func newArenaAssets() (*arenaAssets, error) {
	as := &arenaAssets{}
	var err error
	if as.cube, err = render.CreateMesh(geom.Cube(2)); err != nil {
		return nil, err
	}
	if as.floor, err = render.CreateTexture(tileTexture(128, 32, 0.82, 0.5), true); err != nil {
		return nil, err
	}
	if as.panel, err = render.CreateTexture(tileTexture(64, 64, 0.9, 0.6), true); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(21, 22))
	textures := []struct {
		m   arena.Material
		img *image.NRGBA
	}{
		{arena.Wood, woodTexture(rng)},
		{arena.Brick, brickTexture(rng)},
		{arena.Concrete, speckleTexture(rng, 0.9, 0.12)},
		{arena.Metal, brushedTexture(rng)},
	}
	for _, t := range textures {
		if as.materials[t.m], err = render.CreateTexture(t.img, true); err != nil {
			return nil, err
		}
	}
	as.materials[arena.Panel] = as.panel
	as.materials[arena.Plate] = as.floor
	if as.chasm, err = newChasm(); err != nil {
		return nil, err
	}
	return as, nil // glass uses the white texture (handle 0)
}

// levelDraws builds the draws (and meshes) for a level's indestructible
// blocks: white clean-sim walls, a grid floor, darker launch bays, the
// steel girders and the mountains' rock. Their glowing trim comes from trimDraws.
func (as *arenaAssets) levelDraws(level []arena.Block) ([]render.DrawCmd, []render.Mesh, error) {
	var draws []render.DrawCmd
	var meshes []render.Mesh
	rock := noise.New(0x70c4)
	for _, b := range level {
		if b.Kind == arena.Rock {
			mesh, err := render.CreateMesh(rockBlockMesh(rock, b.Centre, b.Half))
			if err != nil {
				return nil, meshes, err
			}
			meshes = append(meshes, mesh)
			model := mathx.Translate(b.Centre[0], b.Centre[1], b.Centre[2]).Mul(b.Rotation.Mat4())
			draws = append(draws, render.DrawCmd{Model: model, Color: chasmRockColor, Flags: gfx.DrawFlat | gfx.DrawSnow, Mesh: mesh})
			continue
		}
		tile, tex, col := float32(2), as.panel, arenaWallColor
		switch b.Kind {
		case arena.Floor:
			tile, tex, col = 4, as.floor, arenaFloorColor
		case arena.Deck:
			tile, tex, col = 4, as.floor, arenaDeckColor
		case arena.Bay:
			col = arenaBayColor
		case arena.Girder:
			tile, tex, col = 1.5, as.materials[arena.Metal], girderColor
		}
		mesh, err := render.CreateMesh(boxMesh(b.Half, tile))
		if err != nil {
			return nil, meshes, err
		}
		meshes = append(meshes, mesh)
		model := mathx.Translate(b.Centre[0], b.Centre[1], b.Centre[2]).Mul(b.Rotation.Mat4())
		draws = append(draws, render.DrawCmd{Model: model, Color: col, Texture: tex, Mesh: mesh})
	}
	return draws, meshes, nil
}

// trimDraws are the glowing bands that outline the level's blocks: along
// the tops of walls and the lips of the launch bays. Each end glows in the
// colour of the player who starts there (south, north); the middle is
// neutral. (The destructible parts carry their own trim: see trimColor.)
func (as *arenaAssets) trimDraws(level []arena.Block, south, north [4]float32) []render.DrawCmd {
	var out []render.DrawCmd
	for _, b := range level {
		col := trimColor(b.Centre, south, north)
		// band adds a strip at local (offset, half) in the block's frame.
		band := func(offset, half mathx.Vec3) {
			c := b.Centre
			m := mathx.Translate(c[0], c[1], c[2]).Mul(b.Rotation.Mat4()).
				Mul(mathx.Translate(offset[0], offset[1], offset[2])).Mul(mathx.Scale(half[0], half[1], half[2]))
			out = append(out, render.DrawCmd{Model: m, Color: col, Flags: gfx.DrawUnlit, Mesh: as.cube})
		}
		h := b.Half
		switch b.Kind {
		case arena.Deck:
			// A strip along the lip facing the arena.
			lip := -h[2] + 0.05
			if b.Centre[2] < 0 {
				lip = -lip
			}
			band(mathx.Vec3{0, h[1] - 0.05, lip}, mathx.Vec3{h[0] + 0.01, 0.05, 0.06})
		case arena.Wall, arena.Bay:
			band(mathx.Vec3{0, h[1] - 0.1, 0}, mathx.Vec3{h[0] + 0.012, 0.05, h[2] + 0.012})
			if b.Kind == arena.Wall && h[1] > 2 {
				band(mathx.Vec3{0, -h[1] + 0.9, 0}, mathx.Vec3{h[0] + 0.012, 0.025, h[2] + 0.012})
			}
		}
	}
	return out
}

// trimColor is the glow for something at p: the colour of whoever starts at
// that end, or neutral in the middle.
func trimColor(p mathx.Vec3, south, north [4]float32) [4]float32 {
	switch {
	case p[2] > 3:
		return south
	case p[2] < -3:
		return north
	}
	return arenaTrimCyan
}

// boxMesh is a box with the given half extents and texture coordinates
// projected per face in metres / tile, so textures keep their scale however
// big the block is.
func boxMesh(half mathx.Vec3, tile float32) geom.MeshData {
	m := geom.Cube(2)
	for i := range m.Vertices {
		v := &m.Vertices[i]
		p := mathx.Vec3{v.Position[0] * half[0], v.Position[1] * half[1], v.Position[2] * half[2]}
		v.Position = p
		switch n := v.Normal; {
		case n[0] > 0.5 || n[0] < -0.5:
			v.UV = [2]float32{p[2] / tile, -p[1] / tile}
		case n[1] > 0.5 || n[1] < -0.5:
			v.UV = [2]float32{p[0] / tile, p[2] / tile}
		default:
			v.UV = [2]float32{p[0] / tile, -p[1] / tile}
		}
	}
	return m
}

// tileTexture is a light square with darker grout lines every cell pixels.
func tileTexture(size, cell int, face, line float32) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f, l := uint8(face*255), uint8(line*255)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			v := f
			if x%cell < 2 || y%cell < 2 {
				v = l
			} else if (x/4+y/4)%9 == 0 { // a little speckle so large faces aren't flat
				v = f - 8
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}

func grey(img *image.NRGBA, x, y int, v float32) {
	b := uint8(clampf(v, 0, 1) * 255)
	img.SetNRGBA(x, y, color.NRGBA{b, b, b, 255})
}

// brickTexture is four courses of bricks with mortar, alternate courses offset.
func brickTexture(rng *rand.Rand) *image.NRGBA {
	const size, course, brick = 64, 16, 32
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	shade := make([]float32, 16)
	for i := range shade {
		shade[i] = 0.78 + 0.22*rng.Float32()
	}
	for y := 0; y < size; y++ {
		row := y / course
		for x := 0; x < size; x++ {
			xo := x + (row%2)*brick/2
			if y%course < 2 || xo%brick < 2 {
				grey(img, x, y, 0.95) // mortar, lighter than the brick
				continue
			}
			grey(img, x, y, shade[(row*4+xo/brick)%len(shade)]-0.05*rng.Float32())
		}
	}
	return img
}

// woodTexture is vertical planks with grain.
func woodTexture(rng *rand.Rand) *image.NRGBA {
	const size, plank = 64, 16
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for x := 0; x < size; x++ {
		base := 0.8 + 0.15*rng.Float32()
		for y := 0; y < size; y++ {
			if x%plank == 0 {
				grey(img, x, y, 0.45)
				continue
			}
			grain := 0.06 * float32((x*7+y/5)%5) / 5
			grey(img, x, y, base-grain)
		}
	}
	return img
}

// speckleTexture is a noisy flat surface (concrete).
func speckleTexture(rng *rand.Rand, base, amount float32) *image.NRGBA {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			grey(img, x, y, base-amount*rng.Float32())
		}
	}
	return img
}

// brushedTexture is metal with fine horizontal streaks.
func brushedTexture(rng *rand.Rand) *image.NRGBA {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		row := 0.85 + 0.1*rng.Float32()
		for x := 0; x < size; x++ {
			grey(img, x, y, row-0.04*rng.Float32())
		}
	}
	return img
}

// gunPart is one box of a first-person weapon model, in weapon space
// (metres; the barrel points down -Z).
type gunPart struct {
	centre, half mathx.Vec3
	color        [4]float32
	flags        gfx.DrawFlags
}

var rifleParts = []gunPart{
	{mathx.Vec3{0, 0, 0}, mathx.Vec3{0.034, 0.048, 0.17}, gunMetal, 0},                          // receiver
	{mathx.Vec3{0, 0.012, -0.29}, mathx.Vec3{0.011, 0.011, 0.13}, gunBlack, 0},                  // barrel
	{mathx.Vec3{0, 0.002, -0.21}, mathx.Vec3{0.028, 0.034, 0.075}, uiAccent, 0},                 // handguard
	{mathx.Vec3{0, -0.085, -0.03}, mathx.Vec3{0.021, 0.06, 0.032}, gunBlack, 0},                 // magazine
	{mathx.Vec3{0, -0.012, 0.21}, mathx.Vec3{0.024, 0.042, 0.07}, gunMetal, 0},                  // stock
	{mathx.Vec3{0, 0.062, -0.02}, mathx.Vec3{0.012, 0.016, 0.045}, gunBlack, 0},                 // sight
	{mathx.Vec3{0, -0.07, 0.085}, mathx.Vec3{0.019, 0.05, 0.024}, gunBlack, 0},                  // grip
	{mathx.Vec3{0, 0.08, -0.02}, mathx.Vec3{0.004, 0.004, 0.004}, arenaTrimCyan, gfx.DrawUnlit}, // sight dot (glows)
}

// rifleMuzzle is the barrel's tip in weapon space.
var rifleMuzzle = mathx.Vec3{0, 0.012, -0.43}

var launcherParts = []gunPart{
	{mathx.Vec3{0, 0, -0.08}, mathx.Vec3{0.055, 0.055, 0.26}, launcherGreen, 0},                  // tube
	{mathx.Vec3{0, 0, -0.345}, mathx.Vec3{0.064, 0.064, 0.02}, gunBlack, 0},                      // muzzle ring
	{mathx.Vec3{0, -0.09, 0.03}, mathx.Vec3{0.02, 0.06, 0.028}, gunBlack, 0},                     // grip
	{mathx.Vec3{0, -0.075, -0.2}, mathx.Vec3{0.018, 0.045, 0.03}, gunBlack, 0},                   // fore grip
	{mathx.Vec3{0, 0.07, -0.05}, mathx.Vec3{0.014, 0.018, 0.05}, gunMetal, 0},                    // sight
	{mathx.Vec3{0, 0.05, 0.16}, mathx.Vec3{0.04, 0.03, 0.05}, uiAccent, 0},                       // shell box
	{mathx.Vec3{0, 0.092, -0.05}, mathx.Vec3{0.005, 0.005, 0.005}, arenaTrimCyan, gfx.DrawUnlit}, // sight dot
}

// The sledgehammer, handle along +Y, the head's striking face towards -Z.
var hammerParts = []gunPart{
	{mathx.Vec3{0, 0.05, 0}, mathx.Vec3{0.018, 0.42, 0.018}, hammerHandle, 0},   // handle
	{mathx.Vec3{0, -0.3, 0}, mathx.Vec3{0.022, 0.09, 0.022}, gunBlack, 0},       // grip wrap
	{mathx.Vec3{0, 0.52, -0.02}, mathx.Vec3{0.065, 0.065, 0.15}, gunMetal, 0},   // head
	{mathx.Vec3{0, 0.52, -0.175}, mathx.Vec3{0.072, 0.072, 0.012}, uiAccent, 0}, // striking face
}
