package tile

import (
	"image/color"
	"testing"
)

func init() {
	// face() reads the typeface from cwd; in tests that is pkg/tile, while
	// the font ships at the repo root.
	SetFontPath("../../DejaVuSans-Bold.ttf")
}

func testHeadroom() HeadroomStyle {
	return HeadroomStyle{
		Label:      "CLAUDE",
		Digits:     "58",
		StatusLine: "↻ 1H45M",
		Icon:       "claude",
		Accent:     color.RGBA{0xD9, 0x77, 0x57, 0xFF},
	}
}

func TestRenderHeadroomRailAndBackground(t *testing.T) {
	img := RenderHeadroom(testHeadroom())

	// Background is Headroom's near-black.
	if got := img.RGBAAt(72, 110); got != hrBG {
		t.Fatalf("background = %v", got)
	}
	// Left rail carries the provider accent across the full height.
	for _, y := range []int{4, 72, 140} {
		if got := img.RGBAAt(3, y); got != (color.RGBA{0xD9, 0x77, 0x57, 0xFF}) {
			t.Fatalf("rail pixel y=%d = %v", y, got)
		}
	}
	// Past the rail the accent is absent from the background band.
	if got := img.RGBAAt(80, 110); got == (color.RGBA{0xD9, 0x77, 0x57, 0xFF}) {
		t.Fatal("accent leaked past the rail")
	}
}

func TestRenderHeadroomNoPill(t *testing.T) {
	img := RenderHeadroom(testHeadroom())
	// The period pill is gone — its old capsule colour appears nowhere.
	pillBG := color.RGBA{0x2A, 0x2A, 0x2E, 0xFF}
	for y := 0; y < Canvas; y++ {
		for x := 0; x < Canvas; x++ {
			if img.RGBAAt(x, y) == pillBG {
				t.Fatalf("pill background pixel at (%d,%d)", x, y)
			}
		}
	}
}

func TestRenderHeadroomHeroDigits(t *testing.T) {
	img := RenderHeadroom(testHeadroom())
	// Hero digits are always Headroom's near-white, never status-coloured.
	found := false
	for y := 55; y < hrHeroBaseline; y++ {
		for x := 30; x < 120; x++ {
			if img.RGBAAt(x, y) == hrHero {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no near-white hero pixels in the digits region")
	}
}

func TestRenderHeadroomAccentStatusLine(t *testing.T) {
	img := RenderHeadroom(testHeadroom())
	// With no explicit LineColor the countdown line takes the accent —
	// the design drops the RAG ramp for provider colour.
	found := false
	for y := hrStatusY - int(hrStatusSize); y < hrStatusY; y++ {
		for x := 40; x < 104; x++ {
			if img.RGBAAt(x, y) == (color.RGBA{0xD9, 0x77, 0x57, 0xFF}) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no accent-coloured pixels on the status line")
	}
}

func TestRenderHeadroomProviderIcon(t *testing.T) {
	img := RenderHeadroom(testHeadroom())
	// The provider glyph sits in the top-right corner in accent colour.
	found := false
	for y := hrIconY; y < hrIconY+hrIconSize; y++ {
		for x := hrIconX; x < hrIconX+hrIconSize; x++ {
			if img.RGBAAt(x, y) == (color.RGBA{0xD9, 0x77, 0x57, 0xFF}) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no provider icon pixels in the corner")
	}

	s := testHeadroom()
	s.Icon = ""
	img = RenderHeadroom(s)
	for y := hrIconY; y < hrIconY+hrIconSize; y++ {
		for x := hrIconX; x < hrIconX+hrIconSize; x++ {
			if img.RGBAAt(x, y) == (color.RGBA{0xD9, 0x77, 0x57, 0xFF}) {
				t.Fatal("icon pixels painted without an icon")
			}
		}
	}
}

func TestRenderHeadroomStaleDot(t *testing.T) {
	s := testHeadroom()
	s.Stale = true
	img := RenderHeadroom(s)
	if got := img.RGBAAt(hrStaleX, hrStaleY); got != HRStale {
		t.Fatalf("stale dot = %v", got)
	}
	img = RenderHeadroom(testHeadroom())
	if got := img.RGBAAt(hrStaleX, hrStaleY); got == HRStale {
		t.Fatal("stale dot painted without stale flag")
	}
}

func TestRenderHeadroomFallbacks(t *testing.T) {
	// Zero-value style must still produce a complete tile.
	img := RenderHeadroom(HeadroomStyle{})
	if img == nil || img.Bounds().Dx() != Canvas {
		t.Fatal("zero style did not render")
	}
	if got := img.RGBAAt(3, 72); got != defaultAccnt {
		t.Fatalf("default accent rail = %v", got)
	}
}
