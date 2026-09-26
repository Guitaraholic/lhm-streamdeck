//go:build linux || darwin

package lhmstreamdeckplugin

import "testing"

func TestIsLocalHost(t *testing.T) {
	local := []string{"", "127.0.0.1", "localhost", "::1"}
	for _, h := range local {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = false, want true", h)
		}
	}
	remote := []string{"192.168.1.10", "172.18.175.238", "lhm.example.org", "127.0.0.2"}
	for _, h := range remote {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = true, want false", h)
		}
	}
}

func TestNormalizePort(t *testing.T) {
	cases := map[int]int{0: 8085, -1: 8085, 70000: 8085, 8085: 8085, 9999: 9999}
	for in, want := range cases {
		if got := normalizePort(in); got != want {
			t.Errorf("normalizePort(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestEffectivePortLocalMigration(t *testing.T) {
	cases := []struct {
		name string
		prof lhmSourceProfile
		want int
	}{
		// Local profiles on the legacy port must move — cliproxy's container
		// publishes 127.0.0.1:8085 and shadows the companion there.
		{"local legacy", lhmSourceProfile{Host: "127.0.0.1", Port: 8085}, defaultCompanionPort},
		{"localhost legacy", lhmSourceProfile{Host: "localhost", Port: 8085}, defaultCompanionPort},
		{"local unset", lhmSourceProfile{Host: "127.0.0.1", Port: 0}, defaultCompanionPort},
		{"empty host", lhmSourceProfile{Host: "", Port: 8085}, defaultCompanionPort},
		{"local explicit other", lhmSourceProfile{Host: "127.0.0.1", Port: 9999}, 9999},
		// Remote endpoints keep 8085 — that's LHM's own web server default.
		{"remote legacy", lhmSourceProfile{Host: "192.168.1.10", Port: 8085}, 8085},
		{"remote unset", lhmSourceProfile{Host: "192.168.1.10", Port: 0}, 8085},
		{"remote explicit", lhmSourceProfile{Host: "192.168.1.10", Port: 9999}, 9999},
	}
	for _, c := range cases {
		if got := effectivePort(c.prof); got != c.want {
			t.Errorf("%s: effectivePort = %d, want %d", c.name, got, c.want)
		}
	}
}

// supervisor harness with injectable probe/spawn.
type supHarness struct {
	sup        *companionSupervisor
	reachable  bool
	childAlive bool
	spawns     int
}

func newSupHarness() *supHarness {
	h := &supHarness{}
	h.sup = &companionSupervisor{
		port:  8085,
		probe: func() bool { return h.reachable },
		spawn: func() (func() bool, error) {
			h.spawns++
			h.childAlive = true
			return func() bool { return !h.childAlive }, nil
		},
	}
	return h
}

func TestSupervisorReusesRunningCompanion(t *testing.T) {
	h := newSupHarness()
	h.reachable = true // external companion (e.g. systemd) already serves the endpoint
	h.sup.ensureOnce()
	h.sup.ensureOnce()
	if h.spawns != 0 {
		t.Fatalf("spawned %d times while endpoint was reachable, want 0", h.spawns)
	}
}

func TestSupervisorSpawnsWhenUnreachable(t *testing.T) {
	h := newSupHarness()
	h.sup.ensureOnce()
	if h.spawns != 1 {
		t.Fatalf("spawns = %d, want 1", h.spawns)
	}
}

func TestSupervisorDoesNotDoubleSpawnLiveChild(t *testing.T) {
	h := newSupHarness()
	h.sup.ensureOnce() // spawns; child alive but endpoint still unreachable (e.g. still binding)
	h.sup.ensureOnce()
	h.sup.ensureOnce()
	if h.spawns != 1 {
		t.Fatalf("spawns = %d, want 1: live child must not be duplicated", h.spawns)
	}
}

func TestSupervisorRespawnsAfterChildExit(t *testing.T) {
	h := newSupHarness()
	h.sup.ensureOnce()
	h.childAlive = false // child crashed, endpoint unreachable
	h.sup.ensureOnce()
	if h.spawns != 2 {
		t.Fatalf("spawns = %d, want 2: crashed child must be respawned", h.spawns)
	}
}

func TestSupervisorGivesUpAfterRapidBindFailures(t *testing.T) {
	// A foreign process holds the port: every child exits immediately and the
	// endpoint never becomes reachable. The supervisor must not spawn forever.
	h := newSupHarness()
	for i := 0; i < 10; i++ {
		h.sup.ensureOnce()
		h.childAlive = false // child dies right away (bind failure)
	}
	if h.spawns > rapidFailLimit+1 {
		t.Fatalf("spawns = %d, want <= %d: supervisor must stop after rapid failures", h.spawns, rapidFailLimit+1)
	}
}

func TestSupervisorRecoversAfterEndpointReturns(t *testing.T) {
	h := newSupHarness()
	for i := 0; i < 6; i++ { // trip the breaker
		h.sup.ensureOnce()
		h.childAlive = false
	}
	h.reachable = true // endpoint healthy again (external companion started)
	h.sup.ensureOnce() // resets the failure counter
	if got := h.sup.rapidFails; got != 0 {
		t.Fatalf("rapidFails = %d after reachable ensure, want 0", got)
	}
}
