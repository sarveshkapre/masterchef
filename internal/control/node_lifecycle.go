package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	NodeStatusBootstrap      = "bootstrap"
	NodeStatusActive         = "active"
	NodeStatusQuarantined    = "quarantined"
	NodeStatusDecommissioned = "decommissioned"
)

type NodeStatusChange struct {
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type ManagedNode struct {
	Name       string             `json:"name"`
	Address    string             `json:"address,omitempty"`
	Transport  string             `json:"transport,omitempty"`
	Labels     map[string]string  `json:"labels,omitempty"`
	Roles      []string           `json:"roles,omitempty"`
	Topology   map[string]string  `json:"topology,omitempty"`
	Source     string             `json:"source,omitempty"`
	Status     string             `json:"status"`
	EnrolledAt time.Time          `json:"enrolled_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	LastSeenAt time.Time          `json:"last_seen_at,omitempty"`
	History    []NodeStatusChange `json:"history,omitempty"`
}

type NodeEnrollInput struct {
	Name      string            `json:"name"`
	Address   string            `json:"address,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Roles     []string          `json:"roles,omitempty"`
	Topology  map[string]string `json:"topology,omitempty"`
	Source    string            `json:"source,omitempty"`
}

type NodeLifecycleStore struct {
	mu    sync.RWMutex
	nodes map[string]*ManagedNode
}

func NewNodeLifecycleStore() *NodeLifecycleStore {
	return &NodeLifecycleStore{
		nodes: map[string]*ManagedNode{},
	}
}

func (s *NodeLifecycleStore) Enroll(in NodeEnrollInput) (ManagedNode, bool, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ManagedNode{}, false, errors.New("node name is required")
	}
	now := time.Now().UTC()
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "api"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.nodes[name]
	if !exists {
		node := &ManagedNode{
			Name:       name,
			Address:    strings.TrimSpace(in.Address),
			Transport:  strings.ToLower(strings.TrimSpace(in.Transport)),
			Labels:     normalizeStringMap(in.Labels),
			Roles:      normalizeStringSlice(in.Roles),
			Topology:   normalizeStringMap(in.Topology),
			Source:     source,
			Status:     NodeStatusBootstrap,
			EnrolledAt: now,
			UpdatedAt:  now,
			History: []NodeStatusChange{
				{Status: NodeStatusBootstrap, Reason: "auto-enrolled", Timestamp: now},
			},
		}
		s.nodes[name] = node
		return cloneNode(*node), true, nil
	}
	current.Address = strings.TrimSpace(in.Address)
	current.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	current.Labels = normalizeStringMap(in.Labels)
	current.Roles = normalizeStringSlice(in.Roles)
	current.Topology = normalizeStringMap(in.Topology)
	current.Source = source
	current.UpdatedAt = now
	return cloneNode(*current), false, nil
}

func (s *NodeLifecycleStore) List(status string) []ManagedNode {
	status = strings.ToLower(strings.TrimSpace(status))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ManagedNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if status != "" && node.Status != status {
			continue
		}
		out = append(out, cloneNode(*node))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *NodeLifecycleStore) Get(name string) (ManagedNode, bool) {
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[name]
	if !ok {
		return ManagedNode{}, false
	}
	return cloneNode(*node), true
}

func (s *NodeLifecycleStore) SetStatus(name, status, reason string) (ManagedNode, error) {
	name = strings.TrimSpace(name)
	status = strings.ToLower(strings.TrimSpace(status))
	if name == "" {
		return ManagedNode{}, errors.New("node name is required")
	}
	switch status {
	case NodeStatusBootstrap, NodeStatusActive, NodeStatusQuarantined, NodeStatusDecommissioned:
	default:
		return ManagedNode{}, errors.New("unsupported node status: " + status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.nodes[name]
	if !ok {
		return ManagedNode{}, errors.New("node not found")
	}
	now := time.Now().UTC()
	node.Status = status
	node.UpdatedAt = now
	node.History = append(node.History, NodeStatusChange{
		Status:    status,
		Reason:    strings.TrimSpace(reason),
		Timestamp: now,
	})
	return cloneNode(*node), nil
}

func (s *NodeLifecycleStore) Heartbeat(name string) (ManagedNode, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ManagedNode{}, errors.New("node name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.nodes[name]
	if !ok {
		return ManagedNode{}, errors.New("node not found")
	}
	now := time.Now().UTC()
	node.LastSeenAt = now
	node.UpdatedAt = now
	return cloneNode(*node), nil
}

func cloneNode(in ManagedNode) ManagedNode {
	out := in
	out.Labels = normalizeStringMap(in.Labels)
	out.Topology = normalizeStringMap(in.Topology)
	out.Roles = append([]string{}, in.Roles...)
	out.History = append([]NodeStatusChange{}, in.History...)
	return out
}

func normalizeStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range in {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
