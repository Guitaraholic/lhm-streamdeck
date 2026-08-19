package sparkdash

import (
	"fmt"
	"hash/fnv"
	"strings"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

// mappedSnapshot is the HardwareService view of one SparkDash unit.
type mappedSnapshot struct {
	sensors  []hwsensorsservice.Sensor
	readings map[string][]hwsensorsservice.Reading
}

type sensor struct {
	id   string
	name string
}

func (s sensor) ID() string   { return s.id }
func (s sensor) Name() string { return s.name }

type reading struct {
	id              int32
	label           string
	unit            string
	typ             string
	typeI           hwsensorsservice.ReadingType
	value           float64
	normalizedValue float64
	min             float64
	max             float64
	average         float64
}

func (r reading) ID() int32                { return r.id }
func (r reading) TypeI() int32             { return int32(r.typeI) }
func (r reading) Type() string             { return r.typ }
func (r reading) Label() string            { return r.label }
func (r reading) Unit() string             { return r.unit }
func (r reading) Value() float64           { return r.value }
func (r reading) ValueNormalized() float64 { return r.normalizedValue }
func (r reading) ValueMin() float64        { return r.min }
func (r reading) ValueMax() float64        { return r.max }
func (r reading) ValueAvg() float64        { return r.average }

type extrema struct {
	min float64
	max float64
}

func track(store map[string]*extrema, id string, val float64) (min, max float64) {
	t, ok := store[id]
	if !ok {
		t = &extrema{min: val, max: val}
		store[id] = t
	}
	if val < t.min {
		t.min = val
	}
	if val > t.max {
		t.max = val
	}
	return t.min, t.max
}

func mapSnapshot(snap *Snapshot, extremaStore map[string]*extrema) mappedSnapshot {
	out := mappedSnapshot{
		readings: make(map[string][]hwsensorsservice.Reading),
	}
	if snap == nil {
		return out
	}
	llms := snap.Metrics.LLM
	for i, llm := range llms {
		port := portForIndex(snap, i)
		sid := fmt.Sprintf("/llm/%d", port)
		s := sensor{id: sid, name: sensorName(llm, port)}
		out.sensors = append(out.sensors, s)
		out.readings[sid] = readingsForLLM(sid, llm, extremaStore)
	}
	return out
}

func portForIndex(snap *Snapshot, i int) int {
	if i >= 0 && i < len(snap.LLMPorts) && snap.LLMPorts[i] > 0 {
		return snap.LLMPorts[i]
	}
	if i == 0 && snap.LLMPort > 0 {
		return snap.LLMPort
	}
	return i
}

func sensorName(llm LlmMetrics, port int) string {
	name := strings.TrimSpace(derefString(llm.ModelID))
	if name == "" {
		name = strings.TrimSpace(llm.Backend)
	}
	if name == "" {
		name = "LLM"
	}
	return fmt.Sprintf("%s :%d", name, port)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func readingsForLLM(sensorID string, llm LlmMetrics, extremaStore map[string]*extrema) []hwsensorsservice.Reading {
	var out []hwsensorsservice.Reading
	add := func(path, label, typ, unit string, value float64) {
		id := sensorID + path
		min, max := track(extremaStore, id, value)
		out = append(out, reading{
			id:              makeReadingID(sensorID, id),
			label:           label,
			unit:            unit,
			typ:             typ,
			typeI:           mapType(typ),
			value:           value,
			normalizedValue: value,
			min:             min,
			max:             max,
			average:         value,
		})
	}

	add("/throughput/0", "Decode", "Throughput", "tok/s", llm.GenerationTps)
	add("/throughput/1", "Prefill", "Throughput", "tok/s", llm.PrefillTps)
	if llm.CachedPrefillTps != nil {
		add("/throughput/2", "Cached Prefill", "Throughput", "tok/s", *llm.CachedPrefillTps)
	}
	if llm.UncachedPrefillTps != nil {
		add("/throughput/3", "Uncached Prefill", "Throughput", "tok/s", *llm.UncachedPrefillTps)
	}
	if llm.KVCacheUsage != nil {
		add("/load/0", "KV Cache", "Load", "%", *llm.KVCacheUsage*100)
	}
	if llm.RequestsRunning != nil {
		add("/level/0", "Running", "Level", "req", float64(*llm.RequestsRunning))
	}
	if llm.RequestsWaiting != nil {
		add("/level/1", "Waiting", "Level", "req", float64(*llm.RequestsWaiting))
	}
	if llm.SlotsActive > 0 || llm.SlotsTotal > 0 {
		add("/level/2", "Slots", "Level", "", float64(llm.SlotsActive))
	}
	if llm.TTFTP95Seconds != nil {
		add("/span/0", "TTFT p95", "Time", "ms", *llm.TTFTP95Seconds*1000)
	}
	return out
}

func mapType(typ string) hwsensorsservice.ReadingType {
	switch strings.ToLower(typ) {
	case "load", "level", "control":
		return hwsensorsservice.ReadingTypeUsage
	default:
		return hwsensorsservice.ReadingTypeOther
	}
}

func makeReadingID(sensorID, readingID string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sensorID))
	_, _ = h.Write([]byte(readingID))
	return int32(h.Sum32() & 0x7fffffff)
}

// ReadingIDFor is exported for tests that need to assert picker IDs.
func ReadingIDFor(sensorID, path string) int32 {
	return makeReadingID(sensorID, sensorID+path)
}
