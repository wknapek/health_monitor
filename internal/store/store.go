package store

import (
	"sync"
	"time"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

type Store struct {
	mu      sync.RWMutex
	devices map[string]*model.Device
	diags   map[string]*model.Diagnostics
}

func New() *Store {
	return &Store{
		devices: make(map[string]*model.Device),
		diags:   make(map[string]*model.Diagnostics),
	}
}

func (s *Store) AddDevice(d *model.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[d.ID] = d
}

func (s *Store) RemoveDevice(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; ok {
		delete(s.devices, id)
		delete(s.diags, id)
		return true
	}
	return false
}

func (s *Store) GetDevice(id string) (*model.Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	return d, ok
}

func (s *Store) ListDevices() []*model.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Device, 0, len(s.devices))
	for _, d := range s.devices {
		result = append(result, d)
	}
	return result
}

func (s *Store) UpdateDeviceStatus(id string, status pb.DeviceStatus, caps []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.devices[id]; ok {
		d.Status = status
		d.LastChecked = time.Now()
		d.Capabilities = caps
	}
}

func (s *Store) SetDiagnostics(id string, diag *model.Diagnostics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diags[id] = diag
}

func (s *Store) GetDiagnostics(id string) (*model.Diagnostics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.diags[id]
	return d, ok
}
