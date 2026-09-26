package lhmstreamdeckplugin

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/moeilijk/lhm-streamdeck/internal/cliproxy"
	"github.com/moeilijk/lhm-streamdeck/pkg/tile"
)

// hrProviderColors mirrors Headroom's per-provider rail colours, extended
// with the lab's extra providers.
var hrProviderColors = map[string]color.RGBA{
	"claude":      {0xD9, 0x77, 0x57, 0xFF},
	"codex":       {0x10, 0xA3, 0x7F, 0xFF},
	"xai":         {0xFF, 0x55, 0x00, 0xFF},
	"grok":        {0xFF, 0x55, 0x00, 0xFF},
	"antigravity": {0x42, 0x85, 0xF4, 0xFF},
	"opencode-go": {0xE8, 0xE4, 0xDF, 0xFF},
	"devin":       {0xB1, 0x8C, 0xFF, 0xFF},
}

// hrProviderIcons maps provider ids to vecicon glyph names for the corner
// mark. Providers without a recognisable brand glyph draw no icon.
var hrProviderIcons = map[string]string{
	"claude":      "claude",
	"codex":       "codex",
	"xai":         "grok",
	"grok":        "grok",
	"opencode-go": "opencode",
}

// headroomFace resolves a bound reading to its Headroom frame when the
// source runtime is a CLI Proxy service and the reading is a quota window.
func (p *Plugin) headroomFace(profileID, suid string, rid int32) (*cliproxy.QuotaFace, bool) {
	rt := p.runtimeForSource(profileID)
	rt.mu.RLock()
	hw := rt.hw
	rt.mu.RUnlock()
	svc, ok := hw.(*cliproxy.Service)
	if !ok {
		return nil, false
	}
	return svc.QuotaFace(suid, rid)
}

// headroomWindowSecs is the hero rotation cadence — session and weekly
// windows trade the centre every ~10s on the wall clock, independent of
// poll rate.
const headroomWindowSecs = 10

// renderHeadroomTile draws one Headroom-style frame. The hero rotates
// through the account's quota windows showing the *remaining* percent; the
// bottom line is the matching reset countdown in the provider accent.
// A custom host label replaces the provider header.
func (p *Plugin) renderHeadroomTile(ctx string, f *cliproxy.QuotaFace, s *actionSettings) ([]byte, error) {
	digits, line := "—", "no quota"
	if n := len(f.Windows); n > 0 {
		w := f.Windows[int(time.Now().Unix()/headroomWindowSecs)%n]
		digits = fmt.Sprintf("%.0f", 100-w.UsedPercent)
		// The window tag keeps the rotating hero attributable now that the
		// header pill is gone; a missing reset still shows which window is
		// up rather than leaving the line blank.
		line = "↻ " + cliproxy.WindowTag(w.Label)
		if rs := cliproxy.CompactReset(w.ResetAt, time.Now()); rs != "" {
			line += " " + rs
		}
	}

	label, labelShort := f.Label, f.LabelShort
	if s != nil && s.HostLabel != "" {
		label = strings.ToUpper(s.HostLabel)
	}

	img := tile.RenderHeadroom(tile.HeadroomStyle{
		Label:      label,
		LabelShort: labelShort,
		Digits:     digits,
		StatusLine: line,
		Stale:      f.Stale,
		Icon:       hrProviderIcons[f.Provider],
		Accent:     hrProviderColor(f.Provider),
	})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hrProviderColor(provider string) color.RGBA {
	if c, ok := hrProviderColors[provider]; ok {
		return c
	}
	return color.RGBA{} // zero → RenderHeadroom falls back to the default accent
}
