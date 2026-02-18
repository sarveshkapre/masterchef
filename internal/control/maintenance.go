package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var validMaintenanceKinds = map[string]struct{}{
	"host":        {},
	"cluster":     {},
	"environment": {},
}

type MaintenanceTarget struct {
	Kind    string    `json:"kind"`
	Name    string    `json:"name"`
	Enabled bool      `json:"enabled"`
	Reason  string    `json:"reason,omitempty"`
	Since   time.Time `json:"since,omitempty"`
}

type MaintenanceStore struct {
	mu      sync.RWMutex
	targets map[string]*MaintenanceTarget
}

func NewMaintenanceStore() *MaintenanceStore {
	return &MaintenanceStore{targets: map[string]*MaintenanceTarget{}}
}

func (m *MaintenanceStore) Set(kind, name string, enabled bool, reason string) (MaintenanceTarget, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	name = strings.TrimSpace(name)
	if _, ok := validMaintenanceKinds[kind]; !ok {
		return MaintenanceTarget{}, errors.New("invalid maintenance kind; must be host, cluster, or environment")
	}
	if name == "" {
		return MaintenanceTarget{}, errors.New("maintenance target name is required")
	}

	key := kind + ":" + strings.ToLower(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	cur, ok := m.targets[key]
	if !ok {
		cur = &MaintenanceTarget{Kind: kind, Name: name}
		m.targets[key] = cur
	}
	cur.Enabled = enabled
	if enabled {
		if cur.Since.IsZero() {
			cur.Since = time.Now().UTC()
		}
		cur.Reason = strings.TrimSpace(reason)
	} else {
		cur.Since = time.Time{}
		cur.Reason = ""
	}
	return *cur, nil
}

func (m *MaintenanceStore) IsActive(kind, name string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	name = strings.ToLower(strings.TrimSpace(name))
	if kind == "" || name == "" {
		return false
	}

	key := kind + ":" + name
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.targets[key]
	return ok && t.Enabled
}

func (m *MaintenanceStore) List() []MaintenanceTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]MaintenanceTarget, 0, len(m.targets))
	for _, t := range m.targets {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
