package cliproxy

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

// mappedAccounts is the HardwareService view of one auth-files listing.
type mappedAccounts struct {
	sensors  []hwsensorsservice.Sensor
	readings map[string][]hwsensorsservice.Reading
}

type sensor struct {
	id   string
	name string
}

func (s sensor) ID() string   { return s.id }
func (s sensor) Name() string { return s.name }

type reading struct {
	id              int32
	label           string
	unit            string
	typ             string
	typeI           hwsensorsservice.ReadingType
	value           float64
	normalizedValue float64
	min             float64
	max             float64
	average         float64
}

func (r reading) ID() int32                { return r.id }
func (r reading) TypeI() int32             { return int32(r.typeI) }
func (r reading) Type() string             { return r.typ }
func (r reading) Label() string            { return r.label }
func (r reading) Unit() string             { return r.unit }
func (r reading) Value() float64           { return r.value }
func (r reading) ValueNormalized() float64 { return r.normalizedValue }
func (r reading) ValueMin() float64        { return r.min }
func (r reading) ValueMax() float64        { return r.max }
func (r reading) ValueAvg() float64        { return r.average }

type extrema struct {
	min float64
	max float64
}

func track(store map[string]*extrema, id string, val float64) (min, max float64) {
	t, ok := store[id]
	if !ok {
		t = &extrema{min: val, max: val}
		store[id] = t
	}
	if val < t.min {
		t.min = val
	}
	if val > t.max {
		t.max = val
	}
	return t.min, t.max
}

func mapAccounts(accounts []Account, quotas map[string]*AccountQuota, extremaStore map[string]*extrema) mappedAccounts {
	out := mappedAccounts{
		readings: make(map[string][]hwsensorsservice.Reading),
	}
	proxyID := "/cliproxy/proxy"
	out.sensors = append(out.sensors, sensor{id: proxyID, name: "CLI Proxy"})
	var proxyReadings []hwsensorsservice.Reading
	addProxy := func(path, label, typ, unit string, value float64) {
		proxyReadings = append(proxyReadings, addReading(proxyID, path, label, typ, unit, value, extremaStore))
	}

	var ready, down, tailReqs, sumSuccess, sumFailed float64
	seen := map[string]bool{}
	for _, a := range accounts {
		key := accountKey(a)
		if seen[key] {
			continue
		}
		seen[key] = true
		sid := "/cliproxy/account/" + key
		out.sensors = append(out.sensors, sensor{id: sid, name: accountName(a, quotas[key])})
		out.readings[sid] = readingsForAccount(sid, a, quotas[key], extremaStore)
		if accountReady(a) {
			ready++
		} else {
			down++
		}
		tailReqs += lastBucketTotal(a.RecentRequests)
		sumSuccess += a.Success
		sumFailed += a.Failed
	}

	addProxy("/accounts/0", "Accounts", "Level", "", float64(len(accounts)))
	addProxy("/accounts/1", "Ready", "Level", "", ready)
	addProxy("/accounts/2", "Down", "Level", "", down)
	addProxy("/requests/0", "Requests (10m)", "Level", "req", tailReqs)
	addProxy("/success/0", "Success", "Level", "req", sumSuccess)
	addProxy("/failed/0", "Failed", "Level", "req", sumFailed)
	addProxy("/rate/0", "Success rate", "Load", "%", successRate(sumSuccess, sumFailed))
	out.readings[proxyID] = proxyReadings
	return out
}

// accountReady mirrors the management API's routing view: a credential only
// serves traffic when it is ready, not disabled, and not marked unavailable.
func accountReady(a Account) bool {
	return !a.Disabled && !a.Unavailable &&
		strings.EqualFold(strings.TrimSpace(a.Status), "ready")
}

// accountName prefers the operator-assigned label, then the identity fields;
// provider stays attached so several accounts on one provider stay distinct
// in the sensor picker. A known plan (Max, Pro, Team…) is appended so the
// picker shows which subscription each account is on.
func accountName(a Account, quota *AccountQuota) string {
	name := strings.TrimSpace(a.Label)
	if name == "" {
		name = strings.TrimSpace(a.Email)
	}
	if name == "" {
		name = strings.TrimSpace(a.Name)
	}
	if name == "" {
		name = accountKey(a)
	}
	if name == "" {
		name = "account"
	}
	provider := strings.TrimSpace(a.Provider)
	if provider != "" && !strings.EqualFold(name, provider) {
		name = fmt.Sprintf("%s (%s)", name, provider)
	}
	if quota != nil && quota.Plan != "" {
		name = fmt.Sprintf("%s · %s", name, quota.Plan)
	}
	return name
}

