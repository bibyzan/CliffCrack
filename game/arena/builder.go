package arena

import (
	"math"
	"math/rand/v2"

	"CliffCrack/engine/mathx"
)

// Chunk sizes: walls are cut into roughly 1 m x 0.75 m panels, slabs into
// 1.5 m tiles, so a rifle chips a panel, a hammer smashes a hole and a
// grenade takes out a section.
const (
	panelWidth  = 1.0
	panelHeight = 0.75
	slabTile    = 1.5
	storey      = 3.0 // floor-to-floor height
	glassThick  = 0.06
)

// builder lays out a structure in its own frame (X right, Z towards the
// front, Y up, ground at 0), placed in the world at origin turned by a
// multiple of 90 degrees, so every chunk stays an axis-aligned box.
type builder struct {
	origin mathx.Vec3
	turns  int // quarter turns about Y
	s      *Structure
}

func newBuilder(name string, origin mathx.Vec3, turns int) *builder {
	return &builder{origin: origin, turns: ((turns % 4) + 4) % 4, s: &Structure{Name: name}}
}

// box adds a chunk spanning lo..hi in the builder's frame.
func (b *builder) box(lo, hi mathx.Vec3, m Material) {
	c := lo.Add(hi).Scale(0.5)
	h := hi.Sub(lo).Scale(0.5)
	for range b.turns { // (x, z) -> (z, -x) per quarter turn; half extents swap
		c = mathx.Vec3{c[2], c[1], -c[0]}
		h = mathx.Vec3{h[2], h[1], h[0]}
	}
	b.s.Chunks = append(b.s.Chunks, &Chunk{Centre: c.Add(b.origin), Half: h, Mat: m})
}

// opening is a hole in a wall in wall coordinates: u metres along the wall
// from its start, v metres up from its base. A glazed opening gets glass.
type opening struct {
	u0, u1, v0, v1 float32
	glass          bool
}

// wall is a straight wall along X or Z, centred on the line from (x0, z0) to
// (x1, z1) at the given thickness, from base up by height, cut into panels.
func (b *builder) wall(x0, z0, x1, z1, base, height, thick float32, m Material, holes ...opening) {
	alongX := z0 == z1
	length := abs(x1-x0) + abs(z1-z0)
	nu := max(1, int(math.Round(float64(length/panelWidth))))
	nv := max(1, int(math.Round(float64(height/panelHeight))))
	du, dv := length/float32(nu), height/float32(nv)
	for iu := 0; iu < nu; iu++ {
		for iv := 0; iv < nv; iv++ {
			u0, u1 := float32(iu)*du, float32(iu+1)*du
			v0, v1 := float32(iv)*dv, float32(iv+1)*dv
			mat, t := m, thick
			skip := false
			for _, o := range holes {
				cu, cv := (u0+u1)/2, (v0+v1)/2
				if cu > o.u0 && cu < o.u1 && cv > o.v0 && cv < o.v1 {
					if o.glass {
						mat, t = Glass, glassThick
					} else {
						skip = true
					}
				}
			}
			if skip {
				continue
			}
			if alongX {
				xa, xb := min(x0, x1)+u0, min(x0, x1)+u1
				b.box(mathx.Vec3{xa, base + v0, z0 - t/2}, mathx.Vec3{xb, base + v1, z0 + t/2}, mat)
			} else {
				za, zb := min(z0, z1)+u0, min(z0, z1)+u1
				b.box(mathx.Vec3{x0 - t/2, base + v0, za}, mathx.Vec3{x0 + t/2, base + v1, zb}, mat)
			}
		}
	}
}

// slab is a floor or roof between (x0, z0) and (x1, z1) with its top at y.
func (b *builder) slab(x0, z0, x1, z1, y, thick float32, m Material) {
	nx := max(1, int(math.Round(float64((x1-x0)/slabTile))))
	nz := max(1, int(math.Round(float64((z1-z0)/slabTile))))
	dx, dz := (x1-x0)/float32(nx), (z1-z0)/float32(nz)
	for i := 0; i < nx; i++ {
		for j := 0; j < nz; j++ {
			b.box(mathx.Vec3{x0 + float32(i)*dx, y - thick, z0 + float32(j)*dz},
				mathx.Vec3{x0 + float32(i+1)*dx, y, z0 + float32(j+1)*dz}, m)
		}
	}
}

// column is a square post of the given width from base up by height.
func (b *builder) column(x, z, base, height, width float32, m Material) {
	n := max(1, int(math.Round(float64(height/panelHeight))))
	dh := height / float32(n)
	for i := 0; i < n; i++ {
		b.box(mathx.Vec3{x - width/2, base + float32(i)*dh, z - width/2},
			mathx.Vec3{x + width/2, base + float32(i+1)*dh, z + width/2}, m)
	}
}

