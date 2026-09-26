package cliproxy

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// QuotaWindow is one provider rate-limit window — Headroom-style session
// usage: how much of the window is used and when it resets.
type QuotaWindow struct {
	// Label is the window name shown on the deck: "Session" (Claude 5h,
	// Codex primary), "Daily" (Devin), "Weekly", or "Monthly".
	Label       string
	UsedPercent float64
	ResetAt     *time.Time
	// WindowSeconds is the full window length, used to project the
	// end-of-window landing from elapsed fraction.
	WindowSeconds int64
}

// AccountQuota is the usage snapshot for one auth-file account. Primary is
// the short window (Claude 5-hour, Codex primary, Devin daily), Secondary is
// the long one (weekly/monthly). Either may be nil when the provider does not
// report it.
type AccountQuota struct {
	Primary   *QuotaWindow
	Secondary *QuotaWindow
	// Tertiary is a third, longest window when a provider reports one
	// (OpenCode monthly on top of rolling + weekly).
	Tertiary   *QuotaWindow
	Plan       string
	ExtraSpend *float64
	// FetchedAt marks when the snapshot was taken — a stale quota paints the
	// tile's staleness dot rather than silently showing old numbers.
	FetchedAt time.Time
}

// quotaSpec describes how one provider's usage endpoint is reached.
type quotaSpec struct {
	// apiCall usage endpoints go through POST /v0/management/api-call so the
	// proxy injects the account's own OAuth credential ($TOKEN$ placeholder).
	method  string
	url     string
	headers map[string]string
	// devin uses the lab shim on the proxy origin instead of api-call.
	originPath string
	// opencode-go serves quota from its own management plugin at
	// POST /v0/management/plugins/{plugin}/quota keyed by the auth-file
	// name stem.
	plugin string
}

func quotaSpecFor(provider string) (quotaSpec, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude":
		return quotaSpec{
			method: "GET",
			url:    "https://api.anthropic.com/api/oauth/usage",
			headers: map[string]string{
				"Authorization":  "Bearer $TOKEN$",
				"Content-Type":   "application/json",
				"anthropic-beta": "oauth-2025-04-20",
			},
		}, true
	case "codex":
		return quotaSpec{
			method: "GET",
			url:    "https://chatgpt.com/backend-api/wham/usage",
			headers: map[string]string{
				"Authorization": "Bearer $TOKEN$",
				"Content-Type":  "application/json",
				"User-Agent":    "codex-tui/0.149.1 (Mac OS 26.5.2; arm64) iTerm.app/3.6.11 (codex-tui; 0.149.1)",
			},
		}, true
	case "devin":
		return quotaSpec{originPath: "/devin/quota"}, true
	case "opencode-go":
		return quotaSpec{plugin: "opencode-go-cliproxyapi"}, true
	default:
		return quotaSpec{}, false
	}
}

// fetchQuota retrieves the usage snapshot for one account. Errors are
// reported to the caller; the service keeps the previous snapshot on failure.
func (c *Client) fetchQuota(a Account) (*AccountQuota, error) {
	spec, ok := quotaSpecFor(a.Provider)
	if !ok {
		return nil, nil
	}
	var q *AccountQuota
	var err error
	switch {
	case spec.originPath != "":
		var body json.RawMessage
		if err := c.getOrigin(spec.originPath, &body); err != nil {
			return nil, err
		}
		q, err = parseDevinQuota(body)
	case spec.plugin != "":
		keyID := strings.TrimSuffix(strings.TrimSpace(a.Name), ".json")
		if !strings.HasPrefix(keyID, "opencode-go-key-") {
			return nil, nil // auth file doesn't carry a usable key id
		}
		var body json.RawMessage
		if err := c.pluginQuota(spec.plugin, keyID, &body); err != nil {
			return nil, err
		}
		q, err = parseOpenCodeQuota(body)
	default:
		body, statusCode, err := c.APICall(a.AuthIndex.String(), spec.method, spec.url, spec.headers)
		if err != nil {
			return nil, err
		}
		if statusCode < 200 || statusCode >= 300 {
			return nil, fmt.Errorf("quota request failed (HTTP %d)", statusCode)
		}
		switch strings.ToLower(strings.TrimSpace(a.Provider)) {
		case "claude":
			q, err = parseClaudeQuota(body)
		case "codex":
			q, err = parseCodexQuota(body)
		}
	}
	if err != nil {
		return nil, err
	}
	if q != nil {
		q.FetchedAt = time.Now()
	}
	return q, nil
}

// --- Claude: GET https://api.anthropic.com/api/oauth/usage ---

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type claudeUsagePayload struct {
	FiveHour         *claudeUsageWindow `json:"five_hour"`
	SevenDay         *claudeUsageWindow `json:"seven_day"`
	SevenDaySonnet   *claudeUsageWindow `json:"seven_day_sonnet"`
	SevenDayOpus     *claudeUsageWindow `json:"seven_day_opus"`
	SevenDayCowork   *claudeUsageWindow `json:"seven_day_cowork"`
	SevenDayOAuthApp *claudeUsageWindow `json:"seven_day_oauth_apps"`
	ExtraUsage       *struct {
		IsEnabled    bool     `json:"is_enabled"`
		MonthlyLimit float64  `json:"monthly_limit"`
		UsedCredits  *float64 `json:"used_credits"`
		Utilization  *float64 `json:"utilization"`
	} `json:"extra_usage"`
}