func lastBucketTotal(buckets []RequestBucket) float64 {
	if len(buckets) == 0 {
		return 0
	}
	b := buckets[len(buckets)-1]
	return b.Success + b.Failed
}

func bucketTotals(buckets []RequestBucket) (success, failed float64) {
	for _, b := range buckets {
		success += b.Success
		failed += b.Failed
	}
	return success, failed
}

// successRate treats an idle window as 100%: no traffic means nothing failed,
// and a "rate dropped" threshold should not fire on a quiet account.
func successRate(success, failed float64) float64 {
	total := success + failed
	if total <= 0 {
		return 100
	}
	return success / total * 100
}

// refreshAgeMinutes returns minutes since the credential's last OAuth token
// refresh. The field arrives as RFC3339 or a unix timestamp depending on the
// provider's auth file.
func refreshAgeMinutes(raw string, now time.Time) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	var t time.Time
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		t = parsed
	} else if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		// Seconds or milliseconds depending on magnitude.
		if n > 1e12 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
	} else {
		return 0, false
	}
	age := now.Sub(t).Minutes()
	if age < 0 {
		age = 0
	}
	return age, true
}

func readingsForAccount(sensorID string, a Account, quota *AccountQuota, extremaStore map[string]*extrema) []hwsensorsservice.Reading {
	var out []hwsensorsservice.Reading
	add := func(path, label, typ, unit string, value float64) {
		out = append(out, addReading(sensorID, path, label, typ, unit, value, extremaStore))
	}

	ready := 0.0
	if accountReady(a) {
		ready = 1
	}
	add("/status/0", "Ready", "Level", "", ready)
	add("/requests/0", "Requests (10m)", "Level", "req", lastBucketTotal(a.RecentRequests))
	winSuccess, winFailed := bucketTotals(a.RecentRequests)
	add("/requests/1", "Requests (window)", "Level", "req", winSuccess+winFailed)
	add("/success/0", "Success", "Level", "req", a.Success)
	add("/failed/0", "Failed", "Level", "req", a.Failed)
	add("/rate/0", "Success rate", "Load", "%", successRate(winSuccess, winFailed))
	if age, ok := refreshAgeMinutes(a.LastRefresh.String(), time.Now()); ok {
		add("/refresh/0", "Token refresh age", "Time", "min", age)
	}
	if quota != nil {
		addQuotaWindow(add, "/quota/session", quota.Primary)
		addQuotaWindow(add, "/quota/weekly", quota.Secondary)
		addQuotaWindow(add, "/quota/monthly", quota.Tertiary)
		if quota.ExtraSpend != nil {
			add("/quota/spend/0", "Extra usage spend", "Level", "", *quota.ExtraSpend)
		}
	}
	return out
}

// addQuotaWindow emits a "used %" gauge and a "resets in" countdown for one
// provider window — the Headroom-style pair. The reset reading is minutes
// until reset; negative values (overdue resets) clamp to zero.
func addQuotaWindow(add func(path, label, typ, unit string, value float64), base string, w *QuotaWindow) {
	if w == nil {
		return
	}
	label := w.Label
	if label == "" {
		label = "Session"
	}
	add(base+"/used/0", label+" used", "Load", "%", w.UsedPercent)
	if w.ResetAt != nil {
		mins := time.Until(*w.ResetAt).Minutes()
		if mins < 0 {
			mins = 0
		}
		add(base+"/reset/0", label+" resets in", "Time", "min", mins)
	}
}

func addReading(sensorID, path, label, typ, unit string, value float64, store map[string]*extrema) hwsensorsservice.Reading {
	id := sensorID + path
	min, max := track(store, id, value)
	return reading{
		id:              makeReadingID(sensorID, id),
		label:           label,
		unit:            unit,
		typ:             typ,
		typeI:           mapType(typ),
		value:           value,
		normalizedValue: value,
		min:             min,
		max:             max,
		average:         value,
	}
}

func mapType(typ string) hwsensorsservice.ReadingType {
	switch strings.ToLower(typ) {
	case "load", "level", "control":
		return hwsensorsservice.ReadingTypeUsage
	default:
		return hwsensorsservice.ReadingTypeOther
	}
}

func makeReadingID(sensorID, readingID string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sensorID))
	_, _ = h.Write([]byte(readingID))
	return int32(h.Sum32() & 0x7fffffff)
}

// ReadingIDFor is exported for tests that need to assert picker IDs.
func ReadingIDFor(sensorID, path string) int32 {
	return makeReadingID(sensorID, sensorID+path)
}
