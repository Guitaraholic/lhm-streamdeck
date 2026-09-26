package cliproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

// pollForReading polls until the named reading appears on the sensor or the
// deadline passes — quota refresh is async off the poll path.
func pollForReading(t *testing.T, svc *Service, sensorID, label string) []hwsensorsservice.Reading {
	t.Helper()
	for i := 0; i < 50; i++ {
		if _, err := svc.PollTime(); err != nil {
			t.Fatal(err)
		}
		rs, err := svc.ReadingsForSensorID(sensorID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			if r.Label() == label {
				return rs
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func claudeUsageBody() map[string]any {
	return map[string]any{
		"five_hour": map[string]any{
			"utilization": 42.5,
			"resets_at":   time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339),
		},
		"seven_day": map[string]any{
			"utilization": 63,
			"resets_at":   time.Now().Add(96 * time.Hour).UTC().Format(time.RFC3339),
		},
		"extra_usage": map[string]any{
			"is_enabled":    true,
			"monthly_limit": 50,
			"used_credits":  12.34,
		},
	}
}

func codexUsageBody() map[string]any {
	return map[string]any{
		"plan_type": "pro",
		"rate_limit": map[string]any{
			"allowed":       true,
			"limit_reached": false,
			"primary_window": map[string]any{
				"used_percent":         55,
				"limit_window_seconds": 18000,
				"reset_after_seconds":  3600,
			},
			"secondary_window": map[string]any{
				"used_percent":         20,
				"limit_window_seconds": 604800,
				"reset_at":             time.Now().Add(72 * time.Hour).Unix(),
			},
		},
	}
}

// quotaServer serves auth-files plus the api-call passthrough, returning
// per-auth-index provider payloads.
func quotaServer(t *testing.T, apiBodies map[string]any, apiCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(testAccountsPayload())
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			if apiCalls != nil {
				apiCalls.Add(1)
			}
			var req apiCallRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("api-call decode: %v", err)
			}
			if req.Header["Authorization"] != "Bearer $TOKEN$" {
				t.Errorf("api-call auth header = %q", req.Header["Authorization"])
			}
			body, ok := apiBodies[req.AuthIndex]
			if !ok {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 404, "body": map[string]any{"error": "no such account"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 200,
				"header":      map[string]any{},
				"body":        body,
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestAPICallRoundTrip(t *testing.T) {
	srv := quotaServer(t, map[string]any{"idx1": claudeUsageBody()}, nil)
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port, "k")

	body, code, err := c.APICall("idx1", "GET", "https://api.anthropic.com/api/oauth/usage", map[string]string{"Authorization": "Bearer $TOKEN$"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("status_code = %d", code)
	}
	var usage map[string]any
	if err := json.Unmarshal(body, &usage); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if _, ok := usage["five_hour"]; !ok {
		t.Fatalf("body missing five_hour: %s", body)
	}
}

func TestAPICallStringEncodedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status_code": 200,
			"body":        `{"five_hour":{"utilization":7,"resets_at":null}}`,
		})
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port, "k")

	body, code, err := c.APICall("idx", "GET", "https://x", nil)
	if err != nil || code != 200 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	var usage struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
		} `json:"five_hour"`
	}
	if err := json.Unmarshal(body, &usage); err != nil {
		t.Fatalf("string body unwrap failed: %v", err)
	}
	if usage.FiveHour.Utilization != 7 {
		t.Fatalf("utilization = %v", usage.FiveHour.Utilization)
	}
}

func TestParseClaudeQuota(t *testing.T) {
	raw, _ := json.Marshal(claudeUsageBody())
	q, err := parseClaudeQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Primary == nil || q.Primary.Label != "Session" || q.Primary.UsedPercent != 42.5 {
		t.Fatalf("primary = %+v", q.Primary)
	}
	if q.Primary.ResetAt == nil {
		t.Fatal("primary reset missing")
	}
	if mins := time.Until(*q.Primary.ResetAt).Minutes(); mins < 89 || mins > 91 {
		t.Fatalf("primary reset ~90min, got %v", mins)
	}
	if q.Secondary == nil || q.Secondary.Label != "Weekly" || q.Secondary.UsedPercent != 63 {
		t.Fatalf("secondary = %+v", q.Secondary)
	}
	if q.ExtraSpend == nil || *q.ExtraSpend != 12.34 {
		t.Fatalf("spend = %v", q.ExtraSpend)
	}
}

