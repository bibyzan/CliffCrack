package mathx

import "math"

// SRGB converts an sRGB colour (what colour pickers show) into the linear RGBA
// the renderer works in. Alpha is already linear and passes through unchanged.
func SRGB(r, g, b, a float32) [4]float32 {
	return [4]float32{srgbToLinear(r), srgbToLinear(g), srgbToLinear(b), a}
}

// Hex converts a 0xRRGGBB sRGB colour into opaque linear RGBA.
func Hex(rgb uint32) [4]float32 {
	return SRGB(float32(rgb>>16&0xff)/255, float32(rgb>>8&0xff)/255, float32(rgb&0xff)/255, 1)
}

func srgbToLinear(c float32) float32 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return float32(math.Pow((float64(c)+0.055)/1.055, 2.4))
}
