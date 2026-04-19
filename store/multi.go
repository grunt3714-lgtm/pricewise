package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Backend names a data source. A single physical store may have several.
type Backend string

const (
	BackendDirect Backend = "direct" // the store's own site/API/scraping
	BackendFlipp  Backend = "flipp"  // the Flipp aggregator
	BackendAuto   Backend = "auto"   // let the store pick; order defined below
)

// BackendStore is a Store that also reports which backend it represents.
// Adapters that can live inside a MultiStore implement this.
type BackendStore interface {
	Store
	Backend() Backend
}

// MultiStore dispatches Fetch across one or more backends for a single
// physical store. With Fallback enabled and Backend == BackendAuto, it
// tries backends in order and returns the first success. With a specific
// Backend, it tries only that one.
type MultiStore struct {
	id       string
	backends []BackendStore // priority order; index 0 is primary
	prefer   Backend        // which backend the caller wants (default: auto)
	fallback bool           // try remaining backends on error
}

// NewMulti constructs a MultiStore. The first backend is the primary.
// A MultiStore with zero backends is invalid; callers should always
// supply at least one.
func NewMulti(id string, backends ...BackendStore) *MultiStore {
	return &MultiStore{id: id, backends: backends, prefer: BackendAuto, fallback: true}
}

// Name returns the store's stable ID (not the backend's).
func (m *MultiStore) Name() string { return m.id }

// WithBackend returns a copy configured for the given backend preference.
// Pass BackendAuto to restore default behavior.
func (m *MultiStore) WithBackend(b Backend) *MultiStore {
	cp := *m
	cp.prefer = b
	return &cp
}

// WithFallback returns a copy with fallback enabled/disabled.
func (m *MultiStore) WithFallback(enabled bool) *MultiStore {
	cp := *m
	cp.fallback = enabled
	return &cp
}

// Backends returns which backends this store has configured, in order.
func (m *MultiStore) Backends() []Backend {
	out := make([]Backend, 0, len(m.backends))
	for _, b := range m.backends {
		out = append(out, b.Backend())
	}
	return out
}

// HasBackend reports whether b is one of the configured backends.
func (m *MultiStore) HasBackend(b Backend) bool {
	for _, x := range m.backends {
		if x.Backend() == b {
			return true
		}
	}
	return false
}

// Fetch dispatches according to prefer and fallback. A failure on the
// chosen backend is returned unless fallback is enabled and more backends
// remain. An unknown preferred backend returns ErrBackendUnavailable.
func (m *MultiStore) Fetch(ctx context.Context) ([]Item, error) {
	if len(m.backends) == 0 {
		return nil, fmt.Errorf("%s: no backends configured", m.id)
	}
	order := m.order()
	if len(order) == 0 {
		return nil, fmt.Errorf("%s: backend %q not available (have: %s)",
			m.id, m.prefer, joinBackends(m.backends))
	}
	var errs []error
	for _, b := range order {
		items, err := b.Fetch(ctx)
		if err == nil {
			return items, nil
		}
		errs = append(errs, fmt.Errorf("%s/%s: %w", m.id, b.Backend(), err))
		if !m.fallback {
			break
		}
	}
	return nil, errors.Join(errs...)
}

// order resolves prefer into the actual backend sequence to try.
func (m *MultiStore) order() []BackendStore {
	if m.prefer == BackendAuto || m.prefer == "" {
		return m.backends
	}
	// Find the requested backend. If fallback is off, return just it.
	// If on, put it first and append the rest.
	var primary BackendStore
	var rest []BackendStore
	for _, b := range m.backends {
		if b.Backend() == m.prefer {
			primary = b
		} else {
			rest = append(rest, b)
		}
	}
	if primary == nil {
		return nil
	}
	if !m.fallback {
		return []BackendStore{primary}
	}
	return append([]BackendStore{primary}, rest...)
}

func joinBackends(bs []BackendStore) string {
	names := make([]string, 0, len(bs))
	for _, b := range bs {
		names = append(names, string(b.Backend()))
	}
	return strings.Join(names, ",")
}

// ErrBackendUnavailable is returned by adapters that can't run without
// optional configuration (e.g. missing OAuth credentials). The multi-
// store chain treats it like any other error — the next backend is
// tried if fallback is enabled.
var ErrBackendUnavailable = errors.New("backend unavailable")