func (b *builder) finish() *Structure {
	b.s.link()
	return b.s
}

// windowsAlong returns glazed openings spaced along a wall of the given
// length, sill at 1 m and head at 2.25 m above the storey's base.
func windowsAlong(length float32, skipMiddle bool) []opening {
	var out []opening
	n := int(length / 3)
	for i := 0; i < n; i++ {
		c := length * (float32(i) + 0.5) / float32(n)
		if skipMiddle && abs(c-length/2) < 1.2 {
			continue // leave room for the door
		}
		out = append(out, opening{c - 0.6, c + 0.6, 1.0, 2.25, true})
	}
	return out
}

// House is a one- or two-storey building with a front door, glazed windows,
// floors and a flat roof.
func House(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	w := float32(6 + 2*rng.IntN(3)) // 6, 8 or 10 m wide
	d := float32(5 + rng.IntN(3))   // 5 to 7 m deep
	floors := 1 + rng.IntN(2)
	walls := []Material{Wood, Brick, Concrete}[rng.IntN(3)]
	floorMat := Concrete
	if walls == Wood {
		floorMat = Wood
	}
	const t = 0.3
	b := newBuilder("house", origin, turns)
	hw, hd := w/2, d/2
	// Front and back walls run past the side walls' centre lines so the
	// corners close; the side walls fit between them.
	long := w + t
	for f := 0; f < floors; f++ {
		base := float32(f) * storey
		front := windowsAlong(long, f == 0)
		if f == 0 {
			front = append(front, opening{long/2 - 0.6, long/2 + 0.6, 0, 2.2, false}) // door
		}
		b.wall(-hw-t/2, hd, hw+t/2, hd, base, storey, t, walls, front...)                       // front (+Z)
		b.wall(-hw-t/2, -hd, hw+t/2, -hd, base, storey, t, walls, windowsAlong(long, false)...) // back
		b.wall(-hw, -hd+t/2, -hw, hd-t/2, base, storey, t, walls, windowsAlong(d-t, false)...)  // left
		b.wall(hw, -hd+t/2, hw, hd-t/2, base, storey, t, walls, windowsAlong(d-t, false)...)    // right
		b.slab(-hw+t/2, -hd+t/2, hw-t/2, hd-t/2, float32(f+1)*storey, 0.25, floorMat)           // ceiling / roof
	}
	return b.finish()
}

// Tower is a three-storey concrete frame: corner columns, floors, a parapet
// and a couple of partial walls per storey.
func Tower(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	const size, col = 6.0, 0.6
	const h = size / 2
	b := newBuilder("tower", origin, turns)
	for f := 0; f < 3; f++ {
		base := float32(f) * storey
		for _, c := range [][2]float32{{-h + col/2, -h + col/2}, {h - col/2, -h + col/2}, {-h + col/2, h - col/2}, {h - col/2, h - col/2}} {
			b.column(c[0], c[1], base, storey-0.3, col, Concrete)
		}
		// Floor slab spans the whole footprint, sitting on the columns.
		b.slab(-h, -h, h, h, base+storey, 0.3, Concrete)
		// A partial wall on one or two sides, between the columns.
		sides := 1 + rng.IntN(2)
		for s := 0; s < sides; s++ {
			switch (f + s + rng.IntN(2)) % 4 {
			case 0:
				b.wall(-h+col, -h+0.15, h-col, -h+0.15, base, storey-0.3, 0.3, Concrete, windowsAlong(size-2*col, false)...)
			case 1:
				b.wall(-h+col, h-0.15, h-col, h-0.15, base, storey-0.3, 0.3, Concrete, windowsAlong(size-2*col, false)...)
			case 2:
				b.wall(-h+0.15, -h+col, -h+0.15, h-col, base, storey-0.3, 0.3, Brick)
			case 3:
				b.wall(h-0.15, -h+col, h-0.15, h-col, base, storey-0.3, 0.3, Brick)
			}
		}
	}
	// Parapet round the roof.
	top := float32(3 * storey)
	b.wall(-h, -h+0.1, h, -h+0.1, top, 1.0, 0.2, Concrete)
	b.wall(-h, h-0.1, h, h-0.1, top, 1.0, 0.2, Concrete)
	return b.finish()
}