func parseClaudeQuota(body json.RawMessage) (*AccountQuota, error) {
	var p claudeUsagePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode claude usage: %w", err)
	}
	q := &AccountQuota{}
	if w := claudeWindow(p.FiveHour, "Session"); w != nil {
		q.Primary = w
	}
	if w := claudeWindow(p.SevenDay, "Weekly"); w != nil {
		q.Secondary = w
	}
	if q.Primary == nil && q.Secondary == nil {
		return nil, fmt.Errorf("claude usage: no quota windows")
	}
	if p.ExtraUsage != nil && p.ExtraUsage.IsEnabled && p.ExtraUsage.UsedCredits != nil {
		spend := *p.ExtraUsage.UsedCredits
		q.ExtraSpend = &spend
	}
	return q, nil
}

func claudeWindow(w *claudeUsageWindow, label string) *QuotaWindow {
	if w == nil {
		return nil
	}
	out := &QuotaWindow{Label: label, UsedPercent: w.Utilization, WindowSeconds: windowSecondsFor(label)}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(w.ResetsAt)); err == nil {
		out.ResetAt = &t
	}
	return out
}

// --- Codex: GET https://chatgpt.com/backend-api/wham/usage ---

type codexUsageWindow struct {
	UsedPercent        *float64     `json:"used_percent"`
	LimitWindowSeconds *json.Number `json:"limit_window_seconds"`
	ResetAfterSeconds  *json.Number `json:"reset_after_seconds"`
	ResetAt            flexString   `json:"reset_at"`
}

type codexRateLimit struct {
	Allowed         *bool             `json:"allowed"`
	LimitReached    *bool             `json:"limit_reached"`
	PrimaryWindow   *codexUsageWindow `json:"primary_window"`
	SecondaryWindow *codexUsageWindow `json:"secondary_window"`
}

type codexUsagePayload struct {
	PlanType  flexString      `json:"plan_type"`
	RateLimit *codexRateLimit `json:"rate_limit"`
}

const (
	codexSessionWindowSeconds = 5 * 3600
	codexWeekWindowSeconds    = 7 * 24 * 3600
)

func parseCodexQuota(body json.RawMessage) (*AccountQuota, error) {
	var p codexUsagePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode codex usage: %w", err)
	}
	q := &AccountQuota{Plan: strings.TrimSpace(p.PlanType.String())}
	rl := p.RateLimit
	if rl == nil {
		return nil, fmt.Errorf("codex usage: no rate limit")
	}
	limitReached := (rl.LimitReached != nil && *rl.LimitReached) || (rl.Allowed != nil && !*rl.Allowed)
	// Slot follows payload position — primary_window is whatever window is
	// currently binding, regardless of its length — while the label follows
	// the real limit_window_seconds so a 7-day primary_window reads
	// "Weekly"/WK rather than lying about being a 5h session.
	for i, s := range []struct {
		w        *codexUsageWindow
		fallback string
	}{
		{rl.PrimaryWindow, "Session"},
		{rl.SecondaryWindow, "Weekly"},
	} {
		if s.w == nil {
			continue
		}
		label := codexWindowLabel(s.w)
		if codexWindowSeconds(s.w) == 0 {
			// Legacy payloads without limit_window_seconds keep the
			// positional guess: primary → session, secondary → weekly.
			label = s.fallback
		}
		win := codexWindow(s.w, label, limitReached)
		if i == 0 {
			q.Primary = win
		} else {
			q.Secondary = win
		}
	}
	if q.Primary == nil && q.Secondary == nil {
		return nil, fmt.Errorf("codex usage: no quota windows")
	}
	return q, nil
}

// codexWindowLabel classifies a window by its duration: anything up to ~6h
// is the session window; week-or-longer windows are the weekly/monthly one.
func codexWindowLabel(w *codexUsageWindow) string {
	secs := codexWindowSeconds(w)
	switch {
	case secs > 0 && secs <= 6*3600:
		return "Session"
	case secs >= codexWeekWindowSeconds && secs <= 31*24*3600:
		if secs > codexWeekWindowSeconds {
			return "Monthly"
		}
		return "Weekly"
	case secs > 31*24*3600:
		return "Monthly"
	default:
		return "Weekly"
	}
}

func codexWindowSeconds(w *codexUsageWindow) int64 {
	if w.LimitWindowSeconds == nil {
		return 0
	}
	n, err := w.LimitWindowSeconds.Int64()
	if err != nil {
		return 0
	}
	return n
}

