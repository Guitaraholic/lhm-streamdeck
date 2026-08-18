// Package vecicon rasterizes SVG path data into images so brand marks and
// glyphs can be drawn at any size without shipping per-size bitmaps.
package vecicon

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/image/vector"
)

// scanner tokenizes SVG path data. Numbers may be separated by whitespace,
// commas, or nothing at all when the sign or decimal point is unambiguous
// (".5.5" is two numbers, "-.5-.5" likewise).
type scanner struct {
	s   string
	pos int
}

func (sc *scanner) skipSep() {
	for sc.pos < len(sc.s) {
		c := sc.s[sc.pos]
		if c == ' ' || c == ',' || c == '\t' || c == '\n' || c == '\r' {
			sc.pos++
			continue
		}
		return
	}
}

func (sc *scanner) peekCmd() (byte, bool) {
	sc.skipSep()
	if sc.pos >= len(sc.s) {
		return 0, false
	}
	c := sc.s[sc.pos]
	if unicode.IsLetter(rune(c)) {
		return c, true
	}
	return 0, false
}

func (sc *scanner) number() (float64, bool) {
	sc.skipSep()
	start := sc.pos
	if sc.pos < len(sc.s) && (sc.s[sc.pos] == '+' || sc.s[sc.pos] == '-') {
		sc.pos++
	}
	seenDot, seenDigit := false, false
	for sc.pos < len(sc.s) {
		c := sc.s[sc.pos]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			sc.pos++
		case c == '.' && !seenDot:
			seenDot = true
			sc.pos++
		case (c == 'e' || c == 'E') && seenDigit:
			// exponent: consume and allow one optional sign
			sc.pos++
			if sc.pos < len(sc.s) && (sc.s[sc.pos] == '+' || sc.s[sc.pos] == '-') {
				sc.pos++
			}
		default:
			goto done
		}
	}
done:
	if !seenDigit {
		sc.pos = start
		return 0, false
	}
	return parseFloat(sc.s[start:sc.pos]), true
}

// flag reads a single-character 0/1 arc flag, which SVG permits to be packed
// against its neighbours ("0 0 1" and "001" are both valid).
func (sc *scanner) flag() (bool, bool) {
	sc.skipSep()
	if sc.pos >= len(sc.s) {
		return false, false
	}
	switch sc.s[sc.pos] {
	case '0':
		sc.pos++
		return false, true
	case '1':
		sc.pos++
		return true, true
	}
	return false, false
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// Rasterize fills the SVG path data d (authored in a viewBox of size
// viewBox x viewBox) into a rasterizer of the given pixel size.
func Rasterize(r *vector.Rasterizer, d string, viewBox, size float64) {
	k := size / viewBox
	sc := &scanner{s: strings.TrimSpace(d)}

	var cx, cy float64   // current point
	var sx, sy float64   // subpath start
	var pcx, pcy float64 // previous cubic control point (for S/s)
	var lastCmd byte     // previous command letter
	open := false

	moveTo := func(x, y float64) {
		if open {
			r.ClosePath()
		}
		r.MoveTo(float32(x*k), float32(y*k))
		open = true
	}

	cmd := byte(0)
	for {
		if c, ok := sc.peekCmd(); ok {
			cmd = c
			sc.pos++
		} else {
			if cmd == 0 {
				break
			}
			// Implicit repeat: the previous command runs again on the next
			// argument tuple. Probe for a number, then rewind so the argument
			// loop below reads it. A repeated moveto becomes a lineto.
			save := sc.pos
			if _, ok := sc.number(); !ok {
				break
			}
			sc.pos = save
			if cmd == 'M' {
				cmd = 'L'
			} else if cmd == 'm' {
				cmd = 'l'
			}
		}

		rel := cmd >= 'a' && cmd <= 'z'
		switch upper(cmd) {
		case 'M':
			x, ok1 := sc.number()
			y, ok2 := sc.number()
			if !ok1 || !ok2 {
				return
			}
			if rel {
				x, y = cx+x, cy+y
			}
			cx, cy, sx, sy = x, y, x, y
			moveTo(x, y)
		case 'L':
			x, ok1 := sc.number()
			y, ok2 := sc.number()
			if !ok1 || !ok2 {
				return
			}
			if rel {
				x, y = cx+x, cy+y
			}
			cx, cy = x, y
			r.LineTo(float32(x*k), float32(y*k))
		case 'H':
			x, ok := sc.number()
			if !ok {
				return
			}
			if rel {
				x = cx + x
			}
			cx = x
			r.LineTo(float32(cx*k), float32(cy*k))
		case 'V':
			y, ok := sc.number()
			if !ok {
				return
			}
			if rel {
				y = cy + y
			}
			cy = y
			r.LineTo(float32(cx*k), float32(cy*k))
		case 'C', 'S':
			var x1, y1 float64
			if upper(cmd) == 'S' {
				// reflect the previous control point about the current point
				if upper(lastCmd) == 'C' || upper(lastCmd) == 'S' {
					x1, y1 = 2*cx-pcx, 2*cy-pcy
				} else {
					x1, y1 = cx, cy
				}
			} else {
				a, ok1 := sc.number()
				b, ok2 := sc.number()
				if !ok1 || !ok2 {
					return
				}
				x1, y1 = a, b
				if rel {
					x1, y1 = cx+a, cy+b
				}
			}
			x2, ok3 := sc.number()
			y2, ok4 := sc.number()
			x, ok5 := sc.number()
			y, ok6 := sc.number()
			if !ok3 || !ok4 || !ok5 || !ok6 {
				return
			}
			if rel {
				x2, y2, x, y = cx+x2, cy+y2, cx+x, cy+y
			}
			r.CubeTo(float32(x1*k), float32(y1*k), float32(x2*k), float32(y2*k), float32(x*k), float32(y*k))
			pcx, pcy = x2, y2
			cx, cy = x, y
		case 'Q', 'T':
			var x1, y1 float64
			if upper(cmd) == 'T' {
				if upper(lastCmd) == 'Q' || upper(lastCmd) == 'T' {
					x1, y1 = 2*cx-pcx, 2*cy-pcy
				} else {
					x1, y1 = cx, cy
				}
			} else {
				a, ok1 := sc.number()
				b, ok2 := sc.number()
				if !ok1 || !ok2 {
					return
				}
				x1, y1 = a, b
				if rel {
					x1, y1 = cx+a, cy+b
				}
			}
			x, ok3 := sc.number()
			y, ok4 := sc.number()
			if !ok3 || !ok4 {
				return
			}
			if rel {
				x, y = cx+x, cy+y
			}
			r.QuadTo(float32(x1*k), float32(y1*k), float32(x*k), float32(y*k))
			pcx, pcy = x1, y1
			cx, cy = x, y
		case 'A':
			rx, ok1 := sc.number()
			ry, ok2 := sc.number()
			rot, ok3 := sc.number()
			large, ok4 := sc.flag()
			sweep, ok5 := sc.flag()
			x, ok6 := sc.number()
			y, ok7 := sc.number()
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
				return
			}
			if rel {
				x, y = cx+x, cy+y
			}
			arcTo(r, k, cx, cy, rx, ry, rot, large, sweep, x, y)
			cx, cy = x, y
		case 'Z':
			if open {
				r.ClosePath()
				open = false
			}
			cx, cy = sx, sy
			r.MoveTo(float32(cx*k), float32(cy*k))
			open = true
		default:
			return
		}
		lastCmd = cmd
	}
	if open {
		r.ClosePath()
	}
}

