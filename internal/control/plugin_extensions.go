package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PluginExtensionType string

const (
	PluginCallback PluginExtensionType = "callback"
	PluginLookup   PluginExtensionType = "lookup"
	PluginFilter   PluginExtensionType = "filter"
	PluginVars     PluginExtensionType = "vars"
	PluginStrategy PluginExtensionType = "strategy"
)

type PluginExtension struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Type        PluginExtensionType `json:"type"`
	Description string              `json:"description,omitempty"`
	Entrypoint  string              `json:"entrypoint"`
	Version     string              `json:"version,omitempty"`
	Config      map[string]any      `json:"config,omitempty"`
	Enabled     bool                `json:"enabled"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type PluginExtensionStore struct {
	mu    sync.RWMutex
	next  int64
	items map[string]PluginExtension
}

func NewPluginExtensionStore() *PluginExtensionStore {
	return &PluginExtensionStore{
		items: map[string]PluginExtension{},
	}
}

func (s *PluginExtensionStore) Create(ext PluginExtension) (PluginExtension, error) {
	typ := normalizePluginType(ext.Type)
	if typ == "" {
		return PluginExtension{}, errors.New("type must be one of callback, lookup, filter, vars, strategy")
	}
	name := strings.TrimSpace(ext.Name)
	if name == "" {
		return PluginExtension{}, errors.New("name is required")
	}
	entrypoint := strings.TrimSpace(ext.Entrypoint)
	if entrypoint == "" {
		return PluginExtension{}, errors.New("entrypoint is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	now := time.Now().UTC()
	item := PluginExtension{
		ID:          "plugin-" + itoa(s.next),
		Name:        name,
		Type:        typ,
		Description: strings.TrimSpace(ext.Description),
		Entrypoint:  entrypoint,
		Version:     strings.TrimSpace(ext.Version),
		Config:      cloneVariableMap(ext.Config),
		Enabled:     ext.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.items[item.ID] = clonePluginExtension(item)
	return clonePluginExtension(item), nil
}

func (s *PluginExtensionStore) List(filterType string) []PluginExtension {
	s.mu.RLock()
	defer s.mu.RUnlock()
	filterType = strings.TrimSpace(strings.ToLower(filterType))
	out := make([]PluginExtension, 0, len(s.items))
	for _, item := range s.items {
		if filterType != "" && string(item.Type) != filterType {
			continue
		}
		out = append(out, clonePluginExtension(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *PluginExtensionStore) Get(id string) (PluginExtension, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return PluginExtension{}, errors.New("plugin extension not found")
	}
	return clonePluginExtension(item), nil
}

func (s *PluginExtensionStore) Delete(id string) bool {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

func (s *PluginExtensionStore) SetEnabled(id string, enabled bool) (PluginExtension, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return PluginExtension{}, errors.New("plugin extension not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	s.items[id] = item
	return clonePluginExtension(item), nil
}

func normalizePluginType(typ PluginExtensionType) PluginExtensionType {
	switch strings.ToLower(strings.TrimSpace(string(typ))) {
	case string(PluginCallback):
		return PluginCallback
	case string(PluginLookup):
		return PluginLookup
	case string(PluginFilter):
		return PluginFilter
	case string(PluginVars):
		return PluginVars
	case string(PluginStrategy):
		return PluginStrategy
	default:
		return ""
	}
}

func clonePluginExtension(in PluginExtension) PluginExtension {
	out := in
	out.Config = cloneVariableMap(in.Config)
	return out
}
