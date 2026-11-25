package solaredge

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// ModeAPI / ModeModbus are solar_source setting values.
const (
	ModeAPI    = "api"
	ModeModbus = "modbus"
)

// Switchable picks API or Modbus at runtime (settings toggle).
type Switchable struct {
	mu   sync.RWMutex
	mode string
	api  *OAuthClient
	mb   *ModbusClient
}

// NewSwitchable wraps both backends. mode should be "api" or "modbus".
func NewSwitchable(mode string, api *OAuthClient, mb *ModbusClient) *Switchable {
	s := &Switchable{api: api, mb: mb}
	s.SetMode(mode)
	return s
}

func (s *Switchable) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.activeLocked()
	return src != nil && src.Enabled()
}

func (s *Switchable) Fetch(ctx context.Context) (Snapshot, error) {
	s.mu.RLock()
	src := s.activeLocked()
	mode := s.mode
	s.mu.RUnlock()
	if src == nil {
		return Snapshot{}, fmt.Errorf("solaredge: no %s backend", mode)
	}
	return src.Fetch(ctx)
}

func (s *Switchable) activeLocked() Source {
	switch s.mode {
	case ModeModbus:
		if s.mb != nil {
			return s.mb
		}
	default:
		if s.api != nil {
			return s.api
		}
	}
	return nil
}

// Mode returns the active backend name.
func (s *Switchable) Mode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// SetMode switches between api and modbus.
func (s *Switchable) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != ModeModbus {
		mode = ModeAPI
	}
	s.mode = mode
}

// SetModbus updates the Modbus TCP endpoint.
func (s *Switchable) SetModbus(addr string, unit byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mb == nil {
		s.mb = NewModbus(ModbusOptions{Addr: addr, UnitID: unit})
		return
	}
	s.mb.SetOptions(ModbusOptions{Addr: addr, UnitID: unit})
}

// API returns the OAuth client (may be nil).
func (s *Switchable) API() *OAuthClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.api
}
