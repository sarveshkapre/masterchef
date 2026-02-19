package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type EphemeralTestEnvironmentInput struct {
	Name       string `json:"name"`
	Profile    string `json:"profile"`
	NodeCount  int    `json:"node_count"`
	TTLMinutes int    `json:"ttl_minutes"`
	CreatedBy  string `json:"created_by,omitempty"`
}

type EphemeralTestEnvironment struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Profile     string    `json:"profile"`
	NodeCount   int       `json:"node_count"`
	TTLMinutes  int       `json:"ttl_minutes"`
	Status      string    `json:"status"` // active|expired|destroyed
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	DestroyedAt time.Time `json:"destroyed_at,omitempty"`
}

type IntegrationCheckInput struct {
	EnvironmentID string `json:"environment_id"`
	Suite         string `json:"suite"`
	Seed          int64  `json:"seed,omitempty"`
	TriggeredBy   string `json:"triggered_by,omitempty"`
}

type IntegrationCheckResult struct {
	ID             string    `json:"id"`
	EnvironmentID  string    `json:"environment_id"`
	Suite          string    `json:"suite"`
	Status         string    `json:"status"` // pass|degraded|fail
	PassedChecks   int       `json:"passed_checks"`
	FailedChecks   int       `json:"failed_checks"`
	DurationMS     int       `json:"duration_ms"`
	FailureSignals []string  `json:"failure_signals,omitempty"`
	TriggeredBy    string    `json:"triggered_by,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
}

type EphemeralEnvironmentStore struct {
	mu           sync.RWMutex
	nextEnvID    int64
	nextCheckID  int64
	environments map[string]*EphemeralTestEnvironment
	checks       map[string]*IntegrationCheckResult
	checkIndex   map[string][]string
}

func NewEphemeralEnvironmentStore() *EphemeralEnvironmentStore {
	return &EphemeralEnvironmentStore{
		environments: map[string]*EphemeralTestEnvironment{},
		checks:       map[string]*IntegrationCheckResult{},
		checkIndex:   map[string][]string{},
	}
}

func (s *EphemeralEnvironmentStore) CreateEnvironment(in EphemeralTestEnvironmentInput) (EphemeralTestEnvironment, error) {
	name := strings.TrimSpace(in.Name)
	profile := strings.TrimSpace(in.Profile)
	if name == "" || profile == "" {
		return EphemeralTestEnvironment{}, errors.New("name and profile are required")
	}
	if in.NodeCount <= 0 {
		return EphemeralTestEnvironment{}, errors.New("node_count must be greater than zero")
	}
	ttl := in.TTLMinutes
	if ttl <= 0 {
		ttl = 60
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEnvID++
	item := EphemeralTestEnvironment{
		ID:         "test-env-" + itoa(s.nextEnvID),
		Name:       name,
		Profile:    profile,
		NodeCount:  in.NodeCount,
		TTLMinutes: ttl,
		Status:     "active",
		CreatedBy:  strings.TrimSpace(in.CreatedBy),
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Duration(ttl) * time.Minute),
	}
	s.environments[item.ID] = &item
	return cloneEphemeralEnvironment(item), nil
}

func (s *EphemeralEnvironmentStore) ListEnvironments() []EphemeralTestEnvironment {
	now := time.Now().UTC()
	s.mu.Lock()
	out := make([]EphemeralTestEnvironment, 0, len(s.environments))
	for _, item := range s.environments {
		if item.Status == "active" && now.After(item.ExpiresAt) {
			item.Status = "expired"
		}
		out = append(out, cloneEphemeralEnvironment(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *EphemeralEnvironmentStore) GetEnvironment(id string) (EphemeralTestEnvironment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return EphemeralTestEnvironment{}, errors.New("environment id is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.environments[id]
	if !ok {
		return EphemeralTestEnvironment{}, errors.New("environment not found")
	}
	if item.Status == "active" && now.After(item.ExpiresAt) {
		item.Status = "expired"
	}
	return cloneEphemeralEnvironment(*item), nil
}

func (s *EphemeralEnvironmentStore) DestroyEnvironment(id string) (EphemeralTestEnvironment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return EphemeralTestEnvironment{}, errors.New("environment id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.environments[id]
	if !ok {
		return EphemeralTestEnvironment{}, errors.New("environment not found")
	}
	item.Status = "destroyed"
	item.DestroyedAt = time.Now().UTC()
	return cloneEphemeralEnvironment(*item), nil
}

func (s *EphemeralEnvironmentStore) RunIntegrationCheck(in IntegrationCheckInput) (IntegrationCheckResult, error) {
	envID := strings.TrimSpace(in.EnvironmentID)
	suite := strings.TrimSpace(in.Suite)
	if envID == "" || suite == "" {
		return IntegrationCheckResult{}, errors.New("environment_id and suite are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	env, ok := s.environments[envID]
	if !ok {
		return IntegrationCheckResult{}, errors.New("environment not found")
	}
	now := time.Now().UTC()
	if env.Status == "active" && now.After(env.ExpiresAt) {
		env.Status = "expired"
	}
	if env.Status != "active" {
		return IntegrationCheckResult{}, errors.New("integration checks require active environment")
	}
	seed := in.Seed
	if seed == 0 {
		seed = now.UnixNano()
	}
	score := deterministicIntegrationScore(env.ID, suite, seed)
	failed := int(score % 4)
	passed := 8 - failed
	if passed < 0 {
		passed = 0
	}
	status := "pass"
	if failed > 0 {
		status = "degraded"
	}
	if failed >= 3 {
		status = "fail"
	}
	signals := make([]string, 0)
	if failed > 0 {
		signals = append(signals, "non-idempotent drift detected in one or more checks")
	}
	if failed >= 2 {
		signals = append(signals, "apply latency above expected envelope")
	}
	if failed >= 3 {
		signals = append(signals, "post-change verification gate failed")
	}
	duration := 200 + int(score%700) + env.NodeCount*2
	completed := now.Add(time.Duration(duration) * time.Millisecond)
	s.nextCheckID++
	item := IntegrationCheckResult{
		ID:             "integration-check-" + itoa(s.nextCheckID),
		EnvironmentID:  env.ID,
		Suite:          suite,
		Status:         status,
		PassedChecks:   passed,
		FailedChecks:   failed,
		DurationMS:     duration,
		FailureSignals: signals,
		TriggeredBy:    strings.TrimSpace(in.TriggeredBy),
		StartedAt:      now,
		CompletedAt:    completed,
	}
	s.checks[item.ID] = &item
	s.checkIndex[env.ID] = append(s.checkIndex[env.ID], item.ID)
	return cloneIntegrationCheck(item), nil
}

func (s *EphemeralEnvironmentStore) ListChecks(environmentID string, limit int) []IntegrationCheckResult {
	environmentID = strings.TrimSpace(environmentID)
	if limit <= 0 {
		limit = 50
	}
	s.mu.RLock()
	ids := cloneStringSlice(s.checkIndex[environmentID])
	out := make([]IntegrationCheckResult, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		item := s.checks[ids[i]]
		if item == nil {
			continue
		}
		out = append(out, cloneIntegrationCheck(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func deterministicIntegrationScore(environmentID, suite string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(environmentID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(suite)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func cloneEphemeralEnvironment(in EphemeralTestEnvironment) EphemeralTestEnvironment {
	return in
}

func cloneIntegrationCheck(in IntegrationCheckResult) IntegrationCheckResult {
	in.FailureSignals = cloneStringSlice(in.FailureSignals)
	return in
}
