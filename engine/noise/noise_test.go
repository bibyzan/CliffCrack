package noise

import (
	"math"
	"testing"
)

func TestDeterministicPerSeed(t *testing.T) {
	a, b, c := New(7), New(7), New(8)
	same, differ := true, false
	for i := 0; i < 100; i++ {
		x, y := float64(i)*0.37, float64(i)*-0.61
		if a.At(x, y) != b.At(x, y) {
			same = false
		}
		if a.At(x, y) != c.At(x, y) {
			differ = true
		}
	}
	if !same {
		t.Error("the same seed must give the same field")
	}
	if !differ {
		t.Error("different seeds should give different fields")
	}
}

func TestRangeAndZeroAtLattice(t *testing.T) {
	f := New(1)
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := 0; i < 20000; i++ {
		x, y := float64(i%200)*0.173-17, float64(i/200)*0.219-11
		v := f.At(x, y)
		lo, hi = min(lo, v), max(hi, v)
	}
	if lo < -1.5 || hi > 1.5 {
		t.Errorf("range [%v, %v] outside [-1.5, 1.5]", lo, hi)
	}
	if hi-lo < 1 {
		t.Errorf("range [%v, %v] is too flat", lo, hi)
	}
	if v := f.At(3, -4); v != 0 {
		t.Errorf("gradient noise is 0 on lattice points, got %v", v)
	}
}

func TestContinuous(t *testing.T) {
	f := New(3)
	const step = 1e-3
	for i := 0; i < 5000; i++ {
		x, y := float64(i)*0.0131-20, float64(i)*0.0077+5
		if d := math.Abs(f.FBM(x+step, y, 4, 0.5) - f.FBM(x, y, 4, 0.5)); d > 0.05 {
			t.Fatalf("jump of %v at (%v, %v)", d, x, y)
		}
	}
}

func TestRidgedRange(t *testing.T) {
	f := New(4)
	for i := 0; i < 5000; i++ {
		v := f.Ridged(float64(i)*0.071, float64(i)*0.013, 5, 0.5)
		if v < 0 || v > 1 {
			t.Fatalf("ridged value %v outside [0, 1]", v)
		}
	}
}