// Bunker is a low, thick-walled concrete box with a glazed slit and a door.
func Bunker(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	w := float32(6 + rng.IntN(3))
	const d, t, height = 5.0, 0.6, 2.4
	hw, hd := w/2, float32(d/2)
	b := newBuilder("bunker", origin, turns)
	slit := opening{0.8, w - 0.8, 1.5, 1.95, true}
	b.wall(-hw, hd, hw, hd, 0, height, t, Concrete, opening{hw - 0.6, hw + 0.6, 0, 2.0, false}, slit)
	b.wall(-hw, -hd, hw, -hd, 0, height, t, Concrete, slit)
	b.wall(-hw, -hd+t/2, -hw, hd-t/2, 0, height, t, Concrete)
	b.wall(hw, -hd+t/2, hw, hd-t/2, 0, height, t, Concrete)
	b.slab(-hw-t/2, -hd-t/2, hw+t/2, hd+t/2, height+0.4, 0.4, Concrete) // roof over the walls
	return b.finish()
}

// Glasshouse is a metal frame with glass walls and roof: it shatters.
func Glasshouse(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	w := float32(5 + rng.IntN(2))
	const d, height, post = 4.0, 2.6, 0.2
	hw, hd := w/2, float32(d/2)
	b := newBuilder("glasshouse", origin, turns)
	for _, c := range [][2]float32{{-hw, -hd}, {hw, -hd}, {-hw, hd}, {hw, hd}} {
		b.column(c[0], c[1], 0, height, post, Metal)
	}
	all := opening{-1, 100, -1, 100, true}
	b.wall(-hw+post/2, hd, hw-post/2, hd, 0, height, glassThick, Glass, all)
	b.wall(-hw+post/2, -hd, hw-post/2, -hd, 0, height, glassThick, Glass, all)
	b.wall(-hw, -hd+post/2, -hw, hd-post/2, 0, height, glassThick, Glass, all)
	b.wall(hw, -hd+post/2, hw, hd-post/2, 0, height, glassThick, Glass, all)
	b.slab(-hw-post/2, -hd-post/2, hw+post/2, hd+post/2, height+0.06, 0.06, Glass)
	return b.finish()
}

// Walls is a few freestanding walls: cover to shoot through or smash.
func Walls(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	b := newBuilder("walls", origin, turns)
	m := []Material{Wood, Brick, Concrete}[rng.IntN(3)]
	height := float32(2 + rng.IntN(2))
	l := float32(5 + rng.IntN(4))
	b.wall(-l/2, -2, l/2, -2, 0, height, 0.3, m)                                       // long wall
	b.wall(-l/2-0.15, -2+0.15, -l/2-0.15, 2, 0, height, 0.3, m)                        // return, an L
	b.wall(1, 3, 1+l*0.6, 3, 0, height*0.6, 0.3, []Material{Wood, Brick}[rng.IntN(2)]) // a low wall in front
	return b.finish()
}

// Crates is a small stack of wooden crates.
func Crates(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	b := newBuilder("crates", origin, turns)
	n := 3 + rng.IntN(4)
	for i := 0; i < n; i++ {
		x := float32(i%3) * 1.05
		y := float32(i/3) * 1.0
		b.box(mathx.Vec3{x, y, 0}, mathx.Vec3{x + 1, y + 1, 1}, Wood)
	}
	return b.finish()
}

// coverMaterial picks wood, brick or concrete.
func coverMaterial(rng *rand.Rand) Material { return []Material{Wood, Brick, Concrete}[rng.IntN(3)] }

// LowWall is a waist-high wall: cover you can shoot over, or hop.
func LowWall(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	b := newBuilder("low wall", origin, turns)
	l := float32(3 + rng.IntN(3))
	b.wall(-l/2, 0, l/2, 0, 0, 1.2, 0.35, coverMaterial(rng))
	return b.finish()
}

// TallWall is a head-high wall, sometimes with a window to shoot through.
func TallWall(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	b := newBuilder("tall wall", origin, turns)
	l := float32(3 + rng.IntN(3))
	var holes []opening
	if rng.IntN(2) == 0 {
		holes = append(holes, opening{l/2 - 0.6, l/2 + 0.6, 1.2, 1.95, rng.IntN(2) == 0})
	}
	b.wall(-l/2, 0, l/2, 0, 0, 2.6, 0.3, coverMaterial(rng), holes...)
	return b.finish()
}

// LCover is two walls meeting in an L, one waist-high and one head-high.
func LCover(rng *rand.Rand, origin mathx.Vec3, turns int) *Structure {
	b := newBuilder("l cover", origin, turns)
	m := coverMaterial(rng)
	l := float32(3 + rng.IntN(2))
	b.wall(-l/2, 0, l/2, 0, 0, 2.6, 0.3, m)
	b.wall(l/2-0.15, 0.15, l/2-0.15, 2.5, 0, 1.2, 0.3, m)
	return b.finish()
}