func codexWindow(w *codexUsageWindow, label string, limitReached bool) *QuotaWindow {
	out := &QuotaWindow{Label: label, WindowSeconds: codexWindowSeconds(w)}
	if out.WindowSeconds == 0 {
		out.WindowSeconds = windowSecondsFor(label)
	}
	if w.UsedPercent != nil {
		out.UsedPercent = *w.UsedPercent
	} else if limitReached {
		out.UsedPercent = 100
	}
	if raw := strings.TrimSpace(w.ResetAt.String()); raw != "" {
		// ChatGPT emits unix seconds for reset_at.
		if n, err := json.Number(raw).Int64(); err == nil {
			t := time.Unix(n, 0)
			out.ResetAt = &t
		} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
			out.ResetAt = &t
		}
	}
	if out.ResetAt == nil && w.ResetAfterSeconds != nil {
		if secs, err := w.ResetAfterSeconds.Int64(); err == nil {
			t := time.Now().Add(time.Duration(secs) * time.Second)
			out.ResetAt = &t
		}
	}
	return out
}

// --- Devin: lab shim at GET {origin}/devin/quota ---

type devinShimWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resets_at"`
}

type devinShimQuota struct {
	Plan  *string `json:"plan"`
	Usage *struct {
		Daily  *devinShimWindow `json:"daily"`
		Weekly *devinShimWindow `json:"weekly"`
	} `json:"usage"`
	ExtraUsageBalanceUsd *float64 `json:"extra_usage_balance_usd"`
}

func parseDevinQuota(body json.RawMessage) (*AccountQuota, error) {
	var p devinShimQuota
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode devin quota: %w", err)
	}
	if p.Usage == nil {
		return nil, fmt.Errorf("devin quota: no usage")
	}
	q := &AccountQuota{}
	if p.Plan != nil {
		q.Plan = strings.TrimSpace(*p.Plan)
	}
	if w := devinWindow(p.Usage.Daily, "Daily"); w != nil {
		q.Primary = w
	}
	if w := devinWindow(p.Usage.Weekly, "Weekly"); w != nil {
		q.Secondary = w
	}
	if q.Primary == nil && q.Secondary == nil {
		return nil, fmt.Errorf("devin quota: no quota windows")
	}
	q.ExtraSpend = p.ExtraUsageBalanceUsd
	return q, nil
}

func devinWindow(w *devinShimWindow, label string) *QuotaWindow {
	if w == nil {
		return nil
	}
	out := &QuotaWindow{Label: label, UsedPercent: w.Percent, WindowSeconds: windowSecondsFor(label)}
	if strings.EqualFold(strings.TrimSpace(w.Status), "rate-limited") {
		out.UsedPercent = 100
	}
	out.UsedPercent = math.Max(0, math.Min(100, out.UsedPercent))
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(w.ResetsAt)); err == nil {
		out.ResetAt = &t
	}
	return out
}

// --- OpenCode Go: POST /v0/management/plugins/opencode-go-cliproxyapi/quota ---
// The account's own management plugin fetches {upstream}/usage with the
// credential and reports rolling (~5h), weekly and monthly windows.

type openCodeWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resets_at"`
}

type openCodeQuotaCard struct {
	Usage *struct {
		Rolling *openCodeWindow `json:"rolling"`
		Weekly  *openCodeWindow `json:"weekly"`
		Monthly *openCodeWindow `json:"monthly"`
	} `json:"usage"`
}

func parseOpenCodeQuota(body json.RawMessage) (*AccountQuota, error) {
	var p openCodeQuotaCard
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode opencode quota: %w", err)
	}
	if p.Usage == nil {
		return nil, fmt.Errorf("opencode quota: no usage")
	}
	q := &AccountQuota{
		Primary:   openCodeWindowToQuota(p.Usage.Rolling, "Session"),
		Secondary: openCodeWindowToQuota(p.Usage.Weekly, "Weekly"),
		Tertiary:  openCodeWindowToQuota(p.Usage.Monthly, "Monthly"),
	}
	if q.Primary == nil && q.Secondary == nil && q.Tertiary == nil {
		return nil, fmt.Errorf("opencode quota: no quota windows")
	}
	return q, nil
}

func openCodeWindowToQuota(w *openCodeWindow, label string) *QuotaWindow {
	if w == nil {
		return nil
	}
	out := &QuotaWindow{Label: label, UsedPercent: w.Percent, WindowSeconds: windowSecondsFor(label)}
	if strings.EqualFold(strings.TrimSpace(w.Status), "rate-limited") {
		out.UsedPercent = 100
	}
	out.UsedPercent = math.Max(0, math.Min(100, out.UsedPercent))
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(w.ResetsAt)); err == nil {
		out.ResetAt = &t
	}
	return out
}

// windowSecondsFor returns the canonical length for a labelled window when a
// provider payload doesn't carry it explicitly.
func windowSecondsFor(label string) int64 {
	switch strings.ToLower(label) {
	case "session":
		return 5 * 3600
	case "daily":
		return 24 * 3600
	case "weekly":
		return 7 * 24 * 3600
	case "monthly":
		return 30 * 24 * 3600
	default:
		return 0
	}
}
