package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type CanaryUpgradeRun struct {
	ID           string    `json:"id"`
	Component    string    `json:"component"`
	FromChannel  string    `json:"from_channel"`
	ToChannel    string    `json:"to_channel"`
	CanaryIDs    []string  `json:"canary_ids,omitempty"`
	AutoRollback bool      `json:"auto_rollback"`
	Status       string    `json:"status"`
	RolledBack   bool      `json:"rolled_back"`
	Reason       string    `json:"reason,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at"`
}

type CanaryUpgradeStore struct {
	mu   sync.RWMutex
	next int64
	runs map[string]*CanaryUpgradeRun
}

func NewCanaryUpgradeStore() *CanaryUpgradeStore {
	return &CanaryUpgradeStore{runs: map[string]*CanaryUpgradeRun{}}
}

func (s *CanaryUpgradeStore) Record(in CanaryUpgradeRun) (CanaryUpgradeRun, error) {
	component := strings.ToLower(strings.TrimSpace(in.Component))
	from := normalizeChannel(in.FromChannel)
	to := normalizeChannel(in.ToChannel)
	if component == "" || from == "" || to == "" {
		return CanaryUpgradeRun{}, errors.New("component, from_channel, and to_channel are required")
	}
	now := time.Now().UTC()
	item := CanaryUpgradeRun{
		Component:    component,
		FromChannel:  from,
		ToChannel:    to,
		CanaryIDs:    normalizeStringSlice(in.CanaryIDs),
		AutoRollback: in.AutoRollback,
		Status:       strings.TrimSpace(in.Status),
		RolledBack:   in.RolledBack,
		Reason:       strings.TrimSpace(in.Reason),
		StartedAt:    now,
		CompletedAt:  now,
	}
	if item.Status == "" {
		item.Status = "completed"
	}
	s.mu.Lock()
	s.next++
	item.ID = "canary-upgrade-" + itoa(s.next)
	s.runs[item.ID] = &item
	s.mu.Unlock()
	return cloneCanaryUpgradeRun(item), nil
}

func (s *CanaryUpgradeStore) List(limit int) []CanaryUpgradeRun {
	s.mu.RLock()
	out := make([]CanaryUpgradeRun, 0, len(s.runs))
	for _, item := range s.runs {
		out = append(out, cloneCanaryUpgradeRun(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *CanaryUpgradeStore) Get(id string) (CanaryUpgradeRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.runs[strings.TrimSpace(id)]
	if !ok {
		return CanaryUpgradeRun{}, false
	}
	return cloneCanaryUpgradeRun(*item), true
}

func cloneCanaryUpgradeRun(in CanaryUpgradeRun) CanaryUpgradeRun {
	out := in
	out.CanaryIDs = append([]string{}, in.CanaryIDs...)
	return out
}
