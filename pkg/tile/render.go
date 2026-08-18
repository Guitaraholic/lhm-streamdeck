package tile

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"

	"github.com/moeilijk/lhm-streamdeck/pkg/vecicon"
)

type pt struct{ x, y float64 }

// Sparkline weights, in canvas (2x) pixels. The trace is heavy enough to read
// as its own line at 72px once Stream Deck downscales the key.
const (
	strokeWidth = 4.0
	areaAlpha   = 0.22

	// railWidth is the accent stripe on the left edge. Headroom uses 6-7 at
	// this canvas size; a little heavier reads better next to our icon.
	railWidth = 9

	// Plot box, matching Native Hardware Monitor's proportions: generous side
	// padding and a tall graph that stops short of the bottom edge.
	plotPad    = 17
	plotTop    = 85
	plotBottom = 134
	// With the middle label hidden the plot reclaims that band, landing close
	// to Native Hardware Monitor's 60px graph.
	plotTopNoLabel = 72

	valueSize      = 40.0
	valueBaseline  = 56
	metricBaseline = 80

	// hostSize / metricSize are deliberately close in weight: the metric name
	// is the thing you read to know what the number means, so it should not be
	// a caption. Native Hardware Monitor sets its equivalent at 18.
	hostSize   = 15.0
	metricSize = 20.0
)

// fillPath rasterizes an already-described path with antialiasing.
func fillRaster(dst *image.RGBA, r *vector.Rasterizer, clr color.Color, alpha float64) {
	if alpha >= 1 {
		r.Draw(dst, dst.Bounds(), image.NewUniform(clr), image.Point{})
		return
	}
	// Pre-multiply the source alpha so the rasterizer's coverage mask blends
	// at the requested opacity.
	c := color.RGBAModel.Convert(clr).(color.RGBA)
	c.A = uint8(math.Round(float64(c.A) * alpha))
	c.R = uint8(math.Round(float64(c.R) * alpha))
	c.G = uint8(math.Round(float64(c.G) * alpha))
	c.B = uint8(math.Round(float64(c.B) * alpha))
	r.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
}

// addCircle appends a circle to the rasterizer using four cubic segments.
//
// Wound clockwise in screen coordinates to match the segment quads in
// strokePolyline. vector.Rasterizer accumulates *signed* coverage, so a disc
// wound the other way would subtract from the quad it overlaps and punch
// scalloped notches out of the trace instead of rounding its joins.
func addCircle(r *vector.Rasterizer, cx, cy, rad float64) {
	const k = 0.5522847498 // circle-to-bezier constant
	o := rad * k
	r.MoveTo(float32(cx+rad), float32(cy))
	r.CubeTo(float32(cx+rad), float32(cy-o), float32(cx+o), float32(cy-rad), float32(cx), float32(cy-rad))
	r.CubeTo(float32(cx-o), float32(cy-rad), float32(cx-rad), float32(cy-o), float32(cx-rad), float32(cy))
	r.CubeTo(float32(cx-rad), float32(cy+o), float32(cx-o), float32(cy+rad), float32(cx), float32(cy+rad))
	r.CubeTo(float32(cx+o), float32(cy+rad), float32(cx+rad), float32(cy+o), float32(cx+rad), float32(cy))
	r.ClosePath()
}

// strokePolyline approximates a round-capped, round-joined stroke by unioning
// a quad per segment with a disc at every vertex. vector.Rasterizer has no
// stroker, and overlapping fills are safe under its non-zero winding rule.
func strokePolyline(r *vector.Rasterizer, pts []pt, width float64) {
	h := width / 2
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*h, dx/l*h
		r.MoveTo(float32(a.x+nx), float32(a.y+ny))
		r.LineTo(float32(b.x+nx), float32(b.y+ny))
		r.LineTo(float32(b.x-nx), float32(b.y-ny))
		r.LineTo(float32(a.x-nx), float32(a.y-ny))
		r.ClosePath()
	}
	for _, p := range pts {
		addCircle(r, p.x, p.y, h)
	}
}

// sparkline draws gridlines, a translucent area fill and a stroked trace
// inside the rectangle (x0,y0)-(x1,y1).
func sparkline(dst *image.RGBA, hist []float64, min, max float64, x0, y0, x1, y1 int, clr color.RGBA) {
	w, h := float64(x1-x0), float64(y1-y0)

	if len(hist) < 2 || max <= min {
		return
	}
	pts := make([]pt, 0, len(hist))
	for i, v := range hist {
		f := (v - min) / (max - min)
		f = math.Max(0, math.Min(1, f))
		pts = append(pts, pt{
			x: float64(x0) + w*float64(i)/float64(len(hist)-1),
			y: float64(y1) - h*f,
		})
	}

	// Area first, kept deliberately faint so the trace on top stays the
	// dominant edge rather than blending into its own fill.
	area := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	area.MoveTo(float32(pts[0].x), float32(y1))
	for _, p := range pts {
		area.LineTo(float32(p.x), float32(p.y))
	}
	area.LineTo(float32(pts[len(pts)-1].x), float32(y1))
	area.ClosePath()
	fillRaster(dst, area, clr, areaAlpha)

	stroke := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	strokePolyline(stroke, pts, strokeWidth)
	fillRaster(dst, stroke, clr, 1)
}

