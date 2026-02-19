package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type HopRelayEndpointInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Region      string `json:"region"`
	URL         string `json:"url"`
	MaxSessions int    `json:"max_sessions,omitempty"`
}

type HopRelayEndpoint struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Region      string    `json:"region"`
	URL         string    `json:"url"`
	MaxSessions int       `json:"max_sessions"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type HopRelaySessionInput struct {
	EndpointID string `json:"endpoint_id"`
	NodeID     string `json:"node_id"`
	TargetHost string `json:"target_host"`
}

type HopRelaySession struct {
	ID         string    `json:"id"`
	EndpointID string    `json:"endpoint_id"`
	NodeID     string    `json:"node_id"`
	TargetHost string    `json:"target_host"`
	EgressOnly bool      `json:"egress_only"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type HopRelayStore struct {
	mu        sync.RWMutex
	nextEP    int64
	nextSes   int64
	endpoints map[string]*HopRelayEndpoint
	sessions  map[string]*HopRelaySession
}

func NewHopRelayStore() *HopRelayStore {
	return &HopRelayStore{
		endpoints: map[string]*HopRelayEndpoint{},
		sessions:  map[string]*HopRelaySession{},
	}
}

func (s *HopRelayStore) UpsertEndpoint(in HopRelayEndpointInput) (HopRelayEndpoint, error) {
	name := strings.TrimSpace(in.Name)
	kind := normalizeRelayKind(in.Kind)
	region := strings.TrimSpace(in.Region)
	url := strings.TrimSpace(in.URL)
	if name == "" || kind == "" || url == "" {
		return HopRelayEndpoint{}, errors.New("name, kind, and url are required")
	}
	if region == "" {
		region = "global"
	}
	maxSessions := in.MaxSessions
	if maxSessions <= 0 {
		maxSessions = 1000
	}
	item := HopRelayEndpoint{
		Name:        name,
		Kind:        kind,
		Region:      region,
		URL:         url,
		MaxSessions: maxSessions,
		UpdatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextEP++
	item.ID = "relay-endpoint-" + itoa(s.nextEP)
	s.endpoints[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *HopRelayStore) GetEndpoint(id string) (HopRelayEndpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.endpoints[strings.TrimSpace(id)]
	if !ok {
		return HopRelayEndpoint{}, false
	}
	return *item, true
}

func (s *HopRelayStore) ListEndpoints() []HopRelayEndpoint {
	s.mu.RLock()
	out := make([]HopRelayEndpoint, 0, len(s.endpoints))
	for _, item := range s.endpoints {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *HopRelayStore) OpenSession(in HopRelaySessionInput) (HopRelaySession, error) {
	endpointID := strings.TrimSpace(in.EndpointID)
	nodeID := strings.TrimSpace(in.NodeID)
	target := strings.TrimSpace(in.TargetHost)
	if endpointID == "" || nodeID == "" || target == "" {
		return HopRelaySession{}, errors.New("endpoint_id, node_id, and target_host are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ep, ok := s.endpoints[endpointID]
	if !ok {
		return HopRelaySession{}, errors.New("relay endpoint not found")
	}
	active := 0
	for _, item := range s.sessions {
		if item.EndpointID == endpointID && item.Status == "active" {
			active++
		}
	}
	if active >= ep.MaxSessions {
		return HopRelaySession{}, errors.New("relay endpoint at capacity")
	}
	s.nextSes++
	item := HopRelaySession{
		ID:         "relay-session-" + itoa(s.nextSes),
		EndpointID: endpointID,
		NodeID:     nodeID,
		TargetHost: target,
		EgressOnly: true,
		Status:     "active",
		CreatedAt:  time.Now().UTC(),
	}
	s.sessions[item.ID] = &item
	return item, nil
}

func (s *HopRelayStore) ListSessions(limit int) []HopRelaySession {
	s.mu.RLock()
	out := make([]HopRelaySession, 0, len(s.sessions))
	for _, item := range s.sessions {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeRelayKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "hop", "ingress":
		return kind
	default:
		return ""
	}
}
