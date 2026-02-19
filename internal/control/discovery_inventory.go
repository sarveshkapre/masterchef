package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	InventoryDiscoveryConsul     = "consul"
	InventoryDiscoveryKubernetes = "kubernetes"
	InventoryDiscoveryCloudTags  = "cloud_tags"
	InventoryDiscoveryAWS        = "aws"
	InventoryDiscoveryAzure      = "azure"
	InventoryDiscoveryGCP        = "gcp"
	InventoryDiscoveryVSphere    = "vsphere"
)

type DiscoverySourceInput struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Endpoint      string            `json:"endpoint"`
	Query         string            `json:"query,omitempty"`
	DefaultLabels map[string]string `json:"default_labels,omitempty"`
	Enabled       bool              `json:"enabled"`
}

type DiscoverySource struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Endpoint      string            `json:"endpoint"`
	Query         string            `json:"query,omitempty"`
	DefaultLabels map[string]string `json:"default_labels,omitempty"`
	Enabled       bool              `json:"enabled"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type DiscoveredHost struct {
	Name      string            `json:"name"`
	Address   string            `json:"address,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Roles     []string          `json:"roles,omitempty"`
	Topology  map[string]string `json:"topology,omitempty"`
}

type DiscoverySyncInput struct {
	SourceID string           `json:"source_id"`
	Hosts    []DiscoveredHost `json:"hosts"`
}

type DiscoverySyncResult struct {
	SourceID       string    `json:"source_id"`
	Kind           string    `json:"kind"`
	RequestedHosts int       `json:"requested_hosts"`
	ValidHosts     int       `json:"valid_hosts"`
	GeneratedAt    time.Time `json:"generated_at"`
}

type DiscoveryInventoryStore struct {
	mu      sync.RWMutex
	nextID  int64
	sources map[string]*DiscoverySource
}

func NewDiscoveryInventoryStore() *DiscoveryInventoryStore {
	return &DiscoveryInventoryStore{sources: map[string]*DiscoverySource{}}
}

func (s *DiscoveryInventoryStore) CreateSource(in DiscoverySourceInput) (DiscoverySource, error) {
	name := strings.TrimSpace(in.Name)
	kind := normalizeDiscoveryKind(in.Kind)
	endpoint := strings.TrimSpace(in.Endpoint)
	if name == "" || kind == "" || endpoint == "" {
		return DiscoverySource{}, errors.New("name, kind, and endpoint are required")
	}
	item := DiscoverySource{
		Name:          name,
		Kind:          kind,
		Endpoint:      endpoint,
		Query:         strings.TrimSpace(in.Query),
		DefaultLabels: normalizeStringMap(in.DefaultLabels),
		Enabled:       in.Enabled,
		UpdatedAt:     time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextID++
	item.ID = "discovery-source-" + itoa(s.nextID)
	s.sources[item.ID] = &item
	s.mu.Unlock()
	return cloneDiscoverySource(item), nil
}

func (s *DiscoveryInventoryStore) ListSources() []DiscoverySource {
	s.mu.RLock()
	out := make([]DiscoverySource, 0, len(s.sources))
	for _, item := range s.sources {
		out = append(out, cloneDiscoverySource(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *DiscoveryInventoryStore) GetSource(id string) (DiscoverySource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.sources[strings.TrimSpace(id)]
	if !ok {
		return DiscoverySource{}, false
	}
	return cloneDiscoverySource(*item), true
}

func (s *DiscoveryInventoryStore) PrepareSync(in DiscoverySyncInput) (DiscoverySource, []NodeEnrollInput, DiscoverySyncResult, error) {
	source, ok := s.GetSource(in.SourceID)
	if !ok {
		return DiscoverySource{}, nil, DiscoverySyncResult{}, errors.New("discovery source not found")
	}
	if !source.Enabled {
		return DiscoverySource{}, nil, DiscoverySyncResult{}, errors.New("discovery source is disabled")
	}
	enrolls := make([]NodeEnrollInput, 0, len(in.Hosts))
	for _, host := range in.Hosts {
		name := strings.TrimSpace(host.Name)
		if name == "" {
			continue
		}
		labels := normalizeStringMap(host.Labels)
		if len(source.DefaultLabels) > 0 {
			if labels == nil {
				labels = map[string]string{}
			}
			for k, v := range source.DefaultLabels {
				if _, exists := labels[k]; !exists {
					labels[k] = v
				}
			}
		}
		enrolls = append(enrolls, NodeEnrollInput{
			Name:      name,
			Address:   strings.TrimSpace(host.Address),
			Transport: strings.ToLower(strings.TrimSpace(host.Transport)),
			Labels:    labels,
			Roles:     normalizeStringSlice(host.Roles),
			Topology:  normalizeStringMap(host.Topology),
			Source:    "discovery:" + source.Kind,
		})
	}
	return source, enrolls, DiscoverySyncResult{
		SourceID:       source.ID,
		Kind:           source.Kind,
		RequestedHosts: len(in.Hosts),
		ValidHosts:     len(enrolls),
		GeneratedAt:    time.Now().UTC(),
	}, nil
}

func normalizeDiscoveryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case InventoryDiscoveryConsul, InventoryDiscoveryKubernetes, InventoryDiscoveryCloudTags, InventoryDiscoveryAWS, InventoryDiscoveryAzure, InventoryDiscoveryGCP, InventoryDiscoveryVSphere:
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return ""
	}
}

func cloneDiscoverySource(in DiscoverySource) DiscoverySource {
	out := in
	out.DefaultLabels = normalizeStringMap(in.DefaultLabels)
	return out
}
