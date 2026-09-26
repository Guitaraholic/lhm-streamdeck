package lhmstreamdeckplugin

import (
	"fmt"
	"testing"
)

func TestSourceKindHelpersCLIProxy(t *testing.T) {
	lhm := lhmSourceProfile{Kind: ""}
	if lhm.isCLIProxy() || lhm.sourceKind() != sourceKindLHM {
		t.Fatalf("empty kind should be LHM: %+v", lhm)
	}
	cp := lhmSourceProfile{Kind: "cliproxy", ManagementKey: "k"}
	if !cp.isCLIProxy() {
		t.Fatal("kind cliproxy should be detected")
	}
	if cp.isSparkDash() {
		t.Fatal("kind cliproxy should not be sparkdash")
	}
	if got := (lhmSourceProfile{Kind: "CLIPROXY"}).sourceKind(); got != sourceKindCLIProxy {
		t.Fatalf("kind should normalize case: %q", got)
	}
}

func TestSameSourceProfileEndpointIncludesManagementKey(t *testing.T) {
	a := lhmSourceProfile{ID: "p1", Host: "10.0.0.8", Port: 8317, Kind: "cliproxy", ManagementKey: "k1"}
	b := a
	if !sameSourceProfileEndpoint(a, b) {
		t.Fatal("identical profiles should match")
	}
	b.ManagementKey = "k2"
	if sameSourceProfileEndpoint(a, b) {
		t.Fatal("different management key should not match")
	}
	b = a
	b.Kind = "sparkdash"
	if sameSourceProfileEndpoint(a, b) {
		t.Fatal("cliproxy vs sparkdash should not match")
	}
}

func TestStartSourceClientLockedCLIProxy(t *testing.T) {
	p := &Plugin{sources: make(map[string]*sourceRuntime)}
	rt := &sourceRuntime{
		profile: lhmSourceProfile{
			ID:            "cp1",
			Kind:          sourceKindCLIProxy,
			Host:          "10.0.0.8",
			Port:          8317,
			ManagementKey: "k",
		},
	}
	if err := p.startSourceClientLocked(rt); err != nil {
		t.Fatal(err)
	}
	if rt.hw == nil {
		t.Fatal("expected cliProxy hardware service")
	}
}

func TestSourceUnavailableMessageCLIProxy(t *testing.T) {
	p := &Plugin{
		globalSettings: globalSettings{
			SourceProfiles: []lhmSourceProfile{
				{ID: "cp", Kind: sourceKindCLIProxy, Host: "10.0.0.8", Port: 8317, ManagementKey: "k"},
			},
		},
	}
	if got := p.sourceUnavailableMessage("cp"); got != "CLI Proxy Unavailable" {
		t.Fatalf("cliproxy message = %q", got)
	}
}

func TestSourceFetchErrorCLIProxy(t *testing.T) {
	p := &Plugin{
		globalSettings: globalSettings{
			SourceProfiles: []lhmSourceProfile{{
				ID: "cp", Kind: sourceKindCLIProxy, Host: "cliproxy.example.com", Port: 443, ManagementKey: "k",
			}},
		},
	}
	got := p.sourceFetchError("cp", fmt.Errorf("request cliProxy: management key rejected (401 Unauthorized)"))
	if got != "CLI Proxy unavailable: request cliProxy: management key rejected (401 Unauthorized)" {
		t.Fatalf("fetch error = %q", got)
	}
}

func TestStartCLIProxySourceWiresHardwareService(t *testing.T) {
	rt := &sourceRuntime{
		profile: lhmSourceProfile{
			ID:            "cp1",
			Kind:          sourceKindCLIProxy,
			Host:          "10.0.0.8",
			Port:          8317,
			ManagementKey: "k",
		},
	}
	if err := startCLIProxySource(rt); err != nil {
		t.Fatal(err)
	}
	if rt.hw == nil {
		t.Fatal("expected cliProxy hardware service")
	}
}
