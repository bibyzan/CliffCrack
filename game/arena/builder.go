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
	trim   bool // mark the top edges of walls, columns and slabs as part of the glowing outline
}

func newBuilder(name string, origin mathx.Vec3, turns int) *builder {
	return &builder{origin: origin, turns: ((turns % 4) + 4) % 4, s: &Structure{Name: name}}
}

// box adds a chunk spanning lo..hi in the builder's frame.
func (b *builder) box(lo, hi mathx.Vec3, m Material) *Chunk {
	c := lo.Add(hi).Scale(0.5)
	h := hi.Sub(lo).Scale(0.5)
	for range b.turns { // (x, z) -> (z, -x) per quarter turn; half extents swap
		c = mathx.Vec3{c[2], c[1], -c[0]}
		h = mathx.Vec3{h[2], h[1], h[0]}
	}
	chunk := &Chunk{Centre: c.Add(b.origin), Half: h, Mat: m}
	b.s.Chunks = append(b.s.Chunks, chunk)
	return chunk
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
			var c *Chunk
			if alongX {
				xa, xb := min(x0, x1)+u0, min(x0, x1)+u1
				c = b.box(mathx.Vec3{xa, base + v0, z0 - t/2}, mathx.Vec3{xb, base + v1, z0 + t/2}, mat)
			} else {
				za, zb := min(z0, z1)+u0, min(z0, z1)+u1
				c = b.box(mathx.Vec3{x0 - t/2, base + v0, za}, mathx.Vec3{x0 + t/2, base + v1, zb}, mat)
			}
			c.Trim = b.trim && iv == nv-1 && mat != Glass
		}
	}
}

// slab is a floor or roof between (x0, z0) and (x1, z1) with its top at y.
func (b *builder) slab(x0, z0, x1, z1, y, thick float32, m Material) {
	b.tiles(x0, z0, x1, z1, y, thick, slabTile, m)
}

// tiles is a slab cut into tiles of about the given size. With trim on,
// the tiles round its edge carry the outline.
func (b *builder) tiles(x0, z0, x1, z1, y, thick, tile float32, m Material) {
	nx := max(1, int(math.Round(float64((x1-x0)/tile))))
	nz := max(1, int(math.Round(float64((z1-z0)/tile))))
	dx, dz := (x1-x0)/float32(nx), (z1-z0)/float32(nz)
	for i := 0; i < nx; i++ {
		for j := 0; j < nz; j++ {
			c := b.box(mathx.Vec3{x0 + float32(i)*dx, y - thick, z0 + float32(j)*dz},
				mathx.Vec3{x0 + float32(i+1)*dx, y, z0 + float32(j+1)*dz}, m)
			c.Trim = b.trim && (i == 0 || j == 0 || i == nx-1 || j == nz-1)
		}
	}
}

// Stairs: solid steps about stairRise high, cut into pieces no taller than
// stairPiece (nor wider than stairWidth) so they break up like everything
// else.
const (
	stairRise  = 0.19
	stairPiece = 1.0
	stairWidth = 3.0 // widest piece across
)

// stairs is a flight of solid steps from base up to top, climbing along X
// (or Z) from the foot at from to the head at to, between w0 and w1 across.
// The last step is level with top, so it meets a deck there flush. Each
// step's top piece collides as its stretch of a smooth ramp through the
// middles of the treads (a sphere can't climb steps); the foot of the ramp
// runs into the floor so there's no lip.
func (b *builder) stairs(alongX bool, from, to, w0, w1, base, top float32, m Material) {
	n := max(1, int(math.Round(float64((top-base)/stairRise))))
	run := (to - from) / float32(n)
	rise := (top - base) / float32(n)
	dir := float32(1) // uphill, along the stairs' axis
	if run < 0 {
		dir = -1
	}
	slope := float32(math.Atan2(float64(rise), float64(abs(run))))
	cos, sin := float32(math.Cos(float64(slope))), float32(math.Sin(float64(slope)))
	var uphill, normal mathx.Vec3
	if alongX {
		uphill, normal = mathx.Vec3{dir * cos, sin, 0}, mathx.Vec3{-dir * sin, cos, 0}
	} else {
		uphill, normal = mathx.Vec3{0, sin, dir * cos}, mathx.Vec3{0, cos, -dir * sin}
	}
	const thick = 0.15 // half thickness of each stretch of ramp
	rot := mathx.QuatFromBasis(normal.Cross(uphill), normal, uphill)
	across := max(1, int(math.Ceil(float64((w1-w0)/stairWidth)))) // one piece across if it'll do: seams catch your feet
	dw := (w1 - w0) / float32(across)
	for k := 0; k < n; k++ {
		u0, u1 := from+float32(k)*run, from+float32(k+1)*run
		u0, u1 = min(u0, u1), max(u0, u1)
		height := (top - base) * float32(k+1) / float32(n)
		up := max(1, int(math.Ceil(float64(height/stairPiece))))
		dh := height / float32(up)
		for a := 0; a < across; a++ {
			wa, wb := w0+float32(a)*dw, w0+float32(a+1)*dw
			var c *Chunk
			for v := 0; v < up; v++ {
				ya, yb := base+float32(v)*dh, base+float32(v+1)*dh
				if alongX {
					c = b.box(mathx.Vec3{u0, ya, wa}, mathx.Vec3{u1, yb, wb}, m)
				} else {
					c = b.box(mathx.Vec3{wa, ya, u0}, mathx.Vec3{wb, yb, u1}, m)
				}
			}
			// The top piece's stretch of ramp: its surface meets the tread's
			// height at the middle of the run.
			length := abs(run)/cos + 0.04
			mid := mathx.Vec3{(u0 + u1) / 2, base + height, (wa + wb) / 2}
			if !alongX {
				mid = mathx.Vec3{(wa + wb) / 2, base + height, (u0 + u1) / 2}
			}
			if k == 0 {
				length += 0.4
				mid = mid.Sub(uphill.Scale(0.2))
			}
			half := mathx.Vec3{(wb - wa) / 2, thick, length / 2}
			c.Slope = b.slope(mid.Sub(normal.Scale(thick)), half, rot)
		}
	}
}

// column is a square post of the given width from base up by height.
func (b *builder) column(x, z, base, height, width float32, m Material) {
	n := max(1, int(math.Round(float64(height/panelHeight))))
	dh := height / float32(n)
	for i := 0; i < n; i++ {
		c := b.box(mathx.Vec3{x - width/2, base + float32(i)*dh, z - width/2},
			mathx.Vec3{x + width/2, base + float32(i+1)*dh, z + width/2}, m)
		c.Trim = b.trim && i == n-1
	}
}

// slope turns a tilted box from the builder's frame into the world's.
func (b *builder) slope(centre, half mathx.Vec3, rot mathx.Quat) *Slope {
	for range b.turns { // as in box: (x, z) -> (z, -x), a quarter turn about +Y
		centre = mathx.Vec3{centre[2], centre[1], -centre[0]}
	}
	turn := mathx.AxisAngle(mathx.Vec3{0, 1, 0}, float32(b.turns)*math.Pi/2)
	return &Slope{Centre: centre.Add(b.origin), Half: half, Rotation: turn.Mul(rot)}
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
