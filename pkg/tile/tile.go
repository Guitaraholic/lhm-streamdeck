// Package tile renders Stream Deck key images for a single reading: a host
// badge identifying the machine, the current value, and a history sparkline.
package tile

import (
	"image"
	"image/color"
	"os"
	"sync"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Canvas is the render size. Stream Deck keys are 72x72; rendering at 2x and
// letting the app downscale keeps text and the sparkline crisp.
const Canvas = 144

// Layout selects how the host badge is arranged relative to the reading.
type Layout string

const (
	// LayoutHeader puts the icon and host name in a row across the top.
	LayoutHeader Layout = "header"
	// LayoutRail puts a vertical accent rail with the icon down the left edge.
	LayoutRail Layout = "rail"
	// LayoutCorner puts a small icon in the top-right and the host at the foot.
	LayoutCorner Layout = "corner"
)

// Style is everything needed to draw one key.
type Style struct {
	Layout      Layout
	Icon        string // vecicon key: apple, nvidia, linux, server, chip, none
	HostLabel   string // e.g. "DGX-01"
	MetricLabel string // e.g. "GPU TEMP"
	ValueText   string // pre-formatted, e.g. "72"
	Unit        string // e.g. "%" or "°C"
	History     []float64
	Min, Max    float64
	Accent      color.RGBA // host accent, used for the rail and icon
	// UseAccentForValue draws the value in the accent colour instead of the
	// green/amber/red threshold ramp.
	UseAccentForValue bool
	Background        color.RGBA
}

var (
	colHost      = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	colMetric    = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	colGood      = color.RGBA{0x44, 0xFF, 0x44, 0xFF}
	colWarn      = color.RGBA{0xFF, 0xAA, 0x00, 0xFF}
	colBad       = color.RGBA{0xFF, 0x44, 0x44, 0xFF}
	defaultBG    = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	defaultAccnt = color.RGBA{0x6E, 0x9E, 0xFF, 0xFF}
)

// valueColor mirrors the familiar green/amber/red ramp: comfortable below
// half scale, warning past it, alarming past 80%.
func valueColor(frac float64) color.RGBA {
	switch {
	case frac > 0.8:
		return colBad
	case frac > 0.5:
		return colWarn
	default:
		return colGood
	}
}

// --- font handling -------------------------------------------------------

var (
	fontMu   sync.Mutex
	fontPath = "DejaVuSans-Bold.ttf"
	ttf      *truetype.Font
	faces    = map[float64]font.Face{}
)

// SetFontPath overrides where the typeface is loaded from. The plugin chdirs
// to its own directory at startup, so the default relative path resolves; the
// preview tool sets an absolute path instead.
func SetFontPath(p string) {
	fontMu.Lock()
	defer fontMu.Unlock()
	if p != fontPath {
		fontPath, ttf, faces = p, nil, map[float64]font.Face{}
	}
}

func face(size float64) (font.Face, error) {
	fontMu.Lock()
	defer fontMu.Unlock()
	if f, ok := faces[size]; ok {
		return f, nil
	}
	if ttf == nil {
		b, err := os.ReadFile(fontPath)
		if err != nil {
			return nil, err
		}
		parsed, err := truetype.Parse(b)
		if err != nil {
			return nil, err
		}
		ttf = parsed
	}
	f := truetype.NewFace(ttf, &truetype.Options{Size: size, DPI: 72, Hinting: font.HintingFull})
	faces[size] = f
	return f, nil
}

func textWidth(f font.Face, s string) int {
	return font.MeasureString(f, s).Round()
}

func drawText(dst *image.RGBA, s string, x, y int, size float64, clr color.Color) int {
	f, err := face(size)
	if err != nil {
		return 0
	}
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(clr),
		Face: f,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
	return textWidth(f, s)
}

func drawTextCentered(dst *image.RGBA, s string, cx, y int, size float64, clr color.Color) {
	f, err := face(size)
	if err != nil {
		return
	}
	drawText(dst, s, cx-textWidth(f, s)/2, y, size, clr)
}

// fitText shrinks s toward minSize until it fits maxW, then truncates with an
// ellipsis. Host names on a 72px key run out of room quickly, and a clipped
// label is worse than a slightly smaller one.
func fitText(s string, maxW int, size, minSize float64) (string, float64) {
	for sz := size; sz >= minSize; sz -= 1 {
		f, err := face(sz)
		if err != nil {
			return s, size
		}
		if textWidth(f, s) <= maxW {
			return s, sz
		}
	}
	f, err := face(minSize)
	if err != nil {
		return s, minSize
	}
	if textWidth(f, s) <= maxW {
		return s, minSize
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		cand := string(r) + "…"
		if textWidth(f, cand) <= maxW {
			return cand, minSize
		}
	}
	return string(r), minSize
}
