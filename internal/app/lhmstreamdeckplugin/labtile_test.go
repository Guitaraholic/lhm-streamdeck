package lhmstreamdeckplugin

import "testing"

func TestLabMetricAndUnitTokenRates(t *testing.T) {
	s := &actionSettings{ReadingLabel: "Decode"}
	metric, unit := labMetricAndUnit(s, "tok/s")
	if metric != "tok/s" || unit != "" {
		t.Fatalf("decode: metric=%q unit=%q, want tok/s and empty unit", metric, unit)
	}

	s.ReadingLabel = "Prefill"
	metric, unit = labMetricAndUnit(s, "tok/s")
	if metric != "prefill/s" || unit != "" {
		t.Fatalf("prefill: metric=%q unit=%q", metric, unit)
	}
}

func TestLabMetricAndUnitKeepsPercentBesideValue(t *testing.T) {
	s := &actionSettings{ReadingLabel: "KV Cache"}
	metric, unit := labMetricAndUnit(s, "%")
	if metric != "KV Cache" || unit != "%" {
		t.Fatalf("percent: metric=%q unit=%q", metric, unit)
	}
	if labMetricDisplay(metric) != "KV CACHE" {
		t.Fatalf("percent display = %q", labMetricDisplay(metric))
	}
}

func TestLabMetricDisplayLeavesRateLabelsAlone(t *testing.T) {
	if got := labMetricDisplay("tok/s"); got != "tok/s" {
		t.Fatalf("tok/s was uppercased: %q", got)
	}
	if got := labMetricDisplay("prefill/s"); got != "prefill/s" {
		t.Fatalf("prefill/s was uppercased: %q", got)
	}
}

func TestLabMetricOverrideWins(t *testing.T) {
	s := &actionSettings{ReadingLabel: "Decode", MetricLabel: "gen"}
	metric, unit := labMetricAndUnit(s, "tok/s")
	if metric != "gen" || unit != "" {
		t.Fatalf("override: metric=%q unit=%q", metric, unit)
	}
}
