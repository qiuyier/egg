// Package cache defines a common cache contract and provides Otter and Redis adapters.
package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrMiss       = errors.New("cache miss")
	ErrInvalidTTL = errors.New("cache TTL must be non-negative")
)

// Adapter is safe for concurrent use. Get returns ErrMiss for absent or expired
// keys. Set replaces the value and TTL together. A zero TTL disables expiration,
// but the backend may still evict the entry. Delete succeeds for missing keys.
// Callers may modify the bytes passed to Set or returned by Get after the call.
type Adapter interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Manager is a concurrent registry of named adapters. The caller owns their lifecycle.
type Manager struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func (m *Manager) Register(name string, adapter Adapter) error {
	if strings.TrimSpace(name) == "" || adapter == nil {
		return errors.New("cache adapter requires a name and implementation")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.adapters[name]; exists {
		return fmt.Errorf("cache adapter %q is already registered", name)
	}
	if m.adapters == nil {
		m.adapters = make(map[string]Adapter)
	}
	m.adapters[name] = adapter
	return nil
}

func (m *Manager) Lookup(name string) (Adapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	adapter, found := m.adapters[name]
	return adapter, found
}
