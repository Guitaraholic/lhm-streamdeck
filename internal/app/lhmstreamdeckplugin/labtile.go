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

	// Explicit override wins, then the tile title, then the reading's own
	// label. Hiding is separate so an override can be kept while switched off.
	metric := s.MetricLabel
	if metric == "" {
		metric = s.Title
	}
	if metric == "" {
		metric = s.ReadingLabel
	}
	if s.HideMetricLabel {
		metric = ""
	}

	min, max := float64(s.Min), float64(s.Max)
	if max <= min {
		min, max = 0, 100
	}

	img := tile.Render(tile.Style{
		Layout:      tile.LayoutRail,
		Icon:        icon,
		HostLabel:   strings.ToUpper(host),
		MetricLabel: strings.ToUpper(metric),
		ValueText:   valueText,
		Unit:        unit,
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