func fillRect(dst *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(dst, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
}

// Render draws one key image at Canvas x Canvas pixels.
func Render(s Style) *image.RGBA {
	bg := s.Background
	if bg.A == 0 {
		bg = defaultBG
	}
	accent := s.Accent
	if accent.A == 0 {
		accent = defaultAccnt
	}

	img := image.NewRGBA(image.Rect(0, 0, Canvas, Canvas))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)

	// colour the value by where it sits in range, unless the host accent is
	// requested instead
	frac := 0.0
	if s.Max > s.Min && len(s.History) > 0 {
		frac = (s.History[len(s.History)-1] - s.Min) / (s.Max - s.Min)
	}
	vc := valueColor(frac)
	if s.UseAccentForValue {
		vc = accent
	}

	switch s.Layout {
	case LayoutRail:
		renderRail(img, s, accent, vc)
	case LayoutCorner:
		renderCorner(img, s, accent, vc)
	default:
		renderHeader(img, s, accent, vc)
	}
	return img
}

// renderHeader: icon + host across the top, metric beneath the value, and a
// full-width sparkline at the foot. Same information as renderRail without the
// accent stripe, for people who prefer an unbroken tile edge.
func renderHeader(img *image.RGBA, s Style, accent, vc color.RGBA) {
	x := 10
	if s.Icon != "" && s.Icon != "none" {
		vecicon.Draw(img, s.Icon, x, 6, 19, accent)
		x += 24
	}
	host, hs := fitText(s.HostLabel, Canvas-10-x, hostSize, 10)
	drawText(img, host, x, 20, hs, colHost)

	drawValue(img, s, vc, valueBaseline, valueSize)
	top := drawMetric(img, s.MetricLabel, Canvas/2, Canvas-2*plotPad+10)
	sparkline(img, s.History, s.Min, s.Max, plotPad, top, Canvas-plotPad, plotBottom, vc)
}

// drawMetric renders the middle label if there is one and returns the y the
// plot should start at. An empty label gives its band back to the graph.
func drawMetric(img *image.RGBA, label string, cx, maxW int) int {
	if label == "" {
		return plotTopNoLabel
	}
	text, size := fitText(label, maxW, metricSize, 11)
	drawTextCentered(img, text, cx, metricBaseline, size, colMetric)
	return plotTop
}

// renderRail: accent rail down the left edge carrying the icon, everything
// else shifted right.
func renderRail(img *image.RGBA, s Style, accent, vc color.RGBA) {
	fillRect(img, 0, 0, railWidth, Canvas, accent)
	x := railWidth + 7
	if s.Icon != "" && s.Icon != "none" {
		vecicon.Draw(img, s.Icon, x, 6, 18, accent)
		x += 23
	}
	host, hs := fitText(s.HostLabel, Canvas-8-x, hostSize, 10)
	drawText(img, host, x, 20, hs, colHost)

	cx := (railWidth + plotPad + Canvas - plotPad) / 2
	drawValueAt(img, s, vc, cx, valueBaseline, valueSize)
	top := drawMetric(img, s.MetricLabel, cx, Canvas-railWidth-2*plotPad+10)
	sparkline(img, s.History, s.Min, s.Max, railWidth+plotPad-8, top, Canvas-plotPad, plotBottom, vc)
}

// renderCorner: value dominates, small icon top-right, host name at the foot.
func renderCorner(img *image.RGBA, s Style, accent, vc color.RGBA) {
	iconW := 0
	if s.Icon != "" && s.Icon != "none" {
		vecicon.Draw(img, s.Icon, Canvas-27, 6, 18, accent)
		iconW = 27
	}
	if s.MetricLabel != "" {
		metric, ms := fitText(s.MetricLabel, Canvas-10-iconW, metricSize, 11)
		drawText(img, metric, 10, 22, ms, colMetric)
	}
	drawValue(img, s, vc, 62, valueSize)
	sparkline(img, s.History, s.Min, s.Max, plotPad, 74, Canvas-plotPad, 120, vc)
	host, hs := fitText(s.HostLabel, Canvas-20, hostSize, 10)
	drawTextCentered(img, host, Canvas/2, 138, hs, colHost)
}

// drawValue centres the value and its unit as a single group, with the unit
// set smaller and in a muted tone so the number reads first.
func drawValue(img *image.RGBA, s Style, vc color.RGBA, baseline int, size float64) {
	drawValueAt(img, s, vc, Canvas/2, baseline, size)
}

func drawValueAt(img *image.RGBA, s Style, vc color.RGBA, cx, baseline int, size float64) {
	vf, err := face(size)
	if err != nil {
		return
	}
	unitSize := size * 0.58
	uf, err2 := face(unitSize)
	if err2 != nil {
		return
	}
	vw := textWidth(vf, s.ValueText)
	uw := 0
	if s.Unit != "" {
		uw = textWidth(uf, s.Unit) + 4
	}
	x := cx - (vw+uw)/2
	drawText(img, s.ValueText, x, baseline, size, vc)
	if s.Unit != "" {
		mutedUnit := color.RGBA{vc.R, vc.G, vc.B, 0xCC}
		drawText(img, s.Unit, x+vw+4, baseline, unitSize, mutedUnit)
	}
}
