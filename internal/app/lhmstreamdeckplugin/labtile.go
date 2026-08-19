package lhmstreamdeckplugin

import (
	"bytes"
	"image/color"
	"image/png"
	"strings"

	"github.com/moeilijk/lhm-streamdeck/pkg/tile"
)

// labHistoryLen is how many samples the lab tile sparkline plots. It matches
// the tile's plot width in canvas pixels closely enough that each sample gets
// roughly two pixels.
const labHistoryLen = 60

// defaultAccents give each icon a sensible colour when a profile has not set
// one, so a freshly configured host still looks deliberate.
var defaultAccents = map[string]color.RGBA{
	"apple":  {0xE6, 0xE8, 0xEB, 0xFF},
	"nvidia": {0x76, 0xB9, 0x00, 0xFF},
	"linux":  {0xE9, 0x95, 0x2C, 0xFF},
	"server": {0x6E, 0x9E, 0xFF, 0xFF},
	"chip":   {0x6E, 0x9E, 0xFF, 0xFF},
}

// pushLabHistory appends v to the tile's rolling history and returns a copy.
func (p *Plugin) pushLabHistory(ctx string, v float64) []float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	h := append(p.labHistory[ctx], v)
	if len(h) > labHistoryLen {
		h = h[len(h)-labHistoryLen:]
	}
	p.labHistory[ctx] = h
	out := make([]float64, len(h))
	copy(out, h)
	return out
}

func (p *Plugin) clearLabHistory(ctx string) {
	p.mu.Lock()
	delete(p.labHistory, ctx)
	p.mu.Unlock()
}

// labTileIdentity resolves the host name, icon and accent shown on a lab tile.
func (p *Plugin) labTileIdentity(s *actionSettings) (host, icon string, accent color.RGBA) {
	prof, _ := p.sourceProfileByID(p.resolvedSourceProfileID(s.SourceProfileID))

	host = s.HostLabel
	if host == "" {
		host = prof.Name
	}
	if host == "" {
		host = prof.Host
	}
	if host == "" {
		host = "LOCAL"
	}

	icon = prof.Icon
	if icon == "" {
		icon = "server"
	}

	if prof.Accent != "" {
		if c := hexToRGBA(prof.Accent); c != nil {
			return host, icon, *c
		}
	}
	if c, ok := defaultAccents[icon]; ok {
		return host, icon, c
	}
	return host, icon, defaultAccents["server"]
}

// renderLabTile draws the host-badged tile and encodes it for Stream Deck.
func (p *Plugin) renderLabTile(s *actionSettings, valueText, unit string, hist []float64) ([]byte, error) {
	host, icon, accent := p.labTileIdentity(s)
	metric, displayUnit := labMetricAndUnit(s, unit)

	min, max := float64(s.Min), float64(s.Max)
	if max <= min {
		min, max = 0, 100
	}

	img := tile.Render(tile.Style{
		Layout:      tile.LayoutRail,
		Icon:        icon,
		HostLabel:   strings.ToUpper(host),
		MetricLabel: labMetricDisplay(metric),
		ValueText:   valueText,
		Unit:        displayUnit,
		History:     hist,
		Min:         min,
		Max:         max,
		Accent:      accent,
	})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// labMetricAndUnit picks the middle label and the unit drawn beside the value.
// Token rates put the unit in the middle ("tok/s", "prefill/s") instead of a
// tiny, hard-to-read suffix next to the number. Percent and other units stay
// beside the value.
func labMetricAndUnit(s *actionSettings, unit string) (metric, displayUnit string) {
	metric = strings.TrimSpace(s.MetricLabel)
	if metric == "" {
		metric = strings.TrimSpace(s.Title)
	}
	if metric == "" {
		metric = strings.TrimSpace(s.ReadingLabel)
	}
	displayUnit = unit
	if strings.EqualFold(unit, "tok/s") {
		displayUnit = ""
		if s.MetricLabel == "" && s.Title == "" {
			metric = tokRateLabel(s.ReadingLabel)
		}
	}
	if s.HideMetricLabel {
		metric = ""
	}
	return metric, displayUnit
}

func tokRateLabel(reading string) string {
	switch strings.ToLower(strings.TrimSpace(reading)) {
	case "decode", "generation":
		return "tok/s"
	case "prefill":
		return "prefill/s"
	case "cached prefill":
		return "cached/s"
	case "uncached prefill":
		return "uncached/s"
	default:
		if strings.TrimSpace(reading) == "" {
			return "tok/s"
		}
		return strings.TrimSpace(reading)
	}
}

func labMetricDisplay(metric string) string {
	if metric == "" || strings.Contains(metric, "/") {
		return metric
	}
	return strings.ToUpper(metric)
}
