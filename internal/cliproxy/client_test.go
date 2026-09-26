package cliproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"127.0.0.1", 8317, "http://127.0.0.1:8317"},
		{"", 0, "http://127.0.0.1:8317"},
		{"cliproxy.example.com", 443, "https://cliproxy.example.com"},
		{"cliproxy.example.com", 80, "http://cliproxy.example.com"},
		{"192.168.1.133", 8317, "http://192.168.1.133:8317"},
		{"https://cliproxy.example.com", 8317, "https://cliproxy.example.com"},
		{"https://cliproxy.example.com/", 443, "https://cliproxy.example.com"},
		{"https://cliproxy.example.com:8443", 8317, "https://cliproxy.example.com:8443"},
		{"http://192.168.1.133:8317", 8080, "http://192.168.1.133:8317"},
		{"::1", 8317, "http://[::1]:8317"},
	}
	for _, tc := range tests {
		got := BaseURL(tc.host, tc.port)
		if got != tc.want {
			t.Errorf("BaseURL(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestClientSendsManagementKey(t *testing.T) {
	var gotAuth string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port, "secret-mgmt-key")
	if _, err := c.Accounts(); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret-mgmt-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotPath != "/v0/management/auth-files" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestClientAuthRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)
	c := NewClient(host, port, "bad-key")
	_, err := c.Accounts()
	if err == nil {
		t.Fatal("expected auth error")
	}
	if got := err.Error(); !strings.Contains(got, "management key rejected") {
		t.Fatalf("error = %q", got)
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
