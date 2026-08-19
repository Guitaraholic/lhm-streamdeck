package lhmstreamdeckplugin

import (
	"fmt"
	"testing"
)

func TestNormalizeTileStyle(t *testing.T) {
	if got := normalizeTileStyle("lab"); got != "lab" {
		t.Fatalf("lab: %q", got)
	}
	if got := normalizeTileStyle("Lab (host badge)"); got != "lab" {
		t.Fatalf("label text: %q", got)
	}
	if got := normalizeTileStyle("classic"); got != "classic" {
		t.Fatalf("classic: %q", got)
	}
	if got := normalizeTileStyle(""); got != "classic" {
		t.Fatalf("empty: %q", got)
	}
}

func TestSourceKindHelpers(t *testing.T) {
	lhm := lhmSourceProfile{Kind: ""}
	if lhm.isSparkDash() || lhm.sourceKind() != sourceKindLHM {
		t.Fatalf("empty kind should be LHM: %+v", lhm)
	}
	explicit := lhmSourceProfile{Kind: "lhm"}
	if explicit.isSparkDash() {
		t.Fatal("kind lhm should not be sparkdash")
	}
	sd := lhmSourceProfile{Kind: "sparkdash", SparkID: "unit-a"}
	if !sd.isSparkDash() {
		t.Fatal("kind sparkdash should be detected")
	}
}

func TestSameSourceProfileEndpointIncludesKindAndUnit(t *testing.T) {
	a := lhmSourceProfile{ID: "p1", Host: "10.0.0.8", Port: 5555, Kind: "sparkdash", SparkID: "unit-a"}
	b := a
	if !sameSourceProfileEndpoint(a, b) {
		t.Fatal("identical profiles should match")
	}
	b.SparkID = "unit-b"
	if sameSourceProfileEndpoint(a, b) {
		t.Fatal("different spark id should not match")
	}
	b = a
	b.Kind = ""
	if sameSourceProfileEndpoint(a, b) {
		t.Fatal("sparkdash vs lhm should not match")
	}
}

func TestStartSourceClientLockedSparkDash(t *testing.T) {
	p := &Plugin{sources: make(map[string]*sourceRuntime)}
	rt := &sourceRuntime{
		profile: lhmSourceProfile{
			ID:      "sd1",
			Kind:    sourceKindSparkDash,
			Host:    "10.0.0.8",
			Port:    5555,
			SparkID: "unit-a",
		},
	}
	if err := p.startSourceClientLocked(rt); err != nil {
		t.Fatal(err)
	}
	if rt.hw == nil {
		t.Fatal("expected sparkDash hardware service")
	}
}

func TestSourceUnavailableMessage(t *testing.T) {
	p := &Plugin{
		globalSettings: globalSettings{
			SourceProfiles: []lhmSourceProfile{
				{ID: "lhm", Host: "127.0.0.1", Port: 8085},
				{ID: "sd", Kind: sourceKindSparkDash, Host: "10.0.0.8", Port: 5555, SparkID: "unit-a"},
			},
		},
	}
	if got := p.sourceUnavailableMessage("lhm"); got != "Libre Hardware Monitor Unavailable" {
		t.Fatalf("lhm message = %q", got)
	}
	if got := p.sourceUnavailableMessage("sd"); got != "SparkDash Unavailable" {
		t.Fatalf("sparkdash message = %q", got)
	}
}

func TestEnsureSourceStartedWiresSparkDash(t *testing.T) {
	const profileID = "sd-new"
	p := &Plugin{
		sources: make(map[string]*sourceRuntime),
		globalSettings: globalSettings{
			SourceProfiles: []lhmSourceProfile{{
				ID:      profileID,
				Kind:    sourceKindSparkDash,
				Host:    "sparkdash.example.com",
				Port:    443,
				SparkID: "unit-a",
			}},
		},
	}
	rt := p.runtimeForSource(profileID)
	rt.mu.RLock()
	hw := rt.hw
	rt.mu.RUnlock()
	if hw != nil {
		t.Fatal("runtime should start with no hardware service")
	}
	if err := p.ensureSourceStarted(rt); err != nil {
		t.Fatalf("starting SparkDash source should not fail: %v", err)
	}
	rt.mu.RLock()
	hw = rt.hw
	rt.mu.RUnlock()
	if hw == nil {
		t.Fatal("ensureSourceStarted should wire hardware service")
	}
}

func TestSourceFetchErrorSparkDashUnit(t *testing.T) {
	p := &Plugin{
		globalSettings: globalSettings{
			SourceProfiles: []lhmSourceProfile{{
				ID: "sd", Kind: sourceKindSparkDash, Host: "sparkdash.example.com", Port: 443,
			}},
		},
	}
	got := p.sourceFetchError("sd", fmt.Errorf("sparkDash unit not selected"))
	if got == "Libre Hardware Monitor Unavailable" {
		t.Fatal("SparkDash unit error was mapped to LHM copy")
	}
}

func TestStartSparkDashSourceWiresHardwareService(t *testing.T) {
	rt := &sourceRuntime{
		profile: lhmSourceProfile{
			ID:      "sd1",
			Kind:    sourceKindSparkDash,
			Host:    "10.0.0.8",
			Port:    5555,
			SparkID: "unit-a",
		},
	}
	if err := startSparkDashSource(rt); err != nil {
		t.Fatal(err)
	}
	if rt.hw == nil {
		t.Fatal("expected sparkDash hardware service")
	}
}
