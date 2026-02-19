package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type FederationPeerInput struct {
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
	Mode     string `json:"mode"`
	Weight   int    `json:"weight,omitempty"`
}

type FederationPeer struct {
	ID        string    `json:"id"`
	Region    string    `json:"region"`
	Endpoint  string    `json:"endpoint"`
	Mode      string    `json:"mode"`
	Weight    int       `json:"weight"`
	Healthy   bool      `json:"healthy"`
	LatencyMs int       `json:"latency_ms"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FederationHealthRow struct {
	Region    string `json:"region"`
	Mode      string `json:"mode"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int    `json:"latency_ms"`
}

type FederationHealthMatrix struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Rows        []FederationHealthRow `json:"rows"`
	Healthy     bool                  `json:"healthy"`
}

type FederationStore struct {
	mu    sync.RWMutex
	next  int64
	peers map[string]*FederationPeer
}

func NewFederationStore() *FederationStore {
	return &FederationStore{peers: map[string]*FederationPeer{}}
}

func (s *FederationStore) UpsertPeer(in FederationPeerInput) (FederationPeer, error) {
	region := strings.ToLower(strings.TrimSpace(in.Region))
	endpoint := strings.TrimSpace(in.Endpoint)
	mode := normalizeFederationMode(in.Mode)
	if region == "" || endpoint == "" || mode == "" {
		return FederationPeer{}, errors.New("region, endpoint, and mode are required")
	}
	weight := in.Weight
	if weight <= 0 {
		weight = 100
	}
	item := FederationPeer{
		Region:    region,
		Endpoint:  endpoint,
		Mode:      mode,
		Weight:    weight,
		Healthy:   true,
		LatencyMs: 25,
		UpdatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.next++
	item.ID = "federation-peer-" + itoa(s.next)
	s.peers[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *FederationStore) GetPeer(id string) (FederationPeer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.peers[strings.TrimSpace(id)]
	if !ok {
		return FederationPeer{}, false
	}
	return *item, true
}

func (s *FederationStore) ListPeers() []FederationPeer {
	s.mu.RLock()
	out := make([]FederationPeer, 0, len(s.peers))
	for _, item := range s.peers {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Region < out[j].Region })
	return out
}

func (s *FederationStore) SetPeerHealth(id string, healthy bool, latencyMs int) (FederationPeer, error) {
	if latencyMs < 0 {
		latencyMs = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.peers[strings.TrimSpace(id)]
	if !ok {
		return FederationPeer{}, errors.New("federation peer not found")
	}
	item.Healthy = healthy
	item.LatencyMs = latencyMs
	item.UpdatedAt = time.Now().UTC()
	return *item, nil
}

func (s *FederationStore) HealthMatrix() FederationHealthMatrix {
	rows := make([]FederationHealthRow, 0)
	healthy := true
	for _, peer := range s.ListPeers() {
		rows = append(rows, FederationHealthRow{
			Region:    peer.Region,
			Mode:      peer.Mode,
			Healthy:   peer.Healthy,
			LatencyMs: peer.LatencyMs,
		})
		if !peer.Healthy {
			healthy = false
		}
	}
	return FederationHealthMatrix{
		GeneratedAt: time.Now().UTC(),
		Rows:        rows,
		Healthy:     healthy,
	}
}

func normalizeFederationMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "active_active", "active_passive":
		return mode
	default:
		return ""
	}
}
