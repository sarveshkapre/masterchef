package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type AgentCheckin struct {
	AgentID         string    `json:"agent_id"`
	IntervalSeconds int       `json:"interval_seconds"`
	MaxSplaySeconds int       `json:"max_splay_seconds"`
	AppliedSplaySec int       `json:"applied_splay_seconds"`
	LastCheckinAt   time.Time `json:"last_checkin_at"`
	NextCheckinAt   time.Time `json:"next_checkin_at"`
}

type AgentCheckinInput struct {
	AgentID         string `json:"agent_id"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	MaxSplaySeconds int    `json:"max_splay_seconds,omitempty"`
}

type AgentCheckinStore struct {
	mu    sync.RWMutex
	items map[string]*AgentCheckin
}

func NewAgentCheckinStore() *AgentCheckinStore {
	return &AgentCheckinStore{
		items: map[string]*AgentCheckin{},
	}
}

func (s *AgentCheckinStore) Checkin(in AgentCheckinInput) (AgentCheckin, error) {
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		return AgentCheckin{}, errors.New("agent_id is required")
	}
	interval := in.IntervalSeconds
	if interval <= 0 {
		interval = 300
	}
	maxSplay := in.MaxSplaySeconds
	if maxSplay < 0 {
		maxSplay = 0
	}
	if maxSplay > 3600 {
		maxSplay = 3600
	}
	splay := deterministicSplay(agentID, maxSplay)
	now := time.Now().UTC()
	item := &AgentCheckin{
		AgentID:         agentID,
		IntervalSeconds: interval,
		MaxSplaySeconds: maxSplay,
		AppliedSplaySec: splay,
		LastCheckinAt:   now,
		NextCheckinAt:   now.Add(time.Duration(interval+splay) * time.Second),
	}

	s.mu.Lock()
	s.items[agentID] = item
	s.mu.Unlock()
	return cloneAgentCheckin(*item), nil
}

func (s *AgentCheckinStore) List() []AgentCheckin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentCheckin, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneAgentCheckin(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}

func deterministicSplay(agentID string, maxSplay int) int {
	if maxSplay <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(agentID))))
	return int(h.Sum32() % uint32(maxSplay+1))
}

func cloneAgentCheckin(in AgentCheckin) AgentCheckin {
	return in
}
