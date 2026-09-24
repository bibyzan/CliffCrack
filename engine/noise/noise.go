// Package noise is seeded, deterministic gradient noise for procedural content
// (pure Go). The same seed always gives the same field, so levels can be
// regenerated from a seed alone, and the renderer and the physics can sample
// the same terrain independently.
package noise

import (
	"math"
	"math/rand/v2"
)

// Field is 2D gradient (Perlin) noise. Values are roughly in [-1, 1] and vary
// smoothly on a scale of about one unit.
type Field struct {
	perm [512]uint8
}

// New returns the field for seed.
func New(seed uint64) *Field {
	f := &Field{}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	p := rng.Perm(256)
	for i := range 512 {
		f.perm[i] = uint8(p[i&255])
	}
	return f
}

// Eight unit gradients, evenly spaced around the circle.
var grads = [8][2]float64{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{math.Sqrt2 / 2, math.Sqrt2 / 2}, {-math.Sqrt2 / 2, math.Sqrt2 / 2},
	{math.Sqrt2 / 2, -math.Sqrt2 / 2}, {-math.Sqrt2 / 2, -math.Sqrt2 / 2},
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func (f *Field) grad(ix, iy int, dx, dy float64) float64 {
	g := grads[f.perm[int(f.perm[ix&255])+iy&255]&7]
	return g[0]*dx + g[1]*dy
}

// At samples the field at (x, y).
func (f *Field) At(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	ix, iy := int(x0), int(y0)
	dx, dy := x-x0, y-y0
	u, v := fade(dx), fade(dy)
	a := f.grad(ix, iy, dx, dy)
	b := f.grad(ix+1, iy, dx-1, dy)
	c := f.grad(ix, iy+1, dx, dy-1)
	d := f.grad(ix+1, iy+1, dx-1, dy-1)
	ab := a + u*(b-a)
	cd := c + u*(d-c)
	return (ab + v*(cd-ab)) * 1.4 // scale the typical range up towards [-1, 1]
}

// Line samples a 1D slice of the field.
func (f *Field) Line(x float64) float64 { return f.At(x, 0.37) }

// FBM sums octaves of noise, each twice the frequency and `gain` times the
// amplitude of the last, and normalises the result back to about [-1, 1].
func (f *Field) FBM(x, y float64, octaves int, gain float64) float64 {
	sum, amp, total := 0.0, 1.0, 0.0
	for i := 0; i < octaves; i++ {
		// Offset each octave so their lattices don't line up at the origin.
		sum += amp * f.At(x+float64(i)*17.3, y-float64(i)*9.1)
		total += amp
		amp *= gain
		x, y = x*2, y*2
	}
	return sum / total
}

// Ridged is FBM of folded noise (1 - |n|): sharp crests with soft valleys,
// good for mountain ranges. The result is in about [0, 1].
func (f *Field) Ridged(x, y float64, octaves int, gain float64) float64 {
	sum, amp, total := 0.0, 1.0, 0.0
	prev := 1.0
	for i := 0; i < octaves; i++ {
		r := 1 - math.Abs(f.At(x+float64(i)*17.3, y-float64(i)*9.1))
		r *= r
		sum += amp * r * prev // detail grows on the crests of the octave below
		prev = r
		total += amp
		amp *= gain
		x, y = x*2, y*2
	}
	return sum / total
}
