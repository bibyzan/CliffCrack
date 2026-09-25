package game

import (
	"image"
	"image/color"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// Arena mode's look: slate blocks at dusk, orange cover crates, and glowing
// cyan and orange trim along the edges so the layout reads at a glance.
var (
	arenaFloorColor    = mathx.SRGB(0.40, 0.42, 0.48, 1)
	arenaWallColor     = mathx.SRGB(0.46, 0.49, 0.58, 1)
	arenaPillarColor   = mathx.SRGB(0.34, 0.36, 0.44, 1)
	arenaPlatformColor = mathx.SRGB(0.55, 0.57, 0.64, 1)
	arenaRampColor     = mathx.SRGB(0.60, 0.56, 0.50, 1)
	arenaCoverColor    = mathx.SRGB(0.86, 0.50, 0.22, 1)
	arenaTrimCyan      = mathx.SRGB(0.25, 0.92, 1.00, 1)
	arenaTrimOrange    = mathx.SRGB(1.00, 0.58, 0.18, 1)
	arenaSkyZenith     = mathx.SRGB(0.10, 0.14, 0.32, 1)
	arenaHaze          = srgb3(0.52, 0.44, 0.58)
	arenaSun           = srgb3(1.0, 0.74, 0.52)
	arenaShade         = srgb3(0.42, 0.50, 0.72)
	arenaSunDir        = mathx.Vec3{-0.55, 0.42, -0.62}

	droneColor     = mathx.SRGB(0.30, 0.30, 0.34, 1)
	droneEyeColor  = mathx.SRGB(1.00, 0.18, 0.12, 1)
	gunMetal       = mathx.SRGB(0.18, 0.19, 0.22, 1)
	gunBlack       = mathx.SRGB(0.08, 0.08, 0.09, 1)
	tracerColor    = mathx.SRGB(1.00, 0.86, 0.45, 1)
	flashColor     = mathx.SRGB(1.00, 0.70, 0.30, 1)
	holeColor      = mathx.SRGB(0.05, 0.05, 0.06, 1)
	explosionColor = mathx.SRGB(1.00, 0.55, 0.20, 1)
)

// arenaAssets are the GPU resources for Arena mode, built once.
type arenaAssets struct {
	blocks []render.DrawCmd // the level, including trim; it never moves
	cube   render.Mesh      // unit cube, 2 across (half extents 1)
	floor  render.Texture
	panel  render.Texture
}

func newArenaAssets(level []arena.Block) (*arenaAssets, error) {
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
	for _, b := range level {
		tile, tex, col := float32(2), as.panel, arenaWallColor
		switch b.Kind {
		case arena.Floor:
			tile, tex, col = 4, as.floor, arenaFloorColor
		case arena.Pillar:
			col = arenaPillarColor
		case arena.Platform:
			col = arenaPlatformColor
		case arena.Ramp:
			col = arenaRampColor
		case arena.Cover:
			tile, col = 1.2, arenaCoverColor
		}
		mesh, err := render.CreateMesh(boxMesh(b.Half, tile))
		if err != nil {
			return nil, err
		}
		model := mathx.Translate(b.Centre[0], b.Centre[1], b.Centre[2]).Mul(b.Rotation.Mat4())
		as.blocks = append(as.blocks, render.DrawCmd{Model: model, Color: col, Texture: tex, Mesh: mesh})
		as.blocks = as.appendTrim(as.blocks, b)
	}
	return as, nil
}

// appendTrim adds glowing edge bands: orange along the top of the perimeter
// walls, cyan around the tops of platforms and a ring round each pillar.
func (as *arenaAssets) appendTrim(out []render.DrawCmd, b arena.Block) []render.DrawCmd {
	band := func(centre, half mathx.Vec3, c [4]float32) []render.DrawCmd {
		m := mathx.Translate(centre[0], centre[1], centre[2]).Mul(b.Rotation.Mat4()).
			Mul(mathx.Scale(half[0], half[1], half[2]))
		return append(out, render.DrawCmd{Model: m, Color: c, Flags: gfx.DrawUnlit, Mesh: as.cube})
	}
	c, h := b.Centre, b.Half
	switch b.Kind {
	case arena.Wall:
		return band(mathx.Vec3{c[0], c[1] + h[1] - 0.12, c[2]}, mathx.Vec3{h[0] + 0.01, 0.06, h[2] + 0.01}, arenaTrimOrange)
	case arena.Platform:
		return band(mathx.Vec3{c[0], c[1] + h[1] - 0.07, c[2]}, mathx.Vec3{h[0] + 0.015, 0.05, h[2] + 0.015}, arenaTrimCyan)
	case arena.Pillar:
		return band(mathx.Vec3{c[0], 2.2, c[2]}, mathx.Vec3{h[0] + 0.015, 0.07, h[2] + 0.015}, arenaTrimCyan)
	}
	return out
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

// gunPart is one box of the first-person rifle, in gun space (metres; the
// barrel points down -Z).
type gunPart struct {
	centre, half mathx.Vec3
	color        [4]float32
	flags        gfx.DrawFlags
}

var gunParts = []gunPart{
	{mathx.Vec3{0, 0, 0}, mathx.Vec3{0.034, 0.048, 0.17}, gunMetal, 0},                          // receiver
	{mathx.Vec3{0, 0.012, -0.29}, mathx.Vec3{0.011, 0.011, 0.13}, gunBlack, 0},                  // barrel
	{mathx.Vec3{0, 0.002, -0.21}, mathx.Vec3{0.028, 0.034, 0.075}, uiAccent, 0},                 // handguard
	{mathx.Vec3{0, -0.085, -0.03}, mathx.Vec3{0.021, 0.06, 0.032}, gunBlack, 0},                 // magazine
	{mathx.Vec3{0, -0.012, 0.21}, mathx.Vec3{0.024, 0.042, 0.07}, gunMetal, 0},                  // stock
	{mathx.Vec3{0, 0.062, -0.02}, mathx.Vec3{0.012, 0.016, 0.045}, gunBlack, 0},                 // sight
	{mathx.Vec3{0, -0.07, 0.085}, mathx.Vec3{0.019, 0.05, 0.024}, gunBlack, 0},                  // grip
	{mathx.Vec3{0, 0.08, -0.02}, mathx.Vec3{0.004, 0.004, 0.004}, arenaTrimCyan, gfx.DrawUnlit}, // sight dot (glows)
}

// gunMuzzle is the barrel's tip in gun space.
var gunMuzzle = mathx.Vec3{0, 0.012, -0.43}
