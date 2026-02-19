package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProviderConformanceSuite struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	Description      string    `json:"description,omitempty"`
	Checks           []string  `json:"checks"`
	RequiredPassRate float64   `json:"required_pass_rate"`
	LastRunID        string    `json:"last_run_id,omitempty"`
	LastStatus       string    `json:"last_status,omitempty"`
	LastRunAt        time.Time `json:"last_run_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ProviderConformanceRunInput struct {
	SuiteID         string `json:"suite_id"`
	ProviderVersion string `json:"provider_version,omitempty"`
	Trigger         string `json:"trigger,omitempty"`
}

type ProviderConformanceCheckResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // pass|fail
	LatencyMS int    `json:"latency_ms"`
	Detail    string `json:"detail,omitempty"`
}

type ProviderConformanceRun struct {
	ID              string                           `json:"id"`
	SuiteID         string                           `json:"suite_id"`
	Provider        string                           `json:"provider"`
	ProviderVersion string                           `json:"provider_version,omitempty"`
	Trigger         string                           `json:"trigger,omitempty"`
	Status          string                           `json:"status"` // pass|degraded|fail
	PassRate        float64                          `json:"pass_rate"`
	PassedChecks    int                              `json:"passed_checks"`
	FailedChecks    int                              `json:"failed_checks"`
	Checks          []ProviderConformanceCheckResult `json:"checks"`
	StartedAt       time.Time                        `json:"started_at"`
	CompletedAt     time.Time                        `json:"completed_at"`
}

type ProviderConformanceStore struct {
	mu      sync.RWMutex
	nextID  int64
	suites  map[string]*ProviderConformanceSuite
	runs    map[string]*ProviderConformanceRun
	runList []string
}

func NewProviderConformanceStore() *ProviderConformanceStore {
	s := &ProviderConformanceStore{
		suites:  map[string]*ProviderConformanceSuite{},
		runs:    map[string]*ProviderConformanceRun{},
		runList: make([]string, 0),
	}
	defaults := []ProviderConformanceSuite{
		{
			ID:               "provider-file-core",
			Provider:         "file",
			Description:      "Convergence and idempotency checks for file provider.",
			Checks:           []string{"idempotency", "drift-detection", "permission-reconcile", "backup-restore"},
			RequiredPassRate: 0.95,
		},
		{
			ID:               "provider-package-core",
			Provider:         "package",
			Description:      "Version pinning and rollback checks for package provider.",
			Checks:           []string{"install", "upgrade", "hold-unhold", "rollback"},
			RequiredPassRate: 0.95,
		},
		{
			ID:               "provider-service-core",
			Provider:         "service",
			Description:      "Service lifecycle and restart-notify checks.",
			Checks:           []string{"start-stop", "enable-disable", "notify-restart", "health-gate"},
			RequiredPassRate: 0.95,
		},
	}
	for _, item := range defaults {
		_, _ = s.UpsertSuite(item)
	}
	return s
}

func (s *ProviderConformanceStore) UpsertSuite(in ProviderConformanceSuite) (ProviderConformanceSuite, error) {
	id := strings.TrimSpace(in.ID)
	provider := strings.TrimSpace(in.Provider)
	if id == "" || provider == "" {
		return ProviderConformanceSuite{}, errors.New("id and provider are required")
	}
	if len(in.Checks) == 0 {
		return ProviderConformanceSuite{}, errors.New("checks are required")
	}
	if in.RequiredPassRate <= 0 || in.RequiredPassRate > 1 {
		return ProviderConformanceSuite{}, errors.New("required_pass_rate must be between 0 and 1")
	}
	cleanChecks := normalizeStringSlice(in.Checks)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.suites[id]
	if !ok {
		item := ProviderConformanceSuite{
			ID:               id,
			Provider:         provider,
			Description:      strings.TrimSpace(in.Description),
			Checks:           cleanChecks,
			RequiredPassRate: in.RequiredPassRate,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		s.suites[id] = &item
		return cloneProviderConformanceSuite(item), nil
	}
	existing.Provider = provider
	existing.Description = strings.TrimSpace(in.Description)
	existing.Checks = cleanChecks
	existing.RequiredPassRate = in.RequiredPassRate
	existing.UpdatedAt = now
	return cloneProviderConformanceSuite(*existing), nil
}

func (s *ProviderConformanceStore) ListSuites() []ProviderConformanceSuite {
	s.mu.RLock()
	out := make([]ProviderConformanceSuite, 0, len(s.suites))
	for _, item := range s.suites {
		out = append(out, cloneProviderConformanceSuite(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *ProviderConformanceStore) Run(in ProviderConformanceRunInput) (ProviderConformanceRun, error) {
	suiteID := strings.TrimSpace(in.SuiteID)
	if suiteID == "" {
		return ProviderConformanceRun{}, errors.New("suite_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	suite, ok := s.suites[suiteID]
	if !ok {
		return ProviderConformanceRun{}, errors.New("suite not found")
	}
	version := strings.TrimSpace(in.ProviderVersion)
	if version == "" {
		version = "latest"
	}
	trigger := strings.TrimSpace(in.Trigger)
	if trigger == "" {
		trigger = "manual"
	}
	started := time.Now().UTC()
	checks := make([]ProviderConformanceCheckResult, 0, len(suite.Checks))
	passed := 0
	failed := 0
	for _, check := range suite.Checks {
		score := deterministicProviderConformanceScore(suite.ID, version, trigger, check)
		status := "pass"
		detail := "check passed"
		if score >= 92 {
			status = "fail"
			detail = "deterministic conformance regression signal"
			failed++
		} else {
			passed++
		}
		checks = append(checks, ProviderConformanceCheckResult{
			Name:      check,
			Status:    status,
			LatencyMS: 20 + int(score%120),
			Detail:    detail,
		})
	}
	passRate := 0.0
	total := passed + failed
	if total > 0 {
		passRate = float64(passed) / float64(total)
	}
	status := "pass"
	if passRate < suite.RequiredPassRate {
		status = "fail"
	} else if failed > 0 {
		status = "degraded"
	}
	duration := 0
	for _, item := range checks {
		duration += item.LatencyMS
	}
	completed := started.Add(time.Duration(duration) * time.Millisecond)
	s.nextID++
	item := ProviderConformanceRun{
		ID:              "provider-conformance-run-" + itoa(s.nextID),
		SuiteID:         suite.ID,
		Provider:        suite.Provider,
		ProviderVersion: version,
		Trigger:         trigger,
		Status:          status,
		PassRate:        passRate,
		PassedChecks:    passed,
		FailedChecks:    failed,
		Checks:          checks,
		StartedAt:       started,
		CompletedAt:     completed,
	}
	s.runs[item.ID] = &item
	s.runList = append(s.runList, item.ID)
	suite.LastRunID = item.ID
	suite.LastStatus = item.Status
	suite.LastRunAt = item.CompletedAt
	suite.UpdatedAt = item.CompletedAt
	return cloneProviderConformanceRun(item), nil
}

func (s *ProviderConformanceStore) ListRuns(suiteID string, limit int) []ProviderConformanceRun {
	suiteID = strings.TrimSpace(suiteID)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]ProviderConformanceRun, 0, len(s.runList))
	for i := len(s.runList) - 1; i >= 0; i-- {
		runID := s.runList[i]
		item := s.runs[runID]
		if item == nil {
			continue
		}
		if suiteID != "" && !strings.EqualFold(item.SuiteID, suiteID) {
			continue
		}
		out = append(out, cloneProviderConformanceRun(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *ProviderConformanceStore) GetRun(id string) (ProviderConformanceRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ProviderConformanceRun{}, errors.New("run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return ProviderConformanceRun{}, errors.New("run not found")
	}
	return cloneProviderConformanceRun(*item), nil
}

func deterministicProviderConformanceScore(suiteID, version, trigger, check string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(suiteID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(version)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(trigger)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(check)))
	return h.Sum32() % 100
}

func cloneProviderConformanceSuite(in ProviderConformanceSuite) ProviderConformanceSuite {
	in.Checks = cloneStringSlice(in.Checks)
	return in
}

func cloneProviderConformanceRun(in ProviderConformanceRun) ProviderConformanceRun {
	in.Checks = cloneProviderConformanceCheckResults(in.Checks)
	return in
}

func cloneProviderConformanceCheckResults(in []ProviderConformanceCheckResult) []ProviderConformanceCheckResult {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProviderConformanceCheckResult, len(in))
	copy(out, in)
	return out
}
