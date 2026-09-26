// tile-preview renders sample key images so tile styling can be iterated on
// without installing the plugin into Stream Deck.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"

	"github.com/moeilijk/lhm-streamdeck/pkg/tile"
)

type sample struct {
	icon, host, metric, value, unit string
	accent                          color.RGBA
	series                          []float64
	min, max                        float64
}

// wave builds a plausible-looking history series so the sparkline has shape.
func wave(base, amp, phase float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / float64(n)
		v := base + amp*math.Sin(t*6.0+phase) + amp*0.45*math.Sin(t*17.0+phase*2)
		out[i] = math.Max(0, math.Min(100, v))
	}
	return out
}

func main() {
	out := flag.String("out", "/tmp/tiles.png", "output png")
	font := flag.String("font", "", "path to DejaVuSans-Bold.ttf")
	scale := flag.Int("scale", 1, "1 = 144px tiles, 2 = also show 72px")
	headroom := flag.Bool("headroom", false, "render Headroom-style quota tiles instead of the layout sheet")
	flag.Parse()
	if *font != "" {
		tile.SetFontPath(*font)
	}
	if *headroom {
		renderHeadroomSheet(*out, *scale)
		return
	}

	apple := color.RGBA{0xE6, 0xE8, 0xEB, 0xFF}
	green := color.RGBA{0x76, 0xB9, 0x00, 0xFF} // NVIDIA green
	amber := color.RGBA{0xE9, 0x95, 0x2C, 0xFF}
	blue := color.RGBA{0x6E, 0x9E, 0xFF, 0xFF}

	samples := []sample{
		{"apple", "MAC STUDIO", "CPU", "34", "%", apple, wave(34, 16, 0.4, 60), 0, 100},
		{"nvidia", "DGX-01", "GPU", "91", "%", green, wave(88, 9, 1.1, 60), 0, 100},
		{"nvidia", "DGX-02", "GPU TEMP", "72", "°C", green, wave(70, 8, 2.0, 60), 0, 100},
		{"linux", "SRV-01", "MEM", "58", "%", amber, wave(58, 12, 0.2, 60), 0, 100},
		{"server", "SRV-02", "CPU", "12", "%", blue, wave(12, 8, 1.6, 60), 0, 100},
		// middle label removed: the plot reclaims the band
		{"nvidia", "DGX-01", "", "64", "°C", green, wave(64, 10, 0.9, 60), 0, 100},
	}
	layouts := []tile.Layout{tile.LayoutHeader, tile.LayoutRail, tile.LayoutCorner}

	pad, cell := 14, tile.Canvas
	rowH := cell + pad
	if *scale > 1 {
		rowH = cell + 72 + pad*2
	}
	w := pad + len(samples)*(cell+pad)
	h := pad + len(layouts)*rowH
	sheet := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{0x0B, 0x0C, 0x0E, 0xFF}), image.Point{}, xdraw.Src)

	for r, lay := range layouts {
		for c, s := range samples {
			img := tile.Render(tile.Style{
				Layout: lay, Icon: s.icon, HostLabel: s.host, MetricLabel: s.metric,
				ValueText: s.value, Unit: s.unit, History: s.series,
				Min: s.min, Max: s.max, Accent: s.accent,
			})
			x := pad + c*(cell+pad)
			y := pad + r*rowH
			xdraw.Draw(sheet, image.Rect(x, y, x+cell, y+cell), img, image.Point{}, xdraw.Src)

			if *scale > 1 {
				small := image.NewRGBA(image.Rect(0, 0, 72, 72))
				xdraw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)
				sx := x + (cell-72)/2
				sy := y + cell + pad
				xdraw.Draw(sheet, image.Rect(sx, sy, sx+72, sy+72), small, image.Point{}, xdraw.Src)
			}
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, sheet); err != nil {
		panic(err)
	}
}

// renderHeadroomSheet lays out Headroom-style quota tiles across the status
// ramp and providers, plus the 72px downscale Stream Deck actually shows.
func renderHeadroomSheet(out string, scale int) {
	samples := []tile.HeadroomStyle{
		{Label: "CLAUDE", LabelShort: "KEV", Digits: "58", StatusLine: "↻ 5H 1H45M", Icon: "claude", Accent: color.RGBA{0xD9, 0x77, 0x57, 0xFF}},
		{Label: "CLAUDE", LabelShort: "PAUL", Digits: "13", StatusLine: "↻ WK 2D9H", Icon: "claude", Accent: color.RGBA{0xD9, 0x77, 0x57, 0xFF}},
		{Label: "CODEX", LabelShort: "PAULOUDEX", Digits: "0", StatusLine: "↻ WK 4D16H", Icon: "codex", Accent: color.RGBA{0x10, 0xA3, 0x7F, 0xFF}},
		{Label: "GROK", LabelShort: "MAIN", Digits: "88", StatusLine: "↻ DY 18H5M", Icon: "grok", Accent: color.RGBA{0xFF, 0x55, 0x00, 0xFF}},
		{Label: "CODEX", LabelShort: "KEV", Digits: "—", StatusLine: "no quota", Stale: true, Icon: "codex", Accent: color.RGBA{0x10, 0xA3, 0x7F, 0xFF}},
		{Label: "OPENCODE", LabelShort: "7F98D", Digits: "50", StatusLine: "↻ MO 16D2H", Icon: "opencode", Accent: color.RGBA{0xE8, 0xE4, 0xDF, 0xFF}},
	}

	pad, cell := 14, tile.Canvas
	rowH := cell + pad
	if scale > 1 {
		rowH = cell + 72 + pad*2
	}
	w := pad + len(samples)*(cell+pad)
	h := pad + rowH
	sheet := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{0x0B, 0x0C, 0x0E, 0xFF}), image.Point{}, xdraw.Src)

	for c, s := range samples {
		img := tile.RenderHeadroom(s)
		x := pad + c*(cell+pad)
		y := pad
		xdraw.Draw(sheet, image.Rect(x, y, x+cell, y+cell), img, image.Point{}, xdraw.Src)
		if scale > 1 {
			small := image.NewRGBA(image.Rect(0, 0, 72, 72))
			xdraw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)
			xdraw.Draw(sheet, image.Rect(x+36, y+cell+pad, x+36+72, y+cell+pad+72), small, image.Point{}, xdraw.Src)
		}
	}

	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, sheet); err != nil {
		panic(err)
	}
}
