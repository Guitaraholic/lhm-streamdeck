package sparkdash

import (
	"os"
	"testing"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"127.0.0.1", 5555, "http://127.0.0.1:5555"},
		{"", 0, "http://127.0.0.1:5555"},
		{"sparkdash.example.com", 443, "https://sparkdash.example.com"},
		{"sparkdash.example.com", 80, "http://sparkdash.example.com"},
		{"10.0.0.8", 5555, "http://10.0.0.8:5555"},
		{"https://sparkdash.example.com", 5555, "https://sparkdash.example.com"},
		{"https://sparkdash.example.com/", 443, "https://sparkdash.example.com"},
		{"https://sparkdash.example.com:8443", 5555, "https://sparkdash.example.com:8443"},
		{"http://10.0.0.8:5555", 8080, "http://10.0.0.8:5555"},
		{"::1", 5555, "http://[::1]:5555"},
	}
	for _, tc := range tests {
		got := BaseURL(tc.host, tc.port)
		if got != tc.want {
			t.Errorf("BaseURL(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestLiveSparkDashHTTPS(t *testing.T) {
	raw := os.Getenv("SPARKDASH_LIVE_URL")
	if raw == "" {
		t.Skip("set SPARKDASH_LIVE_URL to a SparkDash origin to run this")
	}
	c := NewClient(raw, 0)
	units, err := c.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected SparkDash units")
	}
	snap, err := c.Snapshot(units[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID == "" {
		t.Fatal("empty snapshot id")
	}
}
