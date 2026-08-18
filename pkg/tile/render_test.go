package tile

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"golang.org/x/image/vector"

	"github.com/moeilijk/lhm-streamdeck/pkg/vecicon"
)

// TestSparklineTraceIsContinuous guards the winding direction of the round
// joins in strokePolyline. vector.Rasterizer accumulates signed coverage, so
// discs wound opposite to the segment quads subtract from them and punch
// periodic notches in the trace. A flat series must paint an unbroken band.
func TestSparklineTraceIsContinuous(t *testing.T) {
	const w, h = 144, 144
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)

	hist := make([]float64, 60)
	for i := range hist {
		hist[i] = 50
	}
	trace := color.RGBA{0, 255, 0, 255}
	x0, y0, x1, y1 := 10, 90, w-10, 136
	sparkline(img, hist, 0, 100, x0, y0, x1, y1, trace)

	// The flat series sits mid-band; scan a few pixels either side of it and
	// require green somewhere in that window at every x.
	mid := y1 - (y1-y0)/2
	for x := x0 + 3; x < x1-3; x++ {
		found := false
		for dy := -3; dy <= 3 && !found; dy++ {
			if g := img.RGBAAt(x, mid+dy).G; g > 160 {
				found = true
			}
		}
		if !found {
			t.Fatalf("gap in trace at x=%d (y=%d±3): joins likely wound against the segment quads", x, mid)
		}
	}
}

// TestStrokeDiscMatchesQuadWinding asserts the two primitives that make up a
// stroke accumulate rather than cancel where they overlap.
func TestStrokeDiscMatchesQuadWinding(t *testing.T) {
	const n = 40
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	r := vector.NewRasterizer(n, n)
	// One horizontal segment plus a disc centred on it.
	strokePolyline(r, []pt{{10, 20}, {30, 20}}, 8)
	r.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{255, 255, 255, 255}), image.Point{})

	if got := img.RGBAAt(20, 20).R; got < 250 {
		t.Fatalf("centre of stroke is not solid (R=%d); disc and quad windings disagree", got)
	}
}

func TestRasterizeHandlesArcs(t *testing.T) {
	// The NVIDIA mark uses elliptical arcs; a parser that skipped them would
	// silently render an empty or malformed icon.
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	r := vector.NewRasterizer(48, 48)
	// quarter-circle arc path
	vecicon.RasterizeForTest(r, "M4 24 a20 20 0 0 1 40 0 L4 24 z", 48, 48)
	r.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{255, 255, 255, 255}), image.Point{})
	painted := 0
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			if img.RGBAAt(x, y).R > 128 {
				painted++
			}
		}
	}
	if painted < 200 {
		t.Fatalf("arc path painted only %d px; arc conversion looks broken", painted)
	}
}

// TestMetricLabelReclaimsPlotBand pins the layout contract: hiding the middle
// label must hand its band to the graph rather than leaving a gap.
func TestMetricLabelReclaimsPlotBand(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Canvas, Canvas))
	SetFontPath("../../DejaVuSans-Bold.ttf")

	withLabel := drawMetric(img, "CPU TOTAL", Canvas/2, 100)
	without := drawMetric(img, "", Canvas/2, 100)

	if without >= withLabel {
		t.Fatalf("hidden label should raise the plot top: got %d, labelled %d", without, withLabel)
	}
	if withLabel != plotTop || without != plotTopNoLabel {
		t.Fatalf("unexpected plot tops: labelled=%d (want %d), hidden=%d (want %d)",
			withLabel, plotTop, without, plotTopNoLabel)
	}
}
