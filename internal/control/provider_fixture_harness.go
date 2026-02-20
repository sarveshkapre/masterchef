package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProviderTestFixture struct {
	ID             string         `json:"id"`
	Provider       string         `json:"provider"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Inputs         map[string]any `json:"inputs,omitempty"`
	ExpectedChecks []string       `json:"expected_checks,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type ProviderHarnessRunInput struct {
	Provider        string   `json:"provider"`
	SuiteID         string   `json:"suite_id,omitempty"`
	FixtureIDs      []string `json:"fixture_ids,omitempty"`
	ProviderVersion string   `json:"provider_version,omitempty"`
	Trigger         string   `json:"trigger,omitempty"`
}

type ProviderHarnessFixtureResult struct {
	FixtureID string   `json:"fixture_id"`
	Name      string   `json:"name"`
	Status    string   `json:"status"` // pass|fail
	Reasons   []string `json:"reasons,omitempty"`
}

type ProviderHarnessRun struct {
	ID              string                         `json:"id"`
	Provider        string                         `json:"provider"`
	SuiteID         string                         `json:"suite_id"`
	ProviderVersion string                         `json:"provider_version,omitempty"`
	Trigger         string                         `json:"trigger,omitempty"`
	Status          string                         `json:"status"` // pass|degraded|fail
	PassedFixtures  int                            `json:"passed_fixtures"`
	FailedFixtures  int                            `json:"failed_fixtures"`
	FixtureResults  []ProviderHarnessFixtureResult `json:"fixture_results"`
	StartedAt       time.Time                      `json:"started_at"`
	CompletedAt     time.Time                      `json:"completed_at"`
}

type ProviderFixtureHarnessStore struct {
	mu       sync.RWMutex
	nextID   int64
	fNextID  int64
	limit    int
	fixtures map[string]*ProviderTestFixture
	runs     map[string]*ProviderHarnessRun
	runList  []string
}

func NewProviderFixtureHarnessStore(limit int) *ProviderFixtureHarnessStore {
	if limit <= 0 {
		limit = 3000
	}
	return &ProviderFixtureHarnessStore{
		limit:    limit,
		fixtures: map[string]*ProviderTestFixture{},
		runs:     map[string]*ProviderHarnessRun{},
		runList:  make([]string, 0, limit),
	}
}

func (s *ProviderFixtureHarnessStore) UpsertFixture(in ProviderTestFixture) (ProviderTestFixture, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	name := strings.TrimSpace(in.Name)
	if provider == "" || name == "" {
		return ProviderTestFixture{}, errors.New("provider and name are required")
	}
	id := strings.TrimSpace(in.ID)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		s.fNextID++
		id = "provider-fixture-" + itoa(s.fNextID)
	}
	item, ok := s.fixtures[id]
	if !ok {
		item = &ProviderTestFixture{
			ID:        id,
			CreatedAt: now,
		}
		s.fixtures[id] = item
	}
	item.Provider = provider
	item.Name = name
	item.Description = strings.TrimSpace(in.Description)
	item.Inputs = cloneFixtureAnyMap(in.Inputs)
	item.ExpectedChecks = normalizeStringSlice(in.ExpectedChecks)
	item.Tags = normalizeStringSlice(in.Tags)
	item.UpdatedAt = now
	return cloneProviderFixture(*item), nil
}

