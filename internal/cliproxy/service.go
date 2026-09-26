package cliproxy

import (
	"fmt"
	"sync"
	"time"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

// quotaInterval paces provider usage calls — they hit Anthropic/OpenAI's real
// APIs, so they must not run at the deck's per-second poll cadence.
const quotaInterval = 5 * time.Minute

// Service polls a CLI Proxy management API and exposes its accounts as a
// HardwareService: one sensor per authenticated credential plus a proxy-wide
// summary sensor. Provider quota windows (session/weekly usage) are fetched
// on a slower cadence and merged into the account sensors' readings.
type Service struct {
	client *Client

	mu            sync.RWMutex
	fetchMu       sync.Mutex
	pollTime      uint64
	mapped        mappedAccounts
	accounts      map[string]Account
	extrema       map[string]*extrema
	quotas        map[string]*AccountQuota
	quotaAt       time.Time
	quotaInterval time.Duration
	ready         bool
}

// NewService builds a HardwareService for a CLI Proxy host/port/key.
func NewService(host string, port int, managementKey string) *Service {
	return NewServiceWithClient(NewClient(host, port, managementKey))
}

// NewServiceWithClient is used by tests.
func NewServiceWithClient(c *Client) *Service {
	return &Service{
		client:        c,
		accounts:      make(map[string]Account),
		extrema:       make(map[string]*extrema),
		quotas:        make(map[string]*AccountQuota),
		quotaInterval: quotaInterval,
	}
}

// PollTime implements HardwareService and is the poll trigger.
func (s *Service) PollTime() (uint64, error) {
	if err := s.refresh(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return 0, fmt.Errorf("cliProxy data unavailable")
	}
	return s.pollTime, nil
}

// Sensors implements HardwareService.
func (s *Service) Sensors() ([]hwsensorsservice.Sensor, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return nil, fmt.Errorf("cliProxy data unavailable")
	}
	out := make([]hwsensorsservice.Sensor, len(s.mapped.sensors))
	copy(out, s.mapped.sensors)
	return out, nil
}

// ReadingsForSensorID implements HardwareService.
func (s *Service) ReadingsForSensorID(id string) ([]hwsensorsservice.Reading, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return nil, fmt.Errorf("cliProxy data unavailable")
	}
	rs, ok := s.mapped.readings[id]
	if !ok {
		return nil, fmt.Errorf("sensor %s not found", id)
	}
	out := make([]hwsensorsservice.Reading, len(rs))
	copy(out, rs)
	return out, nil
}

func (s *Service) ensureReady() error {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()
	if ready {
		return nil
	}
	return s.refresh()
}

func (s *Service) refresh() error {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	accounts, err := s.client.Accounts()
	if err != nil {
		return err
	}

	// Quota refresh runs off the poll path: upstream calls take seconds, and
	// the readings simply land on the next poll once the map is updated.
	if time.Since(s.quotaAt) >= s.quotaInterval {
		s.quotaAt = time.Now()
		go s.refreshQuotas(accounts)
	}

	s.mu.Lock()
	byKey := make(map[string]Account, len(accounts))
	for _, a := range accounts {
		if k := accountKey(a); k != "" {
			if _, dupe := byKey[k]; !dupe {
				byKey[k] = a
			}
		}
	}
	s.accounts = byKey
	s.mapped = mapAccounts(accounts, s.quotas, s.extrema)
	s.pollTime = uint64(time.Now().UnixNano())
	s.ready = true
	s.mu.Unlock()
	return nil
}

// refreshQuotas updates provider usage snapshots for accounts whose provider
// exposes a quota endpoint. Calls run concurrently so several upstream
// requests can't serialize into a long poll stall; failures keep the
// previous snapshot — a transient upstream error shouldn't blank readings.
func (s *Service) refreshQuotas(accounts []Account) {
	type result struct {
		key string
		q   *AccountQuota
	}
	results := make(chan result, len(accounts))
	var wg sync.WaitGroup
	seen := map[string]bool{}
	for _, a := range accounts {
		key := accountKey(a)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		wg.Add(1)
		go func(a Account, key string) {
			defer wg.Done()
			if q, err := s.client.fetchQuota(a); err == nil && q != nil {
				results <- result{key, q}
			}
		}(a, key)
	}
	wg.Wait()
	close(results)

	s.mu.Lock()
	for r := range results {
		s.quotas[r.key] = r.q
	}
	s.mu.Unlock()
}
