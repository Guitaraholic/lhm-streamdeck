package cliproxy

import (
	"fmt"
	"strings"
	"time"
)

// FaceWindow is one quota window as the rotating tile shows it: the usage
// fraction and the reset instant the countdown counts toward.
type FaceWindow struct {
	Label       string // "Session", "Daily", "Weekly", "Monthly"
	UsedPercent float64
	ResetAt     *time.Time
}

// QuotaFace is the resolved Headroom-style tile frame for one bound quota
// reading: provider identity plus every quota window the tile rotates
// through — session, then weekly, then monthly when the provider reports one.
type QuotaFace struct {
	Provider   string // "claude", "codex", "devin", …
	Label      string // top label — the provider name, e.g. "CLAUDE"
	LabelShort string // compact account-only label for tight headers
	Stale      bool
	Windows    []FaceWindow
}

// accountSensorPrefix is the sensor-ID root for per-account sensors.
const accountSensorPrefix = "/cliproxy/account/"

// quotaStaleAfter tolerates one missed refresh before the dot appears.
const quotaStaleAfter = 2 * quotaInterval

// QuotaFace resolves a bound account-sensor reading to its Headroom frame.
// Only the "… used" quota readings qualify — countdown and request readings
// render as ordinary tiles. Any used-slot resolves the same face: the tile
// itself rotates through every window the provider reports.
func (s *Service) QuotaFace(suid string, rid int32) (*QuotaFace, bool) {
	if !strings.HasPrefix(suid, accountSensorPrefix) {
		return nil, false
	}
	switch rid {
	case ReadingIDFor(suid, "/quota/session/used/0"),
		ReadingIDFor(suid, "/quota/weekly/used/0"),
		ReadingIDFor(suid, "/quota/monthly/used/0"):
	default:
		return nil, false
	}
	key := strings.TrimPrefix(suid, accountSensorPrefix)

	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.accounts[key]
	if !ok {
		return nil, false
	}
	q := s.quotas[key]

	f := &QuotaFace{
		Provider:   strings.ToLower(strings.TrimSpace(a.Provider)),
		Label:      providerLabel(a.Provider),
		LabelShort: accountShort(a),
	}
	if q == nil {
		f.Stale = true
		return f, true
	}
	f.Stale = q.FetchedAt.IsZero() || time.Since(q.FetchedAt) > quotaStaleAfter
	for _, w := range []*QuotaWindow{q.Primary, q.Secondary, q.Tertiary} {
		if w == nil {
			continue
		}
		f.Windows = append(f.Windows, FaceWindow{
			Label:       w.Label,
			UsedPercent: w.UsedPercent,
			ResetAt:     w.ResetAt,
		})
	}
	return f, true
}

// CompactReset formats a reset instant as the tight countdown the rotating
// tile shows under the remaining percentage: "5D7H" beyond a day, "2H14M"
// beyond an hour, "45M" below that, "NOW" once overdue.
func CompactReset(t *time.Time, now time.Time) string {
	if t == nil {
		return ""
	}
	d := t.Sub(now)
	if d <= 0 {
		return "NOW"
	}
	totalMin := int(d.Minutes())
	days := totalMin / (24 * 60)
	hours := totalMin % (24 * 60) / 60
	mins := totalMin % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dD%dH", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dH%dM", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dM", mins)
	default:
		return "1M"
	}
}

// WindowTag is the compact window identifier shown beside the reset
// countdown so the rotating hero stays attributable: Session→5H, Daily→DY,
// Weekly→WK, Monthly→MO.
func WindowTag(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "session":
		return "5H"
	case "daily":
		return "DY"
	case "weekly":
		return "WK"
	case "monthly":
		return "MO"
	}
	return "?"
}

// providerLabel is Headroom's short provider name (their In helper), with
// the lab's extra providers appended.
func providerLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude":
		return "CLAUDE"
	case "codex":
		return "CODEX"
	case "devin":
		return "DEVIN"
	case "xai", "grok":
		return "GROK"
	case "opencode-go":
		return "OPENCODE"
	case "antigravity":
		return "AGY"
	default:
		p := strings.ToUpper(strings.TrimSpace(provider))
		if p == "" {
			return "PROXY"
		}
		return p
	}
}

// accountShort is the compact account identity for the top label: the
// operator label when set, else the email local part, else a key prefix.
func accountShort(a Account) string {
	if l := strings.TrimSpace(a.Label); l != "" {
		return strings.ToUpper(l)
	}
	if e := strings.TrimSpace(a.Email); e != "" {
		if i := strings.IndexByte(e, '@'); i > 0 {
			return strings.ToUpper(e[:i])
		}
		return strings.ToUpper(e)
	}
	if n := strings.TrimSpace(a.Name); n != "" {
		return strings.ToUpper(n)
	}
	k := accountKey(a)
	if len(k) > 8 {
		k = k[:8]
	}
	return strings.ToUpper(k)
}
