package game

import (
	"image"
	"image/color"
)

// checker returns a size x size checkerboard with cells x cells squares.
func checker(size, cells int, a, b color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cell := size / cells
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := a
			if (x/cell+y/cell)%2 == 1 {
				c = b
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// panel returns a white square with a darker bevelled border, meant to be
// tinted by the draw colour (a simple crate/tile look).
func panel(size, border int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			edge := min(x, y, size-1-x, size-1-y)
			v := uint8(255)
			switch {
			case edge < border/3:
				v = 110
			case edge < border:
				v = 190
			case (x+y)%16 < 2: // faint diagonal grain
				v = 235
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}
