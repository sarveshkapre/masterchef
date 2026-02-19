package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type FlakePolicy struct {
	AutoQuarantine              bool      `json:"auto_quarantine"`
	MinSamples                  int       `json:"min_samples"`
	FlakeRateThreshold          float64   `json:"flake_rate_threshold"`
	ConsecutiveFailureThreshold int       `json:"consecutive_failure_threshold"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type FlakeObservation struct {
	Suite  string `json:"suite"`
	Test   string `json:"test"`
	Status string `json:"status"` // pass|fail|flaky
}

type FlakeCase struct {
	ID                  string    `json:"id"`
	Suite               string    `json:"suite"`
	Test                string    `json:"test"`
	Passes              int       `json:"passes"`
	Failures            int       `json:"failures"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	FlakeRate           float64   `json:"flake_rate"`
	Quarantined         bool      `json:"quarantined"`
	QuarantineReason    string    `json:"quarantine_reason,omitempty"`
	QuarantinedAt       time.Time `json:"quarantined_at,omitempty"`
	LastObservedStatus  string    `json:"last_observed_status,omitempty"`
	LastSeenAt          time.Time `json:"last_seen_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type FlakeObservationResult struct {
	Case   FlakeCase `json:"case"`
	Action string    `json:"action"` // observed|auto-quarantined
}

type FlakeSummary struct {
	Total           int `json:"total"`
	Quarantined     int `json:"quarantined"`
	AutoQuarantined int `json:"auto_quarantined"`
}

type FlakeQuarantineStore struct {
	mu              sync.RWMutex
	nextID          int64
	policy          FlakePolicy
	cases           map[string]*FlakeCase
	byQualifiedName map[string]string
	autoQuarantined int
}

func NewFlakeQuarantineStore() *FlakeQuarantineStore {
	return &FlakeQuarantineStore{
		policy: FlakePolicy{
			AutoQuarantine:              true,
			MinSamples:                  10,
			FlakeRateThreshold:          0.05,
			ConsecutiveFailureThreshold: 3,
			UpdatedAt:                   time.Now().UTC(),
		},
		cases:           map[string]*FlakeCase{},
		byQualifiedName: map[string]string{},
	}
}

func (s *FlakeQuarantineStore) Policy() FlakePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *FlakeQuarantineStore) SetPolicy(in FlakePolicy) (FlakePolicy, error) {
	if in.MinSamples <= 0 {
		in.MinSamples = 10
	}
	if in.FlakeRateThreshold < 0 || in.FlakeRateThreshold > 1 {
		return FlakePolicy{}, errors.New("flake_rate_threshold must be between 0 and 1")
	}
	if in.ConsecutiveFailureThreshold <= 0 {
		in.ConsecutiveFailureThreshold = 3
	}
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = in
	s.mu.Unlock()
	return in, nil
}

func (s *FlakeQuarantineStore) Observe(in FlakeObservation) (FlakeObservationResult, error) {
	suite := strings.TrimSpace(in.Suite)
	testName := strings.TrimSpace(in.Test)
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if suite == "" || testName == "" {
		return FlakeObservationResult{}, errors.New("suite and test are required")
	}
	switch status {
	case "pass", "fail", "flaky":
	default:
		return FlakeObservationResult{}, errors.New("status must be pass, fail, or flaky")
	}

	key := strings.ToLower(suite + "::" + testName)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byQualifiedName[key]
	var item *FlakeCase
	if !ok {
		s.nextID++
		id = "flake-" + itoa(s.nextID)
		item = &FlakeCase{
			ID:        id,
			Suite:     suite,
			Test:      testName,
			UpdatedAt: now,
		}
		s.cases[id] = item
		s.byQualifiedName[key] = id
	} else {
		item = s.cases[id]
	}

	switch status {
	case "pass":
		item.Passes++
		item.ConsecutiveFailures = 0
	case "fail":
		item.Failures++
		item.ConsecutiveFailures++
	case "flaky":
		item.Failures++
		item.ConsecutiveFailures++
	}
	total := item.Passes + item.Failures
	if total > 0 {
		item.FlakeRate = float64(item.Failures) / float64(total)
	}
	item.LastObservedStatus = status
	item.LastSeenAt = now
	item.UpdatedAt = now

	action := "observed"
	policy := s.policy
	if policy.AutoQuarantine && !item.Quarantined && total >= policy.MinSamples &&
		(item.FlakeRate >= policy.FlakeRateThreshold || item.ConsecutiveFailures >= policy.ConsecutiveFailureThreshold) {
		item.Quarantined = true
		item.QuarantineReason = "auto quarantine by flake policy"
		item.QuarantinedAt = now
		s.autoQuarantined++
		action = "auto-quarantined"
	}
	return FlakeObservationResult{Case: cloneFlakeCase(*item), Action: action}, nil
}

func (s *FlakeQuarantineStore) List(filter string) []FlakeCase {
	filter = strings.ToLower(strings.TrimSpace(filter))
	s.mu.RLock()
	out := make([]FlakeCase, 0, len(s.cases))
	for _, item := range s.cases {
		switch filter {
		case "quarantined":
			if !item.Quarantined {
				continue
			}
		case "active":
			if item.Quarantined {
				continue
			}
		}
		out = append(out, cloneFlakeCase(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Quarantined != out[j].Quarantined {
			return out[i].Quarantined
		}
		if out[i].FlakeRate != out[j].FlakeRate {
			return out[i].FlakeRate > out[j].FlakeRate
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out
}

func (s *FlakeQuarantineStore) Get(id string) (FlakeCase, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FlakeCase{}, errors.New("flake case id is required")
	}
	s.mu.RLock()
	item, ok := s.cases[id]
	s.mu.RUnlock()
	if !ok {
		return FlakeCase{}, errors.New("flake case not found")
	}
	return cloneFlakeCase(*item), nil
}

func (s *FlakeQuarantineStore) Quarantine(id, reason string) (FlakeCase, error) {
	id = strings.TrimSpace(id)
	reason = strings.TrimSpace(reason)
	if id == "" {
		return FlakeCase{}, errors.New("flake case id is required")
	}
	if reason == "" {
		reason = "manual quarantine"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.cases[id]
	if !ok {
		return FlakeCase{}, errors.New("flake case not found")
	}
	item.Quarantined = true
	item.QuarantineReason = reason
	item.QuarantinedAt = time.Now().UTC()
	item.UpdatedAt = item.QuarantinedAt
	return cloneFlakeCase(*item), nil
}

func (s *FlakeQuarantineStore) Unquarantine(id, reason string) (FlakeCase, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FlakeCase{}, errors.New("flake case id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.cases[id]
	if !ok {
		return FlakeCase{}, errors.New("flake case not found")
	}
	item.Quarantined = false
	item.QuarantineReason = strings.TrimSpace(reason)
	item.QuarantinedAt = time.Time{}
	item.ConsecutiveFailures = 0
	item.UpdatedAt = time.Now().UTC()
	return cloneFlakeCase(*item), nil
}

func (s *FlakeQuarantineStore) Summary() FlakeSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := FlakeSummary{
		Total:           len(s.cases),
		AutoQuarantined: s.autoQuarantined,
	}
	for _, item := range s.cases {
		if item.Quarantined {
			out.Quarantined++
		}
	}
	return out
}

func cloneFlakeCase(in FlakeCase) FlakeCase {
	return in
}