func TestParseClaudeQuotaEmptyWindows(t *testing.T) {
	if _, err := parseClaudeQuota(json.RawMessage(`{"five_hour":null,"seven_day":null}`)); err == nil {
		t.Fatal("expected no-windows error")
	}
}

func TestParseCodexQuota(t *testing.T) {
	raw, _ := json.Marshal(codexUsageBody())
	q, err := parseCodexQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Plan != "pro" {
		t.Fatalf("plan = %q", q.Plan)
	}
	if q.Primary == nil || q.Primary.Label != "Session" || q.Primary.UsedPercent != 55 {
		t.Fatalf("primary = %+v", q.Primary)
	}
	if q.Primary.ResetAt == nil {
		t.Fatal("primary reset missing (reset_after_seconds)")
	}
	if q.Secondary == nil || q.Secondary.Label != "Weekly" || q.Secondary.UsedPercent != 20 {
		t.Fatalf("secondary = %+v", q.Secondary)
	}
}

func TestParseCodexQuotaMonthlySecondary(t *testing.T) {
	body := map[string]any{
		"rate_limit": map[string]any{
			"primary_window":   map[string]any{"used_percent": 10, "limit_window_seconds": 18000},
			"secondary_window": map[string]any{"used_percent": 40, "limit_window_seconds": 2592000},
		},
	}
	raw, _ := json.Marshal(body)
	q, err := parseCodexQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Secondary == nil || q.Secondary.Label != "Monthly" {
		t.Fatalf("secondary = %+v", q.Secondary)
	}
}

func TestParseCodexQuotaWeeklyPrimary(t *testing.T) {
	// Live wham/usage puts the binding window in primary_window — for
	// rate-limited business accounts that is the 7-day credit window. The
	// window must keep its real label rather than masquerade as a session.
	body := map[string]any{
		"plan_type": "self_serve_business_prolite",
		"rate_limit": map[string]any{
			"allowed":       false,
			"limit_reached": true,
			"primary_window": map[string]any{
				"used_percent":         100,
				"limit_window_seconds": 604800,
				"reset_after_seconds":  405904,
			},
			"secondary_window": nil,
		},
	}
	raw, _ := json.Marshal(body)
	q, err := parseCodexQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Primary == nil || q.Primary.Label != "Weekly" || q.Primary.UsedPercent != 100 {
		t.Fatalf("weekly-labelled primary = %+v", q.Primary)
	}
	if q.Primary.WindowSeconds != 604800 {
		t.Fatalf("primary window seconds = %d", q.Primary.WindowSeconds)
	}
	if q.Secondary != nil {
		t.Fatalf("same window must not duplicate into secondary: %+v", q.Secondary)
	}
}

func TestParseCodexQuotaLimitReached(t *testing.T) {
	body := map[string]any{
		"rate_limit": map[string]any{
			"limit_reached": true,
			"primary_window": map[string]any{
				"limit_window_seconds": 18000,
				"reset_after_seconds":  1200,
			},
		},
	}
	raw, _ := json.Marshal(body)
	q, err := parseCodexQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Primary == nil || q.Primary.UsedPercent != 100 {
		t.Fatalf("limit-reached primary = %+v", q.Primary)
	}
}

