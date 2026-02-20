package control

import (
	"errors"
	"sync"
	"time"
)

type StuckRecoveryPolicy struct {
	Enabled         bool      `json:"enabled"`
	MaxAgeSeconds   int       `json:"max_age_seconds"`
	CooldownSeconds int       `json:"cooldown_seconds"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type StuckRecoveryStatus struct {
	Policy            StuckRecoveryPolicy `json:"policy"`
	LastCheckAt       time.Time           `json:"last_check_at,omitempty"`
	LastRecoveredAt   time.Time           `json:"last_recovered_at,omitempty"`
	LastRecovered     int                 `json:"last_recovered"`
	TotalRecovered    int                 `json:"total_recovered"`
	TotalChecks       int                 `json:"total_checks"`
	LastMode          string              `json:"last_mode,omitempty"` // auto|manual
	LastMaxAgeSeconds int                 `json:"last_max_age_seconds,omitempty"`
}

type StuckRecoveryStore struct {
	mu     sync.RWMutex
	policy StuckRecoveryPolicy
	status StuckRecoveryStatus
}

func NewStuckRecoveryStore() *StuckRecoveryStore {
	policy := StuckRecoveryPolicy{
		Enabled:         true,
		MaxAgeSeconds:   300,
		CooldownSeconds: 60,
		UpdatedAt:       time.Now().UTC(),
	}
	return &StuckRecoveryStore{
		policy: policy,
		status: StuckRecoveryStatus{
			Policy: policy,
		},
	}
}

func (s *StuckRecoveryStore) Policy() StuckRecoveryPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *StuckRecoveryStore) SetPolicy(in StuckRecoveryPolicy) (StuckRecoveryPolicy, error) {
	if in.MaxAgeSeconds <= 0 {
		return StuckRecoveryPolicy{}, errors.New("max_age_seconds must be > 0")
	}
	if in.CooldownSeconds < 0 {
		return StuckRecoveryPolicy{}, errors.New("cooldown_seconds must be >= 0")
	}
	item := StuckRecoveryPolicy{
		Enabled:         in.Enabled,
		MaxAgeSeconds:   in.MaxAgeSeconds,
		CooldownSeconds: in.CooldownSeconds,
		UpdatedAt:       time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = item
	s.status.Policy = item
	s.mu.Unlock()
	return item, nil
}

func (s *StuckRecoveryStore) Status() StuckRecoveryStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.status
	out.Policy = s.policy
	return out
}

func (s *StuckRecoveryStore) PrepareAutoRun(now time.Time) (StuckRecoveryPolicy, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.policy.Enabled {
		return s.policy, false
	}
	if s.policy.CooldownSeconds > 0 && !s.status.LastCheckAt.IsZero() {
		cooldown := time.Duration(s.policy.CooldownSeconds) * time.Second
		if now.Sub(s.status.LastCheckAt) < cooldown {
			return s.policy, false
		}
	}
	s.status.LastCheckAt = now
	s.status.TotalChecks++
	s.status.LastMode = "auto"
	s.status.LastMaxAgeSeconds = s.policy.MaxAgeSeconds
	return s.policy, true
}

func (s *StuckRecoveryStore) RecordAutoRunResult(now time.Time, recovered int) {
	s.mu.Lock()
	s.recordRunResultLocked(now, recovered)
	s.mu.Unlock()
}

func (s *StuckRecoveryStore) RecordManualRun(now time.Time, maxAgeSeconds int, recovered int) {
	s.mu.Lock()
	s.status.LastCheckAt = now
	s.status.TotalChecks++
	s.status.LastMode = "manual"
	if maxAgeSeconds <= 0 {
		maxAgeSeconds = s.policy.MaxAgeSeconds
	}
	s.status.LastMaxAgeSeconds = maxAgeSeconds
	s.recordRunResultLocked(now, recovered)
	s.mu.Unlock()
}

func (s *StuckRecoveryStore) recordRunResultLocked(now time.Time, recovered int) {
	if recovered < 0 {
		recovered = 0
	}
	s.status.LastRecovered = recovered
	if recovered > 0 {
		s.status.LastRecoveredAt = now
		s.status.TotalRecovered += recovered
	}
	s.status.Policy = s.policy
}
