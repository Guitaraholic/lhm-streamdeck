package cliproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func testAccountsPayload() map[string]any {
	return map[string]any{
		"files": []map[string]any{
			{
				"id":          "claude-user@example.com",
				"auth_index":  "a1b2c3d4e5f67890",
				"name":        "claude-user@example.com.json",
				"provider":    "claude",
				"label":       "Claude Prod",
				"email":       "user@example.com",
				"status":      "ready",
				"disabled":    false,
				"unavailable": false,
				"success":     12,
				"failed":      1,
				"recent_requests": []map[string]any{
					{"time": "12:00-12:10", "success": 3, "failed": 0},
					{"time": "12:10-12:20", "success": 1, "failed": 1},
				},
				"last_refresh": "2025-08-31T01:23:45Z",
			},
			{
				"id":         "claude-second@example.com",
				"auth_index": "b2c3d4e5f67890a1",
				"name":       "claude-second@example.com.json",
				"provider":   "claude",
				"email":      "second@example.com",
				"status":     "ready",
				"success":    7,
				"failed":     0,
				"recent_requests": []map[string]any{
					{"time": "12:10-12:20", "success": 2, "failed": 0},
				},
			},
			{
				"id":          "codex-acct",
				"auth_index":  "c3d4e5f67890a1b2",
				"name":        "codex-acct.json",
				"provider":    "codex",
				"status":      "cooldown",
				"unavailable": true,
				"success":     40,
				"failed":      9,
			},
		},
	}
}

func TestClientAccountsMultiAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testAccountsPayload())
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port, "k")

	accounts, err := c.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %+v", accounts)
	}
	if accounts[0].AuthIndex.String() != "a1b2c3d4e5f67890" {
		t.Fatalf("auth_index = %q", accounts[0].AuthIndex)
	}
	if accounts[1].Email != "second@example.com" {
		t.Fatalf("email = %q", accounts[1].Email)
	}
	if len(accounts[0].RecentRequests) != 2 {
		t.Fatalf("recent_requests = %+v", accounts[0].RecentRequests)
	}
}

func TestServiceSensorsMultiAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testAccountsPayload())
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")

	if _, err := svc.PollTime(); err != nil {
		t.Fatal(err)
	}
	sensors, err := svc.Sensors()
	if err != nil {
		t.Fatal(err)
	}
	// Proxy summary + one sensor per account.
	if len(sensors) != 4 {
		t.Fatalf("sensors = %+v", sensors)
	}
	if sensors[0].ID() != "/cliproxy/proxy" {
		t.Fatalf("first sensor = %q", sensors[0].ID())
	}
	if sensors[1].Name() != "Claude Prod (claude)" {
		t.Fatalf("account sensor name = %q", sensors[1].Name())
	}
	if sensors[2].Name() != "second@example.com (claude)" {
		t.Fatalf("second account name = %q", sensors[2].Name())
	}
	if sensors[3].ID() != "/cliproxy/account/c3d4e5f67890a1b2" {
		t.Fatalf("third sensor id = %q", sensors[3].ID())
	}

	readings, err := svc.ReadingsForSensorID("/cliproxy/account/a1b2c3d4e5f67890")
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]float64{}
	for _, r := range readings {
		byLabel[r.Label()] = r.Value()
	}
	if byLabel["Ready"] != 1 {
		t.Fatalf("ready = %v", byLabel["Ready"])
	}
	if byLabel["Requests (10m)"] != 2 {
		t.Fatalf("last bucket = %v", byLabel["Requests (10m)"])
	}
	if byLabel["Success"] != 12 || byLabel["Failed"] != 1 {
		t.Fatalf("totals = %v/%v", byLabel["Success"], byLabel["Failed"])
	}
	// Window rate: 4 success / 5 total = 80%.
	if byLabel["Success rate"] != 80 {
		t.Fatalf("rate = %v", byLabel["Success rate"])
	}

	// Down account reports Ready = 0.
	down, err := svc.ReadingsForSensorID("/cliproxy/account/c3d4e5f67890a1b2")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range down {
		if r.Label() == "Ready" && r.Value() != 0 {
			t.Fatalf("down account ready = %v", r.Value())
		}
	}

	// Proxy summary: 3 accounts, 2 ready, 1 down, tail buckets 2+2+0 = 4.
	proxy, err := svc.ReadingsForSensorID("/cliproxy/proxy")
	if err != nil {
		t.Fatal(err)
	}
	pb := map[string]float64{}
	for _, r := range proxy {
		pb[r.Label()] = r.Value()
	}
	if pb["Accounts"] != 3 || pb["Ready"] != 2 || pb["Down"] != 1 {
		t.Fatalf("proxy counts = %+v", pb)
	}
	if pb["Requests (10m)"] != 4 {
		t.Fatalf("proxy tail = %v", pb["Requests (10m)"])
	}
}

func TestServiceStableIDsAcrossReorderAndDupes(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/management/auth-files" {
			// api-call / quota probes get an empty upstream reply.
			_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 404})
			return
		}
		payload := testAccountsPayload()
		files := payload["files"].([]map[string]any)
		if polls.Add(1) == 1 {
			// First poll: reverse order and repeat one auth_index.
			payload["files"] = []map[string]any{files[2], files[1], files[0], files[0]}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")

	if _, err := svc.PollTime(); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Sensors()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 4 {
		t.Fatalf("duplicate auth_index should dedupe to 3 accounts, got %+v", first)
	}
	ids := map[string]bool{}
	for _, s := range first {
		if ids[s.ID()] {
			t.Fatalf("duplicate sensor id %q", s.ID())
		}
		ids[s.ID()] = true
	}

	svc2 := NewService(host, port, "k")
	if _, err := svc2.PollTime(); err != nil {
		t.Fatal(err)
	}
	second, err := svc2.Sensors()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range second {
		if !ids[s.ID()] {
			t.Fatalf("sensor id %q changed across reorder", s.ID())
		}
	}
}

func TestServiceHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "k")
	if _, err := svc.PollTime(); err == nil {
		t.Fatal("expected 404 error")
	}
}
