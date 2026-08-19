package sparkdash

import (
	"fmt"
	"sync"
	"time"

	hwsensorsservice "github.com/moeilijk/lhm-streamdeck/pkg/service"
)

// Service polls one SparkDash unit and exposes it as a HardwareService.
type Service struct {
	client  *Client
	sparkID string

	mu       sync.RWMutex
	fetchMu  sync.Mutex
	pollTime uint64
	mapped   mappedSnapshot
	extrema  map[string]*extrema
	ready    bool
}

// NewService builds a HardwareService for a SparkDash host/port/unit.
func NewService(host string, port int, sparkID string) *Service {
	return &Service{
		client:  NewClient(host, port),
		sparkID: sparkID,
		extrema: make(map[string]*extrema),
	}
}

// NewServiceWithClient is used by tests.
func NewServiceWithClient(c *Client, sparkID string) *Service {
	return &Service{
		client:  c,
		sparkID: sparkID,
		extrema: make(map[string]*extrema),
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
		return 0, fmt.Errorf("sparkDash data unavailable")
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
		return nil, fmt.Errorf("sparkDash data unavailable")
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
		return nil, fmt.Errorf("sparkDash data unavailable")
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

	snap, err := s.client.Snapshot(s.sparkID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.mapped = mapSnapshot(snap, s.extrema)
	s.pollTime = uint64(time.Now().UnixNano())
	s.ready = true
	s.mu.Unlock()
	return nil
}
