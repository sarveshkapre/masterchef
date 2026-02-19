package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SyndicNodeInput struct {
	Name    string `json:"name"`
	Parent  string `json:"parent,omitempty"`
	Role    string `json:"role"`
	Region  string `json:"region,omitempty"`
	Segment string `json:"segment,omitempty"`
}

type SyndicNode struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Parent    string    `json:"parent,omitempty"`
	Role      string    `json:"role"`
	Region    string    `json:"region,omitempty"`
	Segment   string    `json:"segment,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SyndicRouteInput struct {
	Target string `json:"target"`
}

type SyndicRoute struct {
	Target string   `json:"target"`
	Path   []string `json:"path"`
	Hops   int      `json:"hops"`
}

type SyndicStore struct {
	mu     sync.RWMutex
	nextID int64
	nodes  map[string]*SyndicNode
	byName map[string]string
}

func NewSyndicStore() *SyndicStore {
	return &SyndicStore{nodes: map[string]*SyndicNode{}, byName: map[string]string{}}
}

func (s *SyndicStore) Upsert(in SyndicNodeInput) (SyndicNode, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if name == "" || role == "" {
		return SyndicNode{}, errors.New("name and role are required")
	}
	if role != "master" && role != "syndic" && role != "minion" {
		return SyndicNode{}, errors.New("role must be master, syndic, or minion")
	}
	parent := strings.ToLower(strings.TrimSpace(in.Parent))
	if role == "master" && parent != "" {
		return SyndicNode{}, errors.New("parent is not allowed for master role")
	}
	if role != "master" && parent == "" {
		return SyndicNode{}, errors.New("parent is required for non-master roles")
	}
	if parent == name {
		return SyndicNode{}, errors.New("node cannot be its own parent")
	}

	item := SyndicNode{
		Name:      name,
		Parent:    parent,
		Role:      role,
		Region:    strings.ToLower(strings.TrimSpace(in.Region)),
		Segment:   strings.ToLower(strings.TrimSpace(in.Segment)),
		UpdatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if parent != "" {
		parentID, ok := s.byName[parent]
		if !ok {
			return SyndicNode{}, errors.New("parent node not found")
		}
		parentNode := s.nodes[parentID]
		if parentNode.Role == "minion" {
			return SyndicNode{}, errors.New("parent role cannot be minion")
		}
	}
	if role == "minion" && s.hasChildrenLocked(name) {
		return SyndicNode{}, errors.New("node with children cannot be set to minion role")
	}
	if id, ok := s.byName[name]; ok {
		item.ID = id
		s.nodes[id] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "syndic-node-" + itoa(s.nextID)
	s.nodes[item.ID] = &item
	s.byName[name] = item.ID
	return item, nil
}

func (s *SyndicStore) List() []SyndicNode {
	s.mu.RLock()
	out := make([]SyndicNode, 0, len(s.nodes))
	for _, item := range s.nodes {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *SyndicStore) ResolveRoute(target string) (SyndicRoute, error) {
	target = strings.ToLower(strings.TrimSpace(target))

	s.mu.RLock()
	node, ok := s.findByNameLocked(target)
	if !ok {
		s.mu.RUnlock()
		return SyndicRoute{}, errors.New("target node not found")
	}
	path := []string{node.Name}
	seen := map[string]struct{}{node.Name: {}}
	for parent := node.Parent; parent != ""; {
		if _, exists := seen[parent]; exists {
			return SyndicRoute{}, errors.New("cyclic parent topology detected")
		}
		seen[parent] = struct{}{}
		path = append(path, parent)
		pNode, ok := s.findByNameLocked(parent)
		if !ok {
			s.mu.RUnlock()
			return SyndicRoute{}, errors.New("broken parent link in topology")
		}
		parent = pNode.Parent
	}
	s.mu.RUnlock()
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return SyndicRoute{Target: node.Name, Path: path, Hops: len(path) - 1}, nil
}

func (s *SyndicStore) findByName(name string) (SyndicNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.findByNameLocked(name)
}

func (s *SyndicStore) findByNameLocked(name string) (SyndicNode, bool) {
	id, ok := s.byName[name]
	if !ok {
		return SyndicNode{}, false
	}
	item, ok := s.nodes[id]
	if !ok {
		return SyndicNode{}, false
	}
	return *item, true
}

func (s *SyndicStore) hasChildrenLocked(name string) bool {
	for _, node := range s.nodes {
		if node.Parent == name {
			return true
		}
	}
	return false
}
