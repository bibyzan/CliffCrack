// Package svgicon reads simple SVG icons and rasterizes them to alpha masks
// for textures: flat silhouettes made of paths (straight lines, and cubic and
// quadratic curves), rects, polygons, circles and ellipses. Shapes filled in
// black (fill="#000", "#000000" or "black") are cut out of the rest, so an
// icon can have holes; everything else is filled white. It is enough for HUD
// icons, not a general SVG renderer: no strokes, transforms, gradients or
// text.
package svgicon

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type point struct{ x, y float64 }

// shape is one filled outline (possibly several closed rings, filled
// even-odd), added to or cut from the icon.
type shape struct {
	rings [][]point
	cut   bool
}

// Icon is a parsed SVG: its view box and shapes.
type Icon struct {
	minX, minY, w, h float64
	shapes           []shape
}

// Aspect is the icon's width over its height.
func (ic *Icon) Aspect() float64 { return ic.w / ic.h }

// Parse reads an SVG document.
func Parse(r io.Reader) (*Icon, error) {
	dec := xml.NewDecoder(r)
	ic := &Icon{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svgicon: %w", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := map[string]string{}
		for _, a := range el.Attr {
			attr[a.Name.Local] = a.Value
		}
		cut := isBlack(attr["fill"])
		if attr["fill"] == "none" {
			continue
		}
		var rings [][]point
		switch el.Name.Local {
		case "svg":
			vb := numbers(attr["viewBox"])
			if len(vb) != 4 {
				return nil, errors.New("svgicon: the svg needs a viewBox")
			}
			ic.minX, ic.minY, ic.w, ic.h = vb[0], vb[1], vb[2], vb[3]
			continue
		case "rect":
			x, y, w, h := num(attr["x"]), num(attr["y"]), num(attr["width"]), num(attr["height"])
			rings = [][]point{{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}}
		case "polygon":
			n := numbers(attr["points"])
			var ring []point
			for i := 0; i+1 < len(n); i += 2 {
				ring = append(ring, point{n[i], n[i+1]})
			}
			rings = [][]point{ring}
		case "circle":
			r := num(attr["r"])
			rings = [][]point{ellipse(num(attr["cx"]), num(attr["cy"]), r, r)}
		case "ellipse":
			rings = [][]point{ellipse(num(attr["cx"]), num(attr["cy"]), num(attr["rx"]), num(attr["ry"]))}
		case "path":
			var err error
			if rings, err = path(attr["d"]); err != nil {
				return nil, err
			}
		default:
			continue
		}
		ic.shapes = append(ic.shapes, shape{rings: rings, cut: cut})
	}
	if ic.w <= 0 || ic.h <= 0 {
		return nil, errors.New("svgicon: no svg element with a viewBox")
	}
	return ic, nil
}

