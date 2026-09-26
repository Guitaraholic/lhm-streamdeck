package cliproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moeilijk/lhm-streamdeck/internal/httptarget"
)

// defaultPort is CLIProxyAPI's stock management/API listener.
const defaultPort = 8317

// Client talks to a CLIProxyAPI management API over HTTP or HTTPS.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// NewClient builds a client for a CLI Proxy host:port with a management key.
// Port 443 uses HTTPS; 80 uses HTTP; 8317 and every other port stay HTTP for
// a direct listener.
func NewClient(host string, port int, managementKey string) *Client {
	return NewClientWithHTTP(host, port, managementKey, &http.Client{Timeout: 5 * time.Second})
}

// NewClientWithHTTP is used by tests that inject a transport.
func NewClientWithHTTP(host string, port int, managementKey string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{
		baseURL: BaseURL(host, port),
		key:     strings.TrimSpace(managementKey),
		http:    hc,
	}
}

// BaseURL builds the CLI Proxy origin from a host/port pair; see
// httptarget.Base for the scheme/port rules. The direct-listener default is
// 8317.
func BaseURL(host string, port int) string {
	return httptarget.Base(host, port, defaultPort)
}

func (c *Client) url(parts ...string) string {
	b := strings.TrimRight(c.baseURL, "/") + "/v0/management"
	for _, p := range parts {
		b += "/" + url.PathEscape(p)
	}
	return b
}

// Accounts returns every credential the proxy has authenticated — the session
// inventory. Multi-account setups come back as one entry per auth file.
func (c *Client) Accounts() ([]Account, error) {
	var resp authFilesResponse
	if err := c.get(c.url("auth-files"), &resp); err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(resp.Files))
	for _, a := range resp.Files {
		if accountKey(a) == "" {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// accountKey is the stable sensor-id component for an account: auth_index is
// the proxy's stable runtime identifier; id/name are fallbacks for the
// auth-dir scan shape that lacks it.
func accountKey(a Account) string {
	if s := strings.TrimSpace(a.AuthIndex.String()); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.ID); s != "" {
		return s
	}
	return strings.TrimSpace(a.Name)
}

// apiCallRequest is the POST body for /v0/management/api-call. The proxy
// resolves auth_index to a credential and substitutes $TOKEN$ in headers.
type apiCallRequest struct {
	AuthIndex string            `json:"auth_index"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Header    map[string]string `json:"header,omitempty"`
	Data      string            `json:"data,omitempty"`
}

type apiCallResponse struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
}

// APICall executes an upstream request through the proxy's api-call
// passthrough using the credential identified by authIndex. The response body
// may arrive as a JSON value or a JSON-encoded string; both are returned as
// raw JSON.
func (c *Client) APICall(authIndex, method, url string, header map[string]string) (json.RawMessage, int, error) {
	payload, err := json.Marshal(apiCallRequest{
		AuthIndex: authIndex,
		Method:    method,
		URL:       url,
		Header:    header,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("request cliProxy api-call: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.url("api-call"), strings.NewReader(string(payload)))
	if err != nil {
		return nil, 0, fmt.Errorf("request cliProxy api-call: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request cliProxy api-call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, 0, fmt.Errorf("request cliProxy api-call: management key rejected (%s)", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("request cliProxy api-call: status %s", resp.Status)
	}
	var out apiCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("decode cliProxy api-call response: %w", err)
	}
	// Some upstreams return the body as a JSON-encoded string.
	if len(out.Body) > 0 && out.Body[0] == '"' {
		var s string
		if err := json.Unmarshal(out.Body, &s); err == nil {
			out.Body = json.RawMessage(s)
		}
	}
	return out.Body, out.StatusCode, nil
}

// pluginQuota POSTs to a management plugin's quota endpoint
// (/v0/management/plugins/{plugin}/quota) with a key_id body — the OpenCode
// Go plugin serves per-credential usage this way.
func (c *Client) pluginQuota(plugin, keyID string, dest *json.RawMessage) error {
	payload, err := json.Marshal(map[string]string{"key_id": keyID})
	if err != nil {
		return fmt.Errorf("request cliProxy plugin quota: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.url("plugins", plugin, "quota"), strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("request cliProxy plugin quota: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request cliProxy plugin quota: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("request cliProxy plugin quota: management key rejected (%s)", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request cliProxy plugin quota: status %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// getOrigin GETs a path on the proxy origin outside /v0/management — used by
// lab-exposed shims like /devin/quota that sit beside the API without auth.
func (c *Client) getOrigin(path string, dest *json.RawMessage) error {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.baseURL, "/")+path, nil)
	if err != nil {
		return fmt.Errorf("request cliProxy: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request cliProxy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request cliProxy: status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode cliProxy response: %w", err)
	}
	return nil
}

func (c *Client) get(url string, dest interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("request cliProxy: %w", err)
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request cliProxy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("request cliProxy: management key rejected (%s)", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request cliProxy: status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode cliProxy response: %w", err)
	}
	return nil
}
