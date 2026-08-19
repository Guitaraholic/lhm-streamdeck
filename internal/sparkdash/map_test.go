package sparkdash

import (
	"testing"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

func strPtr(s string) *string   { return &s }
func f64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int         { return &v }

func TestMapSnapshotOnePort(t *testing.T) {
	snap := &Snapshot{
		ID:       "unit-a",
		Name:     "unit-a",
		Online:   true,
		LLMPort:  8000,
		LLMPorts: []int{8000},
		Metrics: SnapMetrics{
			LLM: []LlmMetrics{{
				Available:          true,
				Backend:            "vllm",
				ModelID:            strPtr("Qwen3-32B"),
				SlotsActive:        2,
				SlotsTotal:         8,
				GenerationTps:      42.3,
				PrefillTps:         180.1,
				CachedPrefillTps:   f64Ptr(1200),
				UncachedPrefillTps: f64Ptr(95),
				KVCacheUsage:       f64Ptr(0.67),
				RequestsRunning:    intPtr(2),
				RequestsWaiting:    intPtr(1),
				TTFTP95Seconds:     f64Ptr(0.12),
			}},
		},
	}

	got := mapSnapshot(snap, map[string]*extrema{})
	if len(got.sensors) != 1 {
		t.Fatalf("sensors = %d, want 1", len(got.sensors))
	}
	if got.sensors[0].ID() != "/llm/8000" {
		t.Fatalf("sensor id = %q, want /llm/8000", got.sensors[0].ID())
	}
	if got.sensors[0].Name() != "Qwen3-32B :8000" {
		t.Fatalf("sensor name = %q", got.sensors[0].Name())
	}

	rs := got.readings["/llm/8000"]
	wantLabels := []string{
		"Decode", "Prefill", "Cached Prefill", "Uncached Prefill",
		"KV Cache", "Running", "Waiting", "Slots", "TTFT p95",
	}
	if len(rs) != len(wantLabels) {
		t.Fatalf("readings = %d, want %d: %v", len(rs), len(wantLabels), labels(rs))
	}
	for i, label := range wantLabels {
		if rs[i].Label() != label {
			t.Errorf("reading[%d] = %q, want %q", i, rs[i].Label(), label)
		}
	}

	decode := rs[0]
	if decode.Value() != 42.3 || decode.Unit() != "tok/s" || decode.Type() != "Throughput" {
		t.Errorf("decode = %+v", decode)
	}
	if decode.TypeI() != int32(hwsensorsservice.ReadingTypeOther) {
		t.Errorf("decode TypeI = %d, want Other", decode.TypeI())
	}
	if decode.ID() != ReadingIDFor("/llm/8000", "/throughput/0") {
		t.Errorf("decode id not stable")
	}

	kv := rs[4]
	if kv.Label() != "KV Cache" || kv.Value() != 67 || kv.Unit() != "%" {
		t.Errorf("kv cache = label %q value %v unit %q", kv.Label(), kv.Value(), kv.Unit())
	}

	ttft := rs[8]
	if ttft.Label() != "TTFT p95" || ttft.Value() != 120 || ttft.Unit() != "ms" {
		t.Errorf("ttft = label %q value %v unit %q", ttft.Label(), ttft.Value(), ttft.Unit())
	}
}

func TestMapSnapshotOmitsNullOptionalReadings(t *testing.T) {
	snap := &Snapshot{
		LLMPorts: []int{8000},
		Metrics: SnapMetrics{
			LLM: []LlmMetrics{{
				Available:     true,
				Backend:       "llama.cpp",
				GenerationTps: 10,
				PrefillTps:    20,
			}},
		},
	}
	got := mapSnapshot(snap, map[string]*extrema{})
	rs := got.readings["/llm/8000"]
	if len(rs) != 2 {
		t.Fatalf("readings = %v, want Decode+Prefill only", labels(rs))
	}
	if got.sensors[0].Name() != "llama.cpp :8000" {
		t.Errorf("name = %q, want backend fallback", got.sensors[0].Name())
	}
}

func TestMapSnapshotMultiplePorts(t *testing.T) {
	snap := &Snapshot{
		LLMPorts: []int{8000, 8888},
		Metrics: SnapMetrics{
			LLM: []LlmMetrics{
				{ModelID: strPtr("Qwen3-32B"), GenerationTps: 40, PrefillTps: 100},
				{ModelID: strPtr("Llama-70B"), GenerationTps: 12, PrefillTps: 50},
			},
		},
	}
	got := mapSnapshot(snap, map[string]*extrema{})
	if len(got.sensors) != 2 {
		t.Fatalf("sensors = %d, want 2", len(got.sensors))
	}
	if got.sensors[1].ID() != "/llm/8888" || got.sensors[1].Name() != "Llama-70B :8888" {
		t.Fatalf("second sensor = %s %s", got.sensors[1].ID(), got.sensors[1].Name())
	}
}

func TestMapSnapshotEmptyLLM(t *testing.T) {
	got := mapSnapshot(&Snapshot{ID: "gpu-host", Metrics: SnapMetrics{}}, map[string]*extrema{})
	if len(got.sensors) != 0 {
		t.Fatalf("sensors = %d, want 0", len(got.sensors))
	}
}

func TestReadingIDsAreStableAcrossPolls(t *testing.T) {
	snap := &Snapshot{
		LLMPorts: []int{8000},
		Metrics:  SnapMetrics{LLM: []LlmMetrics{{GenerationTps: 1, PrefillTps: 2}}},
	}
	a := mapSnapshot(snap, map[string]*extrema{})
	snap.Metrics.LLM[0].GenerationTps = 99
	b := mapSnapshot(snap, map[string]*extrema{})
	if a.readings["/llm/8000"][0].ID() != b.readings["/llm/8000"][0].ID() {
		t.Fatal("reading id changed when value changed")
	}
}

func TestExtremaTrackMinMax(t *testing.T) {
	store := map[string]*extrema{}
	snap := &Snapshot{
		LLMPorts: []int{8000},
		Metrics:  SnapMetrics{LLM: []LlmMetrics{{GenerationTps: 10, PrefillTps: 0}}},
	}
	mapSnapshot(snap, store)
	snap.Metrics.LLM[0].GenerationTps = 40
	got := mapSnapshot(snap, store)
	r := got.readings["/llm/8000"][0]
	if r.ValueMin() != 10 || r.ValueMax() != 40 {
		t.Fatalf("min/max = %v/%v, want 10/40", r.ValueMin(), r.ValueMax())
	}
}

func labels(rs []hwsensorsservice.Reading) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Label()
	}
	return out
}
