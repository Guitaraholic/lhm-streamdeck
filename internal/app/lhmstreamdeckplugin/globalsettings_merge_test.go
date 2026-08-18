package lhmstreamdeckplugin

import (
	"encoding/json"
	"testing"
)

func configured() globalSettings {
	return globalSettings{
		PollInterval: 1000,
		SourceProfiles: []lhmSourceProfile{
			{ID: "default", Name: "Mac Mini", Host: "127.0.0.1", Port: 8085, Icon: "apple"},
			{ID: "source_178", Name: "Spark B", Host: "192.168.1.40", Port: 8085, Icon: "nvidia"},
		},
		DefaultSourceProfileID: "default",
		FavoriteReadings:       []favoriteReading{{ID: "fav1", SensorUID: "/cpu"}},
		GlobalThresholds:       []Threshold{{ID: "t1"}},
		DerivedPresets:         json.RawMessage(`[{"name":"p1"}]`),
	}
}

// A property inspector writing one field produces a payload with everything
// else missing. That must not wipe the user's hosts.
func TestMergeGlobalSettingsKeepsProfilesOnPartialWrite(t *testing.T) {
	current := configured()
	partial := globalSettings{PollInterval: 2000} // e.g. the poll-interval control

	got, repersist := mergeGlobalSettings(partial, current)

	if !repersist {
		t.Error("expected repersist so the store is rewritten with full state")
	}
	if len(got.SourceProfiles) != 2 {
		t.Fatalf("source profiles lost: got %d, want 2", len(got.SourceProfiles))
	}
	if got.SourceProfiles[1].Host != "192.168.1.40" {
		t.Errorf("Spark profile mangled: %+v", got.SourceProfiles[1])
	}
	if got.DefaultSourceProfileID != "default" {
		t.Errorf("default profile id lost: %q", got.DefaultSourceProfileID)
	}
	if len(got.FavoriteReadings) != 1 || len(got.GlobalThresholds) != 1 {
		t.Error("favorites or thresholds lost")
	}
	if string(got.DerivedPresets) != `[{"name":"p1"}]` {
		t.Errorf("derived presets lost: %s", got.DerivedPresets)
	}
	if got.PollInterval != 2000 {
		t.Errorf("the field actually being written was not applied: %d", got.PollInterval)
	}
}

// A genuine full write must pass through untouched.
func TestMergeGlobalSettingsPassesThroughFullWrite(t *testing.T) {
	current := configured()
	incoming := configured()
	incoming.SourceProfiles = []lhmSourceProfile{{ID: "only", Name: "Only", Host: "10.0.0.1", Port: 8085}}
	incoming.DefaultSourceProfileID = "only"

	got, repersist := mergeGlobalSettings(incoming, current)

	if repersist {
		t.Error("a complete write needs no corrective repersist")
	}
	if len(got.SourceProfiles) != 1 || got.SourceProfiles[0].ID != "only" {
		t.Fatalf("full write was not honoured: %+v", got.SourceProfiles)
	}
	if got.DefaultSourceProfileID != "only" {
		t.Errorf("default id not honoured: %q", got.DefaultSourceProfileID)
	}
}

// First run has nothing stored; migration should still be free to synthesise.
func TestMergeGlobalSettingsFirstRunIsUntouched(t *testing.T) {
	got, repersist := mergeGlobalSettings(globalSettings{}, globalSettings{})
	if repersist {
		t.Error("nothing to preserve on first run")
	}
	if len(got.SourceProfiles) != 0 {
		t.Errorf("unexpected profiles synthesised here: %+v", got.SourceProfiles)
	}
}
