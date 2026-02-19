package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type ScenarioDefinition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	FleetSize   int       `json:"fleet_size"`
	Services    int       `json:"services"`
	FailureRate float64   `json:"failure_rate"`
	ChaosLevel  int       `json:"chaos_level"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ScenarioRunInput struct {
	ScenarioID  string `json:"scenario_id"`
	Seed        int64  `json:"seed,omitempty"`
	TriggeredBy string `json:"triggered_by,omitempty"`
}

type ScenarioRun struct {
	ID                 string    `json:"id"`
	ScenarioID         string    `json:"scenario_id"`
	ScenarioName       string    `json:"scenario_name"`
	Seed               int64     `json:"seed"`
	Status             string    `json:"status"` // passed|degraded|failed
	NodesTotal         int       `json:"nodes_total"`
	NodesSucceeded     int       `json:"nodes_succeeded"`
	NodesFailed        int       `json:"nodes_failed"`
	DriftFindings      int       `json:"drift_findings"`
	MeanApplyLatencyMS int       `json:"mean_apply_latency_ms"`
	DurationMS         int       `json:"duration_ms"`
	TriggeredBy        string    `json:"triggered_by,omitempty"`
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at"`
}

type ScenarioTestStore struct {
	mu          sync.RWMutex
	nextRunID   int64
	definitions map[string]*ScenarioDefinition
	runs        map[string]*ScenarioRun
}

func NewScenarioTestStore() *ScenarioTestStore {
	store := &ScenarioTestStore{
		definitions: map[string]*ScenarioDefinition{},
		runs:        map[string]*ScenarioRun{},
	}
	defaults := []ScenarioDefinition{
		{
			ID:          "release-canary-fleet",
			Name:        "Release Canary Fleet",
			Description: "Simulates progressive rollout and rollback decisions under moderate risk.",
			FleetSize:   500,
			Services:    18,
			FailureRate: 0.02,
			ChaosLevel:  20,
		},
		{
			ID:          "certificate-rotation-wave",
			Name:        "Certificate Rotation Wave",
			Description: "Simulates mass certificate rotation with dependent restarts.",
			FleetSize:   2000,
			Services:    35,
			FailureRate: 0.03,
			ChaosLevel:  35,
		},
		{
			ID:          "regional-failover-drill",
			Name:        "Regional Failover Drill",
			Description: "Simulates active-active failover under elevated disruption.",
			FleetSize:   3500,
			Services:    60,
			FailureRate: 0.04,
			ChaosLevel:  50,
		},
	}
	for _, item := range defaults {
		_, _ = store.UpsertScenario(item)
	}
	return store
}

func (s *ScenarioTestStore) UpsertScenario(in ScenarioDefinition) (ScenarioDefinition, error) {
	id := strings.TrimSpace(in.ID)
	name := strings.TrimSpace(in.Name)
	if id == "" || name == "" {
		return ScenarioDefinition{}, errors.New("id and name are required")
	}
	if in.FleetSize <= 0 {
		return ScenarioDefinition{}, errors.New("fleet_size must be greater than zero")
	}
	if in.Services <= 0 {
		return ScenarioDefinition{}, errors.New("services must be greater than zero")
	}
	if in.FailureRate < 0 || in.FailureRate > 1 {
		return ScenarioDefinition{}, errors.New("failure_rate must be between 0 and 1")
	}
	if in.ChaosLevel < 0 || in.ChaosLevel > 100 {
		return ScenarioDefinition{}, errors.New("chaos_level must be between 0 and 100")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.definitions[id]
	if !ok {
		in.CreatedAt = now
		item := in
		item.UpdatedAt = now
		item.Description = strings.TrimSpace(item.Description)
		s.definitions[id] = &item
		return cloneScenarioDefinition(item), nil
	}
	existing.Name = name
	existing.Description = strings.TrimSpace(in.Description)
	existing.FleetSize = in.FleetSize
	existing.Services = in.Services
	existing.FailureRate = in.FailureRate
	existing.ChaosLevel = in.ChaosLevel
	existing.UpdatedAt = now
	return cloneScenarioDefinition(*existing), nil
}

func (s *ScenarioTestStore) ListScenarios() []ScenarioDefinition {
	s.mu.RLock()
	out := make([]ScenarioDefinition, 0, len(s.definitions))
	for _, item := range s.definitions {
		out = append(out, cloneScenarioDefinition(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *ScenarioTestStore) Run(in ScenarioRunInput) (ScenarioRun, error) {
	scenarioID := strings.TrimSpace(in.ScenarioID)
	if scenarioID == "" {
		return ScenarioRun{}, errors.New("scenario_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scenario, ok := s.definitions[scenarioID]
	if !ok {
		return ScenarioRun{}, errors.New("scenario not found")
	}
	seed := in.Seed
	if seed == 0 {
		seed = time.Now().UTC().UnixNano()
	}
	score := deterministicScenarioScore(scenarioID, seed)
	baseFailure := scenario.FailureRate + (float64(scenario.ChaosLevel) / 1500.0)
	if baseFailure > 0.95 {
		baseFailure = 0.95
	}
	nodesFailed := int(float64(scenario.FleetSize) * baseFailure)
	nodesFailed += int(score % 7)
	if nodesFailed > scenario.FleetSize {
		nodesFailed = scenario.FleetSize
	}
	nodesSucceeded := scenario.FleetSize - nodesFailed
	drift := int((float64(scenario.Services)*baseFailure)*1.8) + int(score%4)
	latency := 150 + scenario.ChaosLevel*6 + int(score%120)
	duration := scenario.FleetSize/2 + scenario.Services*15 + scenario.ChaosLevel*25 + int(score%80)
	status := "passed"
	if nodesFailed > 0 {
		status = "degraded"
	}
	if float64(nodesFailed)/float64(scenario.FleetSize) > 0.08 {
		status = "failed"
	}

	started := time.Now().UTC()
	completed := started.Add(time.Duration(duration) * time.Millisecond)
	s.nextRunID++
	item := ScenarioRun{
		ID:                 "scenario-run-" + itoa(s.nextRunID),
		ScenarioID:         scenario.ID,
		ScenarioName:       scenario.Name,
		Seed:               seed,
		Status:             status,
		NodesTotal:         scenario.FleetSize,
		NodesSucceeded:     nodesSucceeded,
		NodesFailed:        nodesFailed,
		DriftFindings:      drift,
		MeanApplyLatencyMS: latency,
		DurationMS:         duration,
		TriggeredBy:        strings.TrimSpace(in.TriggeredBy),
		StartedAt:          started,
		CompletedAt:        completed,
	}
	s.runs[item.ID] = &item
	return cloneScenarioRun(item), nil
}

func (s *ScenarioTestStore) ListRuns() []ScenarioRun {
	s.mu.RLock()
	out := make([]ScenarioRun, 0, len(s.runs))
	for _, item := range s.runs {
		out = append(out, cloneScenarioRun(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

func (s *ScenarioTestStore) GetRun(id string) (ScenarioRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScenarioRun{}, errors.New("scenario run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return ScenarioRun{}, errors.New("scenario run not found")
	}
	return cloneScenarioRun(*item), nil
}

func deterministicScenarioScore(scenarioID string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(scenarioID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func cloneScenarioDefinition(in ScenarioDefinition) ScenarioDefinition {
	return in
}

func cloneScenarioRun(in ScenarioRun) ScenarioRun {
	return in
}
