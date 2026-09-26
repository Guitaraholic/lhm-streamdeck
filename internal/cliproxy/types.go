package cliproxy

import "encoding/json"

// Account is one authenticated credential from GET /v0/management/auth-files.
// In multi-account setups each auth file is a separate upstream session the
// proxy can route requests to.
type Account struct {
	ID             string          `json:"id"`
	AuthIndex      flexString      `json:"auth_index"`
	Name           string          `json:"name"`
	Provider       string          `json:"provider"`
	Label          string          `json:"label"`
	Email          string          `json:"email"`
	Status         string          `json:"status"`
	StatusMessage  string          `json:"status_message"`
	Disabled       bool            `json:"disabled"`
	Unavailable    bool            `json:"unavailable"`
	RuntimeOnly    bool            `json:"runtime_only"`
	Success        float64         `json:"success"`
	Failed         float64         `json:"failed"`
	LastRefresh    flexString      `json:"last_refresh"`
	RecentRequests []RequestBucket `json:"recent_requests"`
	// Some builds emit camelCase; merged in UnmarshalJSON and not re-emitted.
	RecentRequestsCamel []RequestBucket `json:"-"`
}

// RequestBucket is one recent_requests bucket (10 minutes of traffic).
type RequestBucket struct {
	Time    string  `json:"time"`
	Success float64 `json:"success"`
	Failed  float64 `json:"failed"`
}

func (a *Account) UnmarshalJSON(b []byte) error {
	type alias Account
	var v struct {
		alias
		RecentRequestsCamel []RequestBucket `json:"recentRequests"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*a = Account(v.alias)
	if len(a.RecentRequests) == 0 {
		a.RecentRequests = v.RecentRequestsCamel
	}
	return nil
}

// flexString decodes JSON strings or numbers into a string — auth_index and
// last_refresh arrive as either depending on the credential backend.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexString(n.String())
		return nil
	}
	return nil
}

func (f flexString) String() string { return string(f) }

type authFilesResponse struct {
	Files []Account `json:"files"`
}
