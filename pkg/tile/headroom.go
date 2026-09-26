package tile

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/moeilijk/lhm-streamdeck/pkg/vecicon"
)

// HeadroomStyle carries one rendered frame of a Headroom-style quota tile:
// the provider rail and label up top with the provider glyph in the corner,
// the hero remaining percent, and a reset countdown on the bottom line —
// all chrome in the provider accent, no status ramp.
type HeadroomStyle struct {
	Label      string     // top line — the provider name, e.g. "CLAUDE"
	LabelShort string     // account-only label, used when Label can't fit
	Digits     string     // hero digits — remaining percent; "—" when unknown
	LineColor  color.RGBA // status line colour; zero falls back to the accent
	StatusLine string     // already-selected status text for this frame
	Stale      bool       // data older than expected — dot top right
	Icon       string     // provider glyph drawn in the top-right corner
	Accent     color.RGBA // provider colour for rail + label + icon + line
	Background color.RGBA
}

// Headroom's palette at canvas (2x) scale.
var (
	hrBG      = color.RGBA{0x0B, 0x0B, 0x0C, 0xFF}
	hrHero    = color.RGBA{0xF5, 0xF5, 0xF5, 0xFF}
	hrMuted   = color.RGBA{0x9A, 0x9E, 0xA8, 0xFF}
	HRStale   = color.RGBA{0x7A, 0x7A, 0x84, 0xFF}
	HRUnknown = color.RGBA{0x8A, 0x8A, 0x92, 0xFF}
)

// MutedHR exposes Headroom's muted tone for callers resolving line colours.
func MutedHR() color.RGBA { return hrMuted }

const (
	// Billboard geometry: 7px rail, label at y=24, hero baseline y=90,
	// status line at y=128 — matching Headroom's high-vis face.
	hrRailW        = 7
	hrLabelX       = hrRailW + 9
	hrLabelY       = 24
	hrLabelSize    = 15.0
	hrHeroBaseline = 90
	hrHeroSize     = 62.0
	hrStatusY      = 128
	hrStatusSize   = 17.0
	hrStaleX       = 132
	hrStaleY       = 36
	hrStaleR       = 3
	// Headroom's label runs with ~1.2px letter tracking.
	hrLabelTrack = 1
	// Provider glyph sits in the top-right corner.
	hrIconX    = 124
	hrIconY    = 9
	hrIconSize = 18
)

// RenderHeadroom draws a Headroom high-vis style key.
func RenderHeadroom(s HeadroomStyle) *image.RGBA {
	bg := s.Background
	if bg.A == 0 {
		bg = hrBG
	}
	accent := s.Accent
	if accent.A == 0 {
		accent = defaultAccnt
	}
	// Headroom's billboard hero is always near-white — accent lives in the
	// rail, label, icon and the bottom countdown line.
	hero := hrHero
	line := s.LineColor
	if line.A == 0 {
		line = accent
	}

	img := image.NewRGBA(image.Rect(0, 0, Canvas, Canvas))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)

	fillRect(img, 0, 0, hrRailW, Canvas, accent)

	// No pill — the label runs the full width up to the corner glyph.
	labelW := Canvas - 8 - hrLabelX
	if s.Icon != "" {
		labelW = hrIconX - 6 - hrLabelX
	}
	label, ls := fitTrackedText(s.Label, labelW, hrLabelSize, 10, hrLabelTrack)
	if s.LabelShort != "" && label != s.Label {
		// The full label needed truncation — the account short name carries
		// more information and usually fits comfortably.
		label, ls = fitTrackedText(s.LabelShort, labelW, hrLabelSize, 10, hrLabelTrack)
	}
	drawTextTracked(img, label, hrLabelX, hrLabelY, ls, accent, hrLabelTrack)

	drawHero(img, s.Digits, hero)

	if s.StatusLine != "" {
		status, ss := fitText(s.StatusLine, Canvas-24, hrStatusSize, 11)
		drawTextCentered(img, status, (hrRailW+9+Canvas-8)/2, hrStatusY, ss, line)
	}

	if s.Stale {
		r := vector.NewRasterizer(Canvas, Canvas)
		addCircle(r, hrStaleX, hrStaleY, hrStaleR)
		fillRaster(img, r, HRStale, 1)
	}

	if s.Icon != "" {
		vecicon.Draw(img, s.Icon, hrIconX, hrIconY, hrIconSize, accent)
	}
	return img
}

// drawHero centres the big digits with a smaller "%" glyph on the same
// baseline; three-digit values shrink slightly so 100 never clips.
func drawHero(img *image.RGBA, digits string, clr color.RGBA) {
	if digits == "" {
		digits = "—"
	}
	size := hrHeroSize
	if digits == "—" {
		size *= 0.85
	} else if len(digits) >= 3 {
		size *= 0.82
	}
	f, err := face(size)
	if err != nil {
		return
	}
	dw := textWidth(f, digits)
	gap, pw := 0, 0
	if digits != "—" {
		gap = int(0.02 * size)
		if pf, err := face(size * 0.4); err == nil {
			pw = textWidth(pf, "%")
		}
	}
	cx := (hrRailW + 9 + Canvas - 8) / 2
	x := cx - (dw+gap+pw)/2
	drawText(img, digits, x, hrHeroBaseline, size, clr)
	if digits != "—" {
		drawText(img, "%", x+dw+gap, hrHeroBaseline-int(0.06*size), size*0.4, clr)
	}
}

// drawTextTracked draws s with `track` extra pixels between glyphs — the
// letter-spacing Headroom applies to its provider label.
func drawTextTracked(dst *image.RGBA, s string, x, y int, size float64, clr color.Color, track int) {
	f, err := face(size)
	if err != nil {
		return
	}
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(clr), Face: f, Dot: fixed.P(x, y)}
	for _, r := range s {
		d.DrawString(string(r))
		d.Dot.X += fixed.I(track)
	}
}

// fitTrackedText mirrors fitText but measures with the extra tracking.
func fitTrackedText(s string, maxW int, size, minSize float64, track int) (string, float64) {
	if s == "" {
		return "", size
	}
	tracked := func(f font.Face, str string) int {
		n := 0
		for range str {
			n++
		}
		if n == 0 {
			return 0
		}
		return textWidth(f, str) + (n-1)*track
	}
	for sz := size; sz >= minSize; sz -= 1 {
		f, err := face(sz)
		if err != nil {
			return s, size
		}
		if tracked(f, s) <= maxW {
			return s, sz
		}
	}
	f, err := face(minSize)
	if err != nil {
		return s, minSize
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		cand := string(r) + "…"
		if tracked(f, cand) <= maxW {
			return cand, minSize
		}
	}
	return string(r), minSize
}