func isBlack(fill string) bool {
	switch strings.ToLower(strings.TrimSpace(fill)) {
	case "#000", "#000000", "black":
		return true
	}
	return false
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// numbers splits a list of numbers separated by spaces and/or commas.
func numbers(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		if v, err := strconv.ParseFloat(f, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func ellipse(cx, cy, rx, ry float64) []point {
	const n = 32
	ring := make([]point, n)
	for i := range ring {
		a := 2 * math.Pi * float64(i) / n
		ring[i] = point{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return ring
}

// path reads path data: M, L, H, V, C, Q and Z, absolute or relative. Curves
// are flattened into straight segments.
func path(d string) ([][]point, error) {
	toks := pathTokens(d)
	var rings [][]point
	var ring []point
	var cur, start point
	cmd := byte(0)
	i := 0
	next := func() (float64, error) {
		if i >= len(toks) || isCommand(toks[i]) {
			return 0, fmt.Errorf("svgicon: path %q: missing number", d)
		}
		v, err := strconv.ParseFloat(toks[i], 64)
		i++
		return v, err
	}
	pt := func(rel bool) (point, error) {
		x, err := next()
		if err != nil {
			return point{}, err
		}
		y, err := next()
		if rel {
			x, y = x+cur.x, y+cur.y
		}
		return point{x, y}, err
	}
	closeRing := func() {
		if len(ring) > 2 {
			rings = append(rings, ring)
		}
		ring = nil
	}
	for i < len(toks) {
		if isCommand(toks[i]) {
			cmd = toks[i][0]
			i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("svgicon: path %q doesn't start with a command", d)
		}
		rel := cmd >= 'a'
		switch cmd | 0x20 { // lower case
		case 'm':
			p, err := pt(rel)
			if err != nil {
				return nil, err
			}
			closeRing()
			cur, start = p, p
			ring = []point{p}
			if cmd == 'm' {
				cmd = 'l' // further pairs are lines
			} else {
				cmd = 'L'
			}
		case 'l':
			p, err := pt(rel)
			if err != nil {
				return nil, err
			}
			cur = p
			ring = append(ring, p)
		case 'h':
			x, err := next()
			if err != nil {
				return nil, err
			}
			if rel {
				x += cur.x
			}
			cur.x = x
			ring = append(ring, cur)
		case 'v':
			y, err := next()
			if err != nil {
				return nil, err
			}
			if rel {
				y += cur.y
			}
			cur.y = y
			ring = append(ring, cur)
		case 'c':
			// (Relative, all three points are from the segment's start: cur
			// doesn't move until the end.)
			c1, err1 := pt(rel)
			c2, err2 := pt(rel)
			end, err3 := pt(rel)
			if err := errors.Join(err1, err2, err3); err != nil {
				return nil, err
			}
			p0 := cur
			for k := 1; k <= 12; k++ {
				t := float64(k) / 12
				u := 1 - t
				ring = append(ring, point{
					u*u*u*p0.x + 3*u*u*t*c1.x + 3*u*t*t*c2.x + t*t*t*end.x,
					u*u*u*p0.y + 3*u*u*t*c1.y + 3*u*t*t*c2.y + t*t*t*end.y,
				})
			}
			cur = end
		case 'q':
			c, err1 := pt(rel)
			end, err2 := pt(rel)
			if err := errors.Join(err1, err2); err != nil {
				return nil, err
			}
			p0 := cur
			for k := 1; k <= 10; k++ {
				t := float64(k) / 10
				u := 1 - t
				ring = append(ring, point{u*u*p0.x + 2*u*t*c.x + t*t*end.x, u*u*p0.y + 2*u*t*c.y + t*t*end.y})
			}
			cur = end
		case 'z':
			closeRing()
			cur = start
		default:
			return nil, fmt.Errorf("svgicon: path command %q isn't supported", cmd)
		}
	}
	closeRing()
	return rings, nil
}

func isCommand(t string) bool {
	return len(t) == 1 && strings.ContainsAny(t, "MmLlHhVvCcQqZz")
}

// pathTokens splits path data into commands and numbers ("M1-2.5.5" is
// M, 1, -2.5, .5).
func pathTokens(d string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range d {
		switch {
		case unicode.IsLetter(r) && r != 'e' && r != 'E':
			flush()
			out = append(out, string(r))
		case r == ',' || unicode.IsSpace(r):
			flush()
		case r == '-' && cur.Len() > 0 && !strings.HasSuffix(cur.String(), "e"):
			flush()
			cur.WriteRune(r)
		case r == '.' && strings.Contains(cur.String(), "."):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// Rasterize draws the icon height pixels tall (and as wide as its aspect
// makes it): white, with alpha the coverage, antialiased 4x4.
func (ic *Icon) Rasterize(height int) *image.NRGBA {
	const ss = 4 // samples per pixel, each way
	width := max(1, int(math.Round(float64(height)*ic.Aspect())))
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	scale := float64(height) / ic.h
	cover := make([]float64, width)
	var xs []float64
	for py := 0; py < height; py++ {
		clear(cover)
		for sy := 0; sy < ss; sy++ {
			y := (float64(py) + (float64(sy)+0.5)/ss) / scale
			y += ic.minY
			// Each sub-row: the shapes' spans, added or cut in order.
			row := make([]bool, width*ss)
			for _, sh := range ic.shapes {
				xs = xs[:0]
				for _, ring := range sh.rings {
					for k := range ring {
						a, b := ring[k], ring[(k+1)%len(ring)]
						if (a.y <= y) != (b.y <= y) {
							x := a.x + (y-a.y)/(b.y-a.y)*(b.x-a.x)
							xs = append(xs, (x-ic.minX)*scale*ss)
						}
					}
				}
				sort.Float64s(xs)
				for k := 0; k+1 < len(xs); k += 2 {
					x0 := max(int(math.Ceil(xs[k]-0.5)), 0)
					x1 := min(int(math.Ceil(xs[k+1]-0.5)), len(row))
					for x := x0; x < x1; x++ {
						row[x] = !sh.cut
					}
				}
			}
			for x, on := range row {
				if on {
					cover[x/ss] += 1.0 / (ss * ss)
				}
			}
		}
		for px, c := range cover {
			img.SetNRGBA(px, py, color.NRGBA{255, 255, 255, uint8(math.Round(min(c, 1) * 255))})
		}
	}
	return img
}
