package mathx

import "testing"

func TestSRGB(t *testing.T) {
	cases := []struct {
		in   uint32
		want [4]float32
	}{
		{0x000000, [4]float32{0, 0, 0, 1}},
		{0xffffff, [4]float32{1, 1, 1, 1}},
		{0x808080, [4]float32{0.2158605, 0.2158605, 0.2158605, 1}}, // sRGB mid-grey
	}
	for _, c := range cases {
		got := Hex(c.in)
		for i := range got {
			if !near(got[i], c.want[i]) {
				t.Errorf("Hex(%06x) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}