// arcTo converts an SVG endpoint-parameterized elliptical arc into cubic
// Béziers (endpoint -> center parameterization, per SVG spec appendix F.6).
func arcTo(r *vector.Rasterizer, k, x1, y1, rx, ry, rotDeg float64, large, sweep bool, x2, y2 float64) {
	if rx == 0 || ry == 0 {
		r.LineTo(float32(x2*k), float32(y2*k))
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := rotDeg * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)

	dx2, dy2 := (x1-x2)/2, (y1-y2)/2
	x1p := cosP*dx2 + sinP*dy2
	y1p := -sinP*dx2 + cosP*dy2

	// scale radii up if they cannot span the endpoints
	lambda := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx, ry = rx*s, ry*s
	}

	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	if den == 0 {
		r.LineTo(float32(x2*k), float32(y2*k))
		return
	}
	if num < 0 {
		num = 0
	}
	co := math.Sqrt(num / den)
	if large == sweep {
		co = -co
	}
	cxp := co * rx * y1p / ry
	cyp := -co * ry * x1p / rx

	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2

	ang := func(ux, uy, vx, vy float64) float64 {
		dot := ux*vx + uy*vy
		len := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		if len == 0 {
			return 0
		}
		c := dot / len
		if c < -1 {
			c = -1
		} else if c > 1 {
			c = 1
		}
		a := math.Acos(c)
		if ux*vy-uy*vx < 0 {
			a = -a
		}
		return a
	}

	theta1 := ang(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dTheta := ang((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dTheta > 0 {
		dTheta -= 2 * math.Pi
	} else if sweep && dTheta < 0 {
		dTheta += 2 * math.Pi
	}

	// one cubic segment per <=90 degrees keeps the approximation error tiny
	segs := int(math.Ceil(math.Abs(dTheta) / (math.Pi / 2)))
	if segs < 1 {
		segs = 1
	}
	delta := dTheta / float64(segs)
	t := 4.0 / 3.0 * math.Tan(delta/4)

	th := theta1
	for i := 0; i < segs; i++ {
		cosT1, sinT1 := math.Cos(th), math.Sin(th)
		th2 := th + delta
		cosT2, sinT2 := math.Cos(th2), math.Sin(th2)

		e := func(ct, st float64) (float64, float64) {
			return cx + rx*cosP*ct - ry*sinP*st, cy + rx*sinP*ct + ry*cosP*st
		}
		ed := func(ct, st float64) (float64, float64) {
			return -rx*cosP*st - ry*sinP*ct, -rx*sinP*st + ry*cosP*ct
		}
		px1, py1 := e(cosT1, sinT1)
		px2, py2 := e(cosT2, sinT2)
		dx1, dy1 := ed(cosT1, sinT1)
		dxe, dye := ed(cosT2, sinT2)

		r.CubeTo(
			float32((px1+t*dx1)*k), float32((py1+t*dy1)*k),
			float32((px2-t*dxe)*k), float32((py2-t*dye)*k),
			float32(px2*k), float32(py2*k),
		)
		th = th2
	}
}

func upper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

// RasterizeForTest is an alias of Rasterize used by tests in sibling packages.
func RasterizeForTest(r *vector.Rasterizer, d string, viewBox, size float64) {
	Rasterize(r, d, viewBox, size)
}