func TestParseOpenCodeQuota(t *testing.T) {
	raw := json.RawMessage(`{
		"key_id": "opencode-go-key-abc123",
		"label": "OpenCode Go credential abc123",
		"usage": {
			"rolling": {"status": "ok", "percent": 12, "resets_at": "2030-01-01T05:00:00Z"},
			"weekly": {"status": "ok", "percent": 40, "resets_at": "2030-01-08T00:00:00Z"},
			"monthly": {"status": "rate-limited", "percent": 50, "resets_at": "2030-02-01T00:00:00Z"}
		}
	}`)
	q, err := parseOpenCodeQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Primary == nil || q.Primary.Label != "Session" || q.Primary.UsedPercent != 12 {
		t.Fatalf("rolling primary = %+v", q.Primary)
	}
	if q.Secondary == nil || q.Secondary.Label != "Weekly" || q.Secondary.UsedPercent != 40 {
		t.Fatalf("weekly secondary = %+v", q.Secondary)
	}
	if q.Tertiary == nil || q.Tertiary.Label != "Monthly" || q.Tertiary.UsedPercent != 100 {
		t.Fatalf("rate-limited monthly must clamp to 100: %+v", q.Tertiary)
	}
}

func TestOpenCodeQuotaEndToEnd(t *testing.T) {
	var gotKeyID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
				"auth_index": "ed188232dbf80b4a",
				"provider":   "opencode-go",
				"name":       "opencode-go-key-7f98dc091f5591249733b83010da5381b6a88ecfd3cd2065804c1006097563eb.json",
				"label":      "OpenCode Go credential 7f98dc091f55",
				"status":     "ready",
			}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/plugins/opencode-go-cliproxyapi/quota":
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotKeyID = req["key_id"]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key_id": gotKeyID,
				"usage": map[string]any{
					"rolling": map[string]any{"status": "ok", "percent": 0, "resets_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)},
					"weekly":  map[string]any{"status": "ok", "percent": 0, "resets_at": time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)},
					"monthly": map[string]any{"status": "ok", "percent": 50, "resets_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewServiceWithClient(NewClient(host, port, "k"))
	suid := "/cliproxy/account/ed188232dbf80b4a"

	readings := pollForReading(t, svc, suid, "Monthly used")
	if readings == nil {
		t.Fatal("monthly quota reading never appeared")
	}
	if gotKeyID != "opencode-go-key-7f98dc091f5591249733b83010da5381b6a88ecfd3cd2065804c1006097563eb" {
		t.Fatalf("key_id sent = %q", gotKeyID)
	}
	f, ok := svc.QuotaFace(suid, ReadingIDFor(suid, "/quota/monthly/used/0"))
	if !ok {
		t.Fatal("monthly reading should resolve to a face")
	}
	if len(f.Windows) == 0 || f.Windows[len(f.Windows)-1].Label != "Monthly" || f.Windows[len(f.Windows)-1].UsedPercent != 50 {
		t.Fatalf("monthly face windows = %+v", f.Windows)
	}
}

func TestParseDevinQuota(t *testing.T) {
	raw := json.RawMessage(`{
		"provider": "devin",
		"plan": "max",
		"usage": {
			"daily":  {"status": "ok", "percent": 33, "resets_at": "` + time.Now().Add(2*time.Hour).UTC().Format(time.RFC3339) + `"},
			"weekly": {"status": "rate-limited", "percent": 88, "resets_at": null}
		},
		"extra_usage_balance_usd": 4.5
	}`)
	q, err := parseDevinQuota(raw)
	if err != nil {
		t.Fatal(err)
	}
	if q.Plan != "max" {
		t.Fatalf("plan = %q", q.Plan)
	}
	if q.Primary == nil || q.Primary.Label != "Daily" || q.Primary.UsedPercent != 33 {
		t.Fatalf("primary = %+v", q.Primary)
	}
	// Rate-limited window reports 100% regardless of percent.
	if q.Secondary == nil || q.Secondary.UsedPercent != 100 {
		t.Fatalf("secondary = %+v", q.Secondary)
	}
	if q.ExtraSpend == nil || *q.ExtraSpend != 4.5 {
		t.Fatalf("balance = %v", q.ExtraSpend)
	}
}

func TestServiceQuotaReadings(t *testing.T) {
	var apiCalls atomic.Int32
	srv := quotaServer(t, map[string]any{
		"a1b2c3d4e5f67890": claudeUsageBody(),
		"c3d4e5f67890a1b2": codexUsageBody(),
	}, &apiCalls)
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")

	readings := pollForReading(t, svc, "/cliproxy/account/a1b2c3d4e5f67890", "Session used")
	if readings == nil {
		t.Fatal("quota readings never appeared")
	}
	byLabel := map[string]float64{}
	for _, r := range readings {
		byLabel[r.Label()] = r.Value()
	}
	if byLabel["Session used"] != 42.5 {
		t.Fatalf("session used = %v", byLabel["Session used"])
	}
	if byLabel["Weekly used"] != 63 {
		t.Fatalf("weekly used = %v", byLabel["Weekly used"])
	}
	reset, ok := byLabel["Session resets in"]
	if !ok || reset < 89 || reset > 91 {
		t.Fatalf("session reset = %v (ok=%v)", reset, ok)
	}
	if byLabel["Extra usage spend"] != 12.34 {
		t.Fatalf("spend = %v", byLabel["Extra usage spend"])
	}

	// Codex account gets the same window labels from its own payload.
	codex := pollForReading(t, svc, "/cliproxy/account/c3d4e5f67890a1b2", "Session used")
	if codex == nil {
		t.Fatal("codex quota readings never appeared")
	}
	cb := map[string]float64{}
	for _, r := range codex {
		cb[r.Label()] = r.Value()
	}
	if cb["Session used"] != 55 || cb["Weekly used"] != 20 {
		t.Fatalf("codex quota = %+v", cb)
	}

	// Second claude account has no apiBodies entry → upstream 404 → no quota
	// readings, but the sensor still exists.
	second, err := svc.ReadingsForSensorID("/cliproxy/account/b2c3d4e5f67890a1")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range second {
		if r.Label() == "Session used" {
			t.Fatal("quota readings present for failed quota fetch")
		}
	}

	// Let the async batch fully settle, then a repoll within the interval must
	// not re-hit the provider APIs.
	time.Sleep(50 * time.Millisecond)
	before := apiCalls.Load()
	if _, err := svc.PollTime(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := apiCalls.Load(); got != before {
		t.Fatalf("quota re-fetched inside interval: %d → %d", before, got)
	}
}

func TestServiceDevinQuotaViaOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "d1", "auth_index": "dev1", "provider": "devin", "status": "ready"},
				},
			})
		case r.URL.Path == "/devin/quota":
			if r.Header.Get("Authorization") != "" {
				t.Error("devin shim should get no auth header")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plan": "max",
				"usage": map[string]any{
					"daily":  map[string]any{"status": "ok", "percent": 20, "resets_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
					"weekly": map[string]any{"status": "ok", "percent": 60, "resets_at": nil},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")
	readings := pollForReading(t, svc, "/cliproxy/account/dev1", "Daily used")
	if readings == nil {
		t.Fatal("devin quota readings never appeared")
	}
	byLabel := map[string]float64{}
	for _, r := range readings {
		byLabel[r.Label()] = r.Value()
	}
	if byLabel["Daily used"] != 20 || byLabel["Weekly used"] != 60 {
		t.Fatalf("devin quota = %+v", byLabel)
	}
	// Plan lands in the sensor name.
	sensors, _ := svc.Sensors()
	if sensors[1].Name() != "dev1 (devin) · max" {
		t.Fatalf("sensor name = %q", sensors[1].Name())
	}
}

func TestServiceQuotaRefreshAfterInterval(t *testing.T) {
	var apiCalls atomic.Int32
	srv := quotaServer(t, map[string]any{"a1b2c3d4e5f67890": claudeUsageBody()}, &apiCalls)
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")
	svc.quotaInterval = time.Millisecond

	if _, err := svc.PollTime(); err != nil {
		t.Fatal(err)
	}
	// Async: wait for the first batch to finish before counting.
	deadline := time.Now().Add(2 * time.Second)
	for apiCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	first := apiCalls.Load()
	time.Sleep(2 * time.Millisecond)
	if _, err := svc.PollTime(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for apiCalls.Load() <= first && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if apiCalls.Load() <= first {
		t.Fatalf("quota not refreshed after interval: %d → %d", first, apiCalls.Load())
	}
}
