package sparkdash

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestClientListUnitsAndSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sparks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sparks": []map[string]any{
				{"id": "gpu-host", "name": "gpu-host"},
				{"id": "unit-a", "name": "unit-a"},
			},
		})
	})
	mux.HandleFunc("/api/sparks/unit-a/metrics", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Snapshot{
			ID:       "unit-a",
			Name:     "unit-a",
			Online:   true,
			LLMPort:  8000,
			LLMPorts: []int{8000},
			Metrics: SnapMetrics{LLM: []LlmMetrics{{
				Available:     true,
				Backend:       "vllm",
				ModelID:       strPtr("Qwen3-32B"),
				GenerationTps: 42.3,
				PrefillTps:    180,
			}}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port)

	units, err := c.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 || units[0].ID != "gpu-host" || units[1].ID != "unit-a" {
		t.Fatalf("units = %+v", units)
	}

	svc := NewServiceWithClient(c, "unit-a")
	pt, err := svc.PollTime()
	if err != nil {
		t.Fatal(err)
	}
	if pt == 0 {
		t.Fatal("poll time was 0")
	}
	sensors, err := svc.Sensors()
	if err != nil {
		t.Fatal(err)
	}
	if len(sensors) != 1 || sensors[0].Name() != "Qwen3-32B :8000" {
		t.Fatalf("sensors = %+v", sensors)
	}
	readings, err := svc.ReadingsForSensorID("/llm/8000")
	if err != nil {
		t.Fatal(err)
	}
	if readings[0].Label() != "Decode" || readings[0].Value() != 42.3 {
		t.Fatalf("decode = %s %v", readings[0].Label(), readings[0].Value())
	}
}

func TestServiceRequiresSparkID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "")
	if _, err := svc.PollTime(); err == nil {
		t.Fatal("expected error for empty spark id")
	}
}

func TestServiceHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	svc := NewService(host, port, "missing")
	if _, err := svc.PollTime(); err == nil {
		t.Fatal("expected 404 error")
	}
}

func hostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), p
}
