package sparkdash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moeilijk/lhm-streamdeck/internal/httptarget"
)

const defaultPort = 5555

// Client talks to a SparkDash dashboard over HTTP or HTTPS.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client for a SparkDash host:port.
// Port 443 uses HTTPS (Caddy); 80 uses HTTP (redirects are followed);
// 5555 and every other port stay HTTP for a direct SparkDash listener.
func NewClient(host string, port int) *Client {
	return NewClientWithHTTP(host, port, &http.Client{Timeout: 5 * time.Second})
}

// NewClientWithHTTP is used by tests that inject a transport.
func NewClientWithHTTP(host string, port int, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: BaseURL(host, port), http: hc}
}

// BaseURL builds the SparkDash origin from a host/port pair.
//
//	5555 → http://host:5555   (plugin default, direct SparkDash listener)
//	443  → https://host       (HTTPS reverse proxy)
//	80   → http://host        (HTTP, typically redirected to HTTPS)
//
// A pasted https:// URL in host is honoured; if the port is still the 5555
// default, it becomes 443.
func BaseURL(host string, port int) string {
	return httptarget.Base(host, port, defaultPort)
}

func (c *Client) url(parts ...string) string {
	b := strings.TrimRight(c.baseURL, "/")
	for _, p := range parts {
		b += "/" + url.PathEscape(p)
	}
	return b
}

// ListUnits returns SparkDash units (id + name) from GET /api/sparks.
func (c *Client) ListUnits() ([]Unit, error) {
	var resp sparksListResponse
	if err := c.get(c.url("api", "sparks"), &resp); err != nil {
		return nil, err
	}
	out := make([]Unit, 0, len(resp.Sparks))
	for _, u := range resp.Sparks {
		if strings.TrimSpace(u.ID) == "" {
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

// Snapshot returns one unit's metrics snapshot.
func (c *Client) Snapshot(sparkID string) (*Snapshot, error) {
	id := strings.TrimSpace(sparkID)
	if id == "" {
		return nil, fmt.Errorf("sparkDash unit not selected")
	}
	var snap Snapshot
	if err := c.get(c.url("api", "sparks", id, "metrics"), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (c *Client) get(url string, dest interface{}) error {
	resp, err := c.http.Get(url)
	if err != nil {
		return fmt.Errorf("request sparkDash: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request sparkDash: status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode sparkDash response: %w", err)
	}
	return nil
}
