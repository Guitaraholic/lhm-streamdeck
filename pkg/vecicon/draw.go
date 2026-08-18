package vecicon

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/vector"
)

// Draw renders the named icon into dst at the given top-left offset, sized
// size x size pixels and filled with clr. Unknown names draw nothing.
func Draw(dst *image.RGBA, name string, x, y, size int, clr color.Color) {
	d, ok := Icons[name]
	if !ok || size <= 0 {
		return
	}
	r := vector.NewRasterizer(size, size)
	Rasterize(r, d, 24, float64(size))
	r.Draw(dst, image.Rect(x, y, x+size, y+size), image.NewUniform(clr), image.Point{})
}

// Render returns the named icon as a standalone RGBA image with a transparent
// background. Useful for compositing and for tests.
func Render(name string, size int, clr color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	Draw(img, name, 0, 0, size, clr)
	return img
}
