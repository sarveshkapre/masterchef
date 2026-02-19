package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChaosExperimentInput struct {
	Name          string `json:"name"`
	Target        string `json:"target"`
	FaultType     string `json:"fault_type"`
	Intensity     int    `json:"intensity"` // 1-100
	DurationSec   int    `json:"duration_sec"`
	Async         bool   `json:"async"`
	TriggeredBy   string `json:"triggered_by,omitempty"`
	ApprovalToken string `json:"approval_token,omitempty"`
}

type ChaosExperiment struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Target                string    `json:"target"`
	FaultType             string    `json:"fault_type"`
	Intensity             int       `json:"intensity"`
	DurationSec           int       `json:"duration_sec"`
	Async                 bool      `json:"async"`
	Status                string    `json:"status"` // running|completed|aborted|blocked
	TriggeredBy           string    `json:"triggered_by,omitempty"`
	ImpactScore           int       `json:"impact_score"`
	ServiceDisruptions    int       `json:"service_disruptions"`
	AutoRollbackTriggered bool      `json:"auto_rollback_triggered"`
	Findings              []string  `json:"findings,omitempty"`
	StartedAt             time.Time `json:"started_at"`
	CompletedAt           time.Time `json:"completed_at,omitempty"`
}

type ChaosExperimentStore struct {
	mu      sync.RWMutex
	nextID  int64
	items   map[string]*ChaosExperiment
	ordered []string
}

func NewChaosExperimentStore() *ChaosExperimentStore {
	return &ChaosExperimentStore{
		items:   map[string]*ChaosExperiment{},
		ordered: make([]string, 0),
	}
}

func (s *ChaosExperimentStore) Create(in ChaosExperimentInput) (ChaosExperiment, error) {
	name := strings.TrimSpace(in.Name)
	target := strings.TrimSpace(in.Target)
	fault := normalizeChaosFaultType(in.FaultType)
	if name == "" || target == "" || fault == "" {
		return ChaosExperiment{}, errors.New("name, target, and valid fault_type are required")
	}
	if in.Intensity <= 0 || in.Intensity > 100 {
		return ChaosExperiment{}, errors.New("intensity must be between 1 and 100")
	}
	duration := in.DurationSec
	if duration <= 0 {
		duration = 60
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := ChaosExperiment{
		ID:          "chaos-exp-" + itoa(s.nextID),
		Name:        name,
		Target:      target,
		FaultType:   fault,
		Intensity:   in.Intensity,
		DurationSec: duration,
		Async:       in.Async,
		Status:      "running",
		TriggeredBy: strings.TrimSpace(in.TriggeredBy),
		StartedAt:   now,
	}
	if in.Intensity >= 85 && strings.Contains(strings.ToLower(target), "prod") && strings.TrimSpace(in.ApprovalToken) == "" {
		item.Status = "blocked"
		item.Findings = []string{"high-intensity production experiment requires approval_token"}
		item.CompletedAt = now
		s.items[item.ID] = &item
		s.ordered = append(s.ordered, item.ID)
		return cloneChaosExperiment(item), nil
	}
	if !in.Async {
		s.completeLocked(&item, now)
	}
	s.items[item.ID] = &item
	s.ordered = append(s.ordered, item.ID)
	return cloneChaosExperiment(item), nil
}

func (s *ChaosExperimentStore) completeLocked(item *ChaosExperiment, started time.Time) {
	score := deterministicChaosScore(item.Target, item.FaultType, item.Intensity, item.DurationSec)
	disruptions := int(score%5) + (item.Intensity / 30)
	findings := make([]string, 0)
	if disruptions > 0 {
		findings = append(findings, "service disruption observed in injected fault domain")
	}
	if disruptions >= 3 {
		findings = append(findings, "error budget burn accelerated during chaos window")
	}
	autoRollback := disruptions >= 4 || item.Intensity >= 90
	if autoRollback {
		findings = append(findings, "auto rollback guardrail triggered")
	}
	item.Status = "completed"
	item.ImpactScore = 20 + int(score%70)
	item.ServiceDisruptions = disruptions
	item.AutoRollbackTriggered = autoRollback
	item.Findings = findings
	item.CompletedAt = started.Add(time.Duration(item.DurationSec) * time.Second)
}

func (s *ChaosExperimentStore) List() []ChaosExperiment {
	s.mu.RLock()
	out := make([]ChaosExperiment, 0, len(s.ordered))
	for i := len(s.ordered) - 1; i >= 0; i-- {
		item := s.items[s.ordered[i]]
		if item == nil {
			continue
		}
		out = append(out, cloneChaosExperiment(*item))
	}
	s.mu.RUnlock()
	return out
}

func (s *ChaosExperimentStore) Get(id string) (ChaosExperiment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ChaosExperiment{}, errors.New("experiment id is required")
	}
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return ChaosExperiment{}, errors.New("experiment not found")
	}
	return cloneChaosExperiment(*item), nil
}

func (s *ChaosExperimentStore) Abort(id, reason string) (ChaosExperiment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ChaosExperiment{}, errors.New("experiment id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return ChaosExperiment{}, errors.New("experiment not found")
	}
	if item.Status != "running" {
		return ChaosExperiment{}, errors.New("only running experiments can be aborted")
	}
	item.Status = "aborted"
	if strings.TrimSpace(reason) == "" {
		reason = "aborted by operator"
	}
	item.Findings = append(item.Findings, strings.TrimSpace(reason))
	item.CompletedAt = time.Now().UTC()
	return cloneChaosExperiment(*item), nil
}

func (s *ChaosExperimentStore) Complete(id string) (ChaosExperiment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ChaosExperiment{}, errors.New("experiment id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return ChaosExperiment{}, errors.New("experiment not found")
	}
	if item.Status != "running" {
		return ChaosExperiment{}, errors.New("only running experiments can be completed")
	}
	s.completeLocked(item, item.StartedAt)
	return cloneChaosExperiment(*item), nil
}

func normalizeChaosFaultType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "network-latency", "packet-loss", "process-crash", "cpu-stress", "queue-delay":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func deterministicChaosScore(target, faultType string, intensity, durationSec int) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(target))))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(faultType))))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(int64(intensity))))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(int64(durationSec))))
	return h.Sum32()
}

func cloneChaosExperiment(in ChaosExperiment) ChaosExperiment {
	in.Findings = cloneStringSlice(in.Findings)
	return in
}

func sortChaosExperimentsByStart(items []ChaosExperiment) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})
}
