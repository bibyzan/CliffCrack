package game

import (
	"math"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/noise"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// The Arena hangs in a chasm: faceted rock walls like Run mode's
// mountainsides rise beyond each end, the launch bays jutting from them,
// and far below is water.
var (
	chasmRockColor = mathx.SRGB(0.93, 0.95, 1.0, 1) // snow on the ledges; the steep faces shade to rock
	waterColor     = mathx.SRGB(0.10, 0.27, 0.33, 1)
)

// chasmProfile is the cross-section of a chasm wall: how far from the
// middle (beyond arena.ChasmWall) the rock face is at each height. It leans
// in below the arena, stands near vertical beside it, then breaks back
// into mountainside above the rim.
var chasmProfile = [][2]float32{
	{-8, arena.WaterLevel - 25}, {-6, arena.WaterLevel}, {-4, -50}, {-2, -30}, {0, -15},
	{0, 0}, {1, 12}, {4, 25}, {9, 36}, {20, 44}, {45, 56}, {90, 82}, {160, 120}, {260, 150},
}

// rimRow is the first profile row above the chasm's rim: from there up the
// skyline gets its peaks.
const rimRow = 9

// chasmWallMesh is one side of the chasm: the wall beyond +Z (side 1) or
// -Z (side -1), 1.6 km along X. Columns crowd together near the arena and
// spread out into the haze; the face gets pushed back (never forward, so it
// stays clear of the launch bay) by ridged noise, and the mountains above
// the rim get ridged peaks.
func chasmWallMesh(field *noise.Field, side float32) geom.MeshData {
	const cols, sub = 181, 3 // sub rows per profile step
	rows := (len(chasmProfile)-1)*sub + 1
	offset := float64(side) * 17.3
	m := geom.Grid(cols, rows, func(i, j int) mathx.Vec3 {
		u := float64(i)/(cols-1)*2 - 1
		x := float32(math.Copysign(math.Pow(math.Abs(u), 1.7)*800, u))
		// Profile, interpolated between its rows.
		k, f := j/sub, float32(j%sub)/sub
		if k >= len(chasmProfile)-1 {
			k, f = len(chasmProfile)-2, 1
		}
		a, b := chasmProfile[k], chasmProfile[k+1]
		d := a[0] + (b[0]-a[0])*f
		y := a[1] + (b[1]-a[1])*f
		fx, fy := float64(x), float64(y)
		// Away from the arena the chasm wanders.
		wander := float32(field.At(fx/260+offset, 3.1)) * 40 * smooth((abs32(x)-40)/160)
		// Ridges and buttresses on the face, pushed back into the rock.
		face := float32(field.Ridged(fx/38+offset, fy/30, 4, 0.5)) * 9
		jitter := float32(field.At(fx/7+offset, fy/7)) * 1.2
		above := float32(k-rimRow) + f
		if above > 0 {
			// Mountains: ridges and peaks, different on every row back, so
			// the skyline is jagged rather than one smooth range.
			peaks := float32(field.Ridged(fx/90+offset, float64(above)*0.45, 5, 0.55))
			rise := min(above/2, 1)
			y += (peaks*80 - 20) * rise
			d += peaks * 25 * rise
			jitter *= 1 + above // coarser facets further back
		}
		z := side * (arena.ChasmWall + d + face + abs32(jitter) + wander) // never forward, into a bay
		return mathx.Vec3{x, y + jitter, z}
	})
	if side > 0 {
		m.FlipWinding() // face -Z, towards the arena
	}
	return m
}

// waterMesh is the water at the bottom of the chasm: a gently lumpy sheet,
// so its facets catch the light.
func waterMesh(field *noise.Field) geom.MeshData {
	const cols, rows = 121, 25
	const width, depth = 1800, 2 * (arena.ChasmWall + 60)
	return geom.Grid(cols, rows, func(i, j int) mathx.Vec3 {
		x := -width/2 + float32(i)*width/(cols-1)
		z := depth/2 - float32(j)*depth/(rows-1)
		swell := float32(field.At(float64(x)/9, float64(z)/9)) * 0.6
		return mathx.Vec3{x, arena.WaterLevel + swell, z}
	})
}

// cragMesh is the ridge along one side of the arena (side 1 beside +X,
// -1 beside -X). It rises out of the chasm just behind the mountain spur's
// pinnacle into a jagged crest, runs the length of the chasm and climbs at
// both ends into the chasm walls' mountains, burying itself in their rock,
// so the spur reads as a shoulder of one continuous range. It's scenery:
// out of reach.
func cragMesh(field *noise.Field, side float32) geom.MeshData {
	const cols, reach = 187, arena.ChasmWall + 12 // past the wall's face, into its rock
	profile := [][2]float32{                      // (out from x = 30, height as a fraction of the crest)
		{0, -3}, {1, -0.5}, {3, 0.45}, {5.5, 0.85}, {9, 1}, {14, 0.9}, {22, 0.6}, {32, 0.2}, {45, -0.6}, {60, -3},
	}
	const sub = 3 // rows per profile step, for facets as fine as the chasm walls'
	offset := float64(side) * 5.3
	m := geom.Grid(cols, (len(profile)-1)*sub+1, func(i, j int) mathx.Vec3 {
		z := -reach + 2*reach*float32(i)/(cols-1)
		// Jagged along the middle, climbing to the chasm walls' mountains
		// at the ends.
		crest := 20 + 16*float32(field.Ridged(float64(z)/15+offset, 2.3, 4, 0.55))
		ends := smooth((abs32(z) - 28) / (arena.ChasmWall - 28))
		crest += (46 - crest) * ends
		k, f := j/sub, float32(j%sub)/sub
		if k >= len(profile)-1 {
			k, f = len(profile)-2, 1
		}
		a, b := profile[k], profile[k+1]
		out, h := a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f
		y := h * crest
		// Buttresses and gullies, as on the chasm walls, pushed back from
		// the arena; and a jitter so every facet catches the light.
		face := float32(field.Ridged(float64(z)/9+offset, float64(y)/8, 3, 0.5)) * 4
		jitter := float32(field.At(float64(z)/2.3+offset, float64(j)*1.3))
		return mathx.Vec3{side * (30 + out + face + jitter*0.7), max(y+jitter*1.2, arena.WaterLevel-5), z}
	})
	if side < 0 {
		m.FlipWinding() // mirrored in X: keep it facing up
	}
	return m
}

// chasm holds the chasm's meshes.
type chasm struct {
	walls [2]render.Mesh
	crags [2]render.Mesh
	water render.Mesh
}

func newChasm() (*chasm, error) {
	field := noise.New(0xc4a5)
	c := &chasm{}
	var err error
	for i, side := range []float32{1, -1} {
		if c.walls[i], err = render.CreateMesh(chasmWallMesh(field, side)); err != nil {
			return nil, err
		}
		if c.crags[i], err = render.CreateMesh(cragMesh(field, side)); err != nil {
			return nil, err
		}
	}
	if c.water, err = render.CreateMesh(waterMesh(field)); err != nil {
		return nil, err
	}
	return c, nil
}

// appendDraws draws the chasm: its walls and water, and with crags the
// ridges along the arena's sides.
func (c *chasm) appendDraws(out []render.DrawCmd, crags bool) []render.DrawCmd {
	id := mathx.Translate(0, 0, 0)
	meshes := c.walls[:]
	if crags {
		meshes = append(meshes, c.crags[:]...)
	}
	for _, w := range meshes {
		out = append(out, render.DrawCmd{Model: id, Color: chasmRockColor, Flags: gfx.DrawFlat | gfx.DrawSnow, Mesh: w})
	}
	return append(out, render.DrawCmd{Model: id, Color: waterColor, Flags: gfx.DrawFlat, Mesh: c.water})
}

// rockCell is roughly how big the facets on a rock block are, in metres.
const rockCell = 2.0

// rockBlockMesh is a block of the mountain: a box (in its own frame, half
// extents half, placed at centre in the world) whose top is flat and exact,
// to walk on, and whose sides bulge out into rough facets, easing to flush
// just under the top edge so what you see matches where you can stand.
func rockBlockMesh(field *noise.Field, centre, half mathx.Vec3) geom.MeshData {
	var m geom.MeshData
	bulge := func(p mathx.Vec3) mathx.Vec3 {
		var out mathx.Vec3
		for _, k := range []int{0, 2} {
			if abs32(abs32(p[k])-half[k]) < 1e-3 {
				out[k] = float32(math.Copysign(1, float64(p[k])))
			}
		}
		if out == (mathx.Vec3{}) {
			return out
		}
		w := centre.Add(p)
		n := float32(field.At(float64(w[0]+w[2])/3.1, float64(w[1])/2.7)+field.At(float64(w[2]-w[0])/4.3+17, float64(w[1])/3.9)) * 0.5
		amount := (0.5 + 0.5*n) * 0.6 * smooth((half[1]-p[1])/1.5)
		// Under the arena's floor nobody walks: there it's a mountain's
		// roots, craggier the further down.
		if below := -w[1]; below > 0 {
			amount += (0.3 + 0.7*n*n) * min(below/4, 5)
		}
		return out.Scale(amount)
	}
	for k := 0; k < 3; k++ {
		for _, s := range []float32{1, -1} {
			a, b := (k+1)%3, (k+2)%3
			if s < 0 {
				a, b = b, a // keep the winding facing out
			}
			na := max(1, int(math.Ceil(float64(2*half[a]/rockCell))))
			nb := max(1, int(math.Ceil(float64(2*half[b]/rockCell))))
			base := uint32(len(m.Vertices))
			for j := 0; j <= nb; j++ {
				for i := 0; i <= na; i++ {
					var p mathx.Vec3
					p[k] = s * half[k]
					p[a] = -half[a] + 2*half[a]*float32(i)/float32(na)
					p[b] = -half[b] + 2*half[b]*float32(j)/float32(nb)
					m.Vertices = append(m.Vertices, geom.Vertex{Position: p.Add(bulge(p))})
				}
			}
			row := uint32(na + 1)
			for j := uint32(0); j < uint32(nb); j++ {
				for i := uint32(0); i < uint32(na); i++ {
					v := base + j*row + i
					m.Indices = append(m.Indices, v, v+1, v+row+1, v, v+row+1, v+row)
				}
			}
		}
	}
	m.ComputeNormals()
	return m
}