func (s *ProviderFixtureHarnessStore) ListFixtures(provider string, limit int) []ProviderTestFixture {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if limit <= 0 {
		limit = 200
	}
	s.mu.RLock()
	out := make([]ProviderTestFixture, 0, len(s.fixtures))
	for _, item := range s.fixtures {
		if provider != "" && item.Provider != provider {
			continue
		}
		out = append(out, cloneProviderFixture(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *ProviderFixtureHarnessStore) GetFixture(id string) (ProviderTestFixture, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ProviderTestFixture{}, errors.New("fixture id is required")
	}
	s.mu.RLock()
	item, ok := s.fixtures[id]
	s.mu.RUnlock()
	if !ok {
		return ProviderTestFixture{}, errors.New("fixture not found")
	}
	return cloneProviderFixture(*item), nil
}

func (s *ProviderFixtureHarnessStore) Run(input ProviderHarnessRunInput, suites *ProviderConformanceStore) (ProviderHarnessRun, error) {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider == "" {
		return ProviderHarnessRun{}, errors.New("provider is required")
	}
	if suites == nil {
		return ProviderHarnessRun{}, errors.New("provider conformance store is required")
	}
	suiteID := strings.TrimSpace(input.SuiteID)
	if suiteID == "" {
		for _, item := range suites.ListSuites() {
			if strings.EqualFold(item.Provider, provider) {
				suiteID = item.ID
				break
			}
		}
	}
	if suiteID == "" {
		return ProviderHarnessRun{}, errors.New("suite_id is required when no default suite exists for provider")
	}
	suite, err := suites.GetSuite(suiteID)
	if err != nil {
		return ProviderHarnessRun{}, err
	}
	if !strings.EqualFold(suite.Provider, provider) {
		return ProviderHarnessRun{}, errors.New("suite/provider mismatch for harness run")
	}

	fixtures := s.collectFixturesForRun(provider, input.FixtureIDs)
	if len(fixtures) == 0 {
		return ProviderHarnessRun{}, errors.New("no fixtures selected for harness run")
	}

	version := strings.TrimSpace(input.ProviderVersion)
	if version == "" {
		version = "latest"
	}
	trigger := strings.TrimSpace(input.Trigger)
	if trigger == "" {
		trigger = "manual"
	}

	started := time.Now().UTC()
	results := make([]ProviderHarnessFixtureResult, 0, len(fixtures))
	passed := 0
	failed := 0
	for _, fixture := range fixtures {
		status := "pass"
		reasons := make([]string, 0, 2)
		for _, expected := range fixture.ExpectedChecks {
			if !containsNormalizedString(suite.Checks, expected) {
				status = "fail"
				reasons = append(reasons, "expected check not covered by suite: "+expected)
			}
		}
		score := deterministicProviderHarnessScore(provider, suite.ID, version, trigger, fixture.ID)
		if score >= 90 {
			status = "fail"
			reasons = append(reasons, "deterministic harness regression signal")
		}
		if status == "pass" {
			passed++
		} else {
			failed++
		}
		results = append(results, ProviderHarnessFixtureResult{
			FixtureID: fixture.ID,
			Name:      fixture.Name,
			Status:    status,
			Reasons:   reasons,
		})
	}
	runStatus := "pass"
	if failed > 0 && passed > 0 {
		runStatus = "degraded"
	} else if failed > 0 {
		runStatus = "fail"
	}
	completed := started.Add(time.Duration(20+len(fixtures)*15) * time.Millisecond)

	s.mu.Lock()
	s.nextID++
	item := ProviderHarnessRun{
		ID:              "provider-harness-run-" + itoa(s.nextID),
		Provider:        provider,
		SuiteID:         suite.ID,
		ProviderVersion: version,
		Trigger:         trigger,
		Status:          runStatus,
		PassedFixtures:  passed,
		FailedFixtures:  failed,
		FixtureResults:  results,
		StartedAt:       started,
		CompletedAt:     completed,
	}
	s.runs[item.ID] = &item
	s.runList = append(s.runList, item.ID)
	if len(s.runList) > s.limit {
		cut := len(s.runList) - s.limit
		for i := 0; i < cut; i++ {
			delete(s.runs, s.runList[i])
		}
		s.runList = append([]string{}, s.runList[cut:]...)
	}
	s.mu.Unlock()
	return cloneProviderHarnessRun(item), nil
}

func (s *ProviderFixtureHarnessStore) ListRuns(provider string, limit int) []ProviderHarnessRun {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]ProviderHarnessRun, 0, len(s.runList))
	for i := len(s.runList) - 1; i >= 0; i-- {
		item := s.runs[s.runList[i]]
		if item == nil {
			continue
		}
		if provider != "" && item.Provider != provider {
			continue
		}
		out = append(out, cloneProviderHarnessRun(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *ProviderFixtureHarnessStore) GetRun(id string) (ProviderHarnessRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ProviderHarnessRun{}, errors.New("run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return ProviderHarnessRun{}, errors.New("harness run not found")
	}
	return cloneProviderHarnessRun(*item), nil
}

func (s *ProviderFixtureHarnessStore) collectFixturesForRun(provider string, fixtureIDs []string) []ProviderTestFixture {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(fixtureIDs) == 0 {
		out := make([]ProviderTestFixture, 0, len(s.fixtures))
		for _, item := range s.fixtures {
			if item.Provider != provider {
				continue
			}
			out = append(out, cloneProviderFixture(*item))
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	out := make([]ProviderTestFixture, 0, len(fixtureIDs))
	for _, raw := range fixtureIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		item, ok := s.fixtures[id]
		if !ok || item.Provider != provider {
			continue
		}
		out = append(out, cloneProviderFixture(*item))
	}
	return out
}

func deterministicProviderHarnessScore(provider, suiteID, version, trigger, fixtureID string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(provider)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(suiteID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(version)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(trigger)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(fixtureID)))
	return h.Sum32() % 100
}

func cloneProviderFixture(in ProviderTestFixture) ProviderTestFixture {
	out := in
	out.Inputs = cloneFixtureAnyMap(in.Inputs)
	out.ExpectedChecks = cloneStringSlice(in.ExpectedChecks)
	out.Tags = cloneStringSlice(in.Tags)
	return out
}

func cloneProviderHarnessRun(in ProviderHarnessRun) ProviderHarnessRun {
	out := in
	out.FixtureResults = make([]ProviderHarnessFixtureResult, len(in.FixtureResults))
	copy(out.FixtureResults, in.FixtureResults)
	for i := range out.FixtureResults {
		out.FixtureResults[i].Reasons = cloneStringSlice(in.FixtureResults[i].Reasons)
	}
	return out
}

func cloneFixtureAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func containsNormalizedString(items []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}
