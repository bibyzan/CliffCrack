package svgicon

import (
	"strings"
	"testing"
)

func alpha(t *testing.T, svg string, height, x, y int) uint8 {
	t.Helper()
	ic, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}
	return ic.Rasterize(height).NRGBAAt(x, y).A
}

func TestRectFillsAndCutsOut(t *testing.T) {
	svg := `<svg viewBox="0 0 10 10"><rect x="1" y="1" width="8" height="8"/><rect x="4" y="4" width="2" height="2" fill="#000"/></svg>`
	if a := alpha(t, svg, 10, 2, 2); a != 255 {
		t.Errorf("inside the rect: alpha %d, want 255", a)
	}
	if a := alpha(t, svg, 10, 0, 0); a != 0 {
		t.Errorf("outside: alpha %d, want 0", a)
	}
	if a := alpha(t, svg, 10, 4, 4); a != 0 {
		t.Errorf("in the cut-out: alpha %d, want 0", a)
	}
}

func TestPathsCurvesAndEdges(t *testing.T) {
	// A triangle, relative lines, and an antialiased diagonal edge.
	svg := `<svg viewBox="0 0 10 10"><path d="M0 0l10 0-10 10z"/></svg>`
	if a := alpha(t, svg, 10, 1, 1); a != 255 {
		t.Errorf("inside the triangle: %d", a)
	}
	if a := alpha(t, svg, 10, 8, 8); a != 0 {
		t.Errorf("outside the triangle: %d", a)
	}
	if a := alpha(t, svg, 10, 5, 4); a == 0 || a == 255 {
		t.Errorf("on the diagonal: %d, want partial coverage", a)
	}
	// A circle and a cubic curve parse and fill.
	if a := alpha(t, `<svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="4"/></svg>`, 20, 10, 10); a != 255 {
		t.Errorf("circle centre: %d", a)
	}
	curve := `<svg viewBox="0 0 10 10"><path d="M1 9C1 1 9 1 9 9Z"/></svg>`
	if a := alpha(t, curve, 10, 5, 6); a != 255 {
		t.Errorf("under the curve: %d", a)
	}
	// Relative curves are the same shape.
	rel := `<svg viewBox="0 0 10 10"><path d="M1 9c0-8 8-8 8 0z"/></svg>`
	for _, p := range [][2]int{{5, 6}, {2, 2}, {8, 8}} {
		if a, b := alpha(t, curve, 10, p[0], p[1]), alpha(t, rel, 10, p[0], p[1]); a != b {
			t.Errorf("at %v: absolute %d, relative %d", p, a, b)
		}
	}
}

func TestAspectAndErrors(t *testing.T) {
	ic, err := Parse(strings.NewReader(`<svg viewBox="0 0 30 10"><rect width="30" height="10"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if b := ic.Rasterize(16).Bounds(); b.Dx() != 48 || b.Dy() != 16 {
		t.Errorf("size %v, want 48x16", b)
	}
	if _, err := Parse(strings.NewReader(`<svg><rect/></svg>`)); err == nil {
		t.Error("an svg without a viewBox should be an error")
	}
	if _, err := Parse(strings.NewReader(`<svg viewBox="0 0 1 1"><path d="M0 0A1 1 0 0 1 1 1"/></svg>`)); err == nil {
		t.Error("an unsupported path command should be an error")
	}
}
