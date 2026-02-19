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
	BaselineID  string `json:"baseline_id,omitempty"`
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
	BaselineID         string    `json:"baseline_id,omitempty"`
	RegressionDetected bool      `json:"regression_detected"`
	RegressionReasons  []string  `json:"regression_reasons,omitempty"`
	RegressionScore    float64   `json:"regression_score"`
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at"`
}

type ScenarioBaselineInput struct {
	Name  string `json:"name"`
	RunID string `json:"run_id"`
}

type ScenarioBaseline struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	ScenarioID         string    `json:"scenario_id"`
	ScenarioName       string    `json:"scenario_name"`
	ReferenceRunID     string    `json:"reference_run_id"`
	NodesTotal         int       `json:"nodes_total"`
	NodesFailed        int       `json:"nodes_failed"`
	DriftFindings      int       `json:"drift_findings"`
	MeanApplyLatencyMS int       `json:"mean_apply_latency_ms"`
	FailureRatio       float64   `json:"failure_ratio"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ScenarioRegressionReport struct {
	RunID              string   `json:"run_id"`
	BaselineID         string   `json:"baseline_id"`
	RegressionDetected bool     `json:"regression_detected"`
	Reasons            []string `json:"reasons,omitempty"`
	FailureDelta       float64  `json:"failure_delta"`
	DriftDelta         int      `json:"drift_delta"`
	LatencyDeltaMS     int      `json:"latency_delta_ms"`
	RegressionScore    float64  `json:"regression_score"`
}

type ScenarioTestStore struct {
	mu             sync.RWMutex
	nextRunID      int64
	nextBaselineID int64
	definitions    map[string]*ScenarioDefinition
	runs           map[string]*ScenarioRun
	baselines      map[string]*ScenarioBaseline
}

func NewScenarioTestStore() *ScenarioTestStore {
	store := &ScenarioTestStore{
		definitions: map[string]*ScenarioDefinition{},
		runs:        map[string]*ScenarioRun{},
		baselines:   map[string]*ScenarioBaseline{},
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
	baselineID := strings.TrimSpace(in.BaselineID)
	if baselineID != "" {
		baseline, ok := s.baselines[baselineID]
		if !ok {
			return ScenarioRun{}, errors.New("baseline not found")
		}
		if baseline.ScenarioID != scenarioID {
			return ScenarioRun{}, errors.New("baseline scenario mismatch")
		}
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
		BaselineID:         baselineID,
		StartedAt:          started,
		CompletedAt:        completed,
	}
	s.runs[item.ID] = &item
	if baselineID != "" {
		report, err := s.compareScenarioRunWithBaseline(item.ID, baselineID)
		if err != nil {
			return ScenarioRun{}, err
		}
		item.RegressionDetected = report.RegressionDetected
		item.RegressionReasons = cloneStringSlice(report.Reasons)
		item.RegressionScore = report.RegressionScore
		s.runs[item.ID] = &item
	}
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

func (s *ScenarioTestStore) CreateBaseline(in ScenarioBaselineInput) (ScenarioBaseline, error) {
	name := strings.TrimSpace(in.Name)
	runID := strings.TrimSpace(in.RunID)
	if name == "" || runID == "" {
		return ScenarioBaseline{}, errors.New("name and run_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return ScenarioBaseline{}, errors.New("reference run not found")
	}
	now := time.Now().UTC()
	s.nextBaselineID++
	item := ScenarioBaseline{
		ID:                 "scenario-baseline-" + itoa(s.nextBaselineID),
		Name:               name,
		ScenarioID:         run.ScenarioID,
		ScenarioName:       run.ScenarioName,
		ReferenceRunID:     run.ID,
		NodesTotal:         run.NodesTotal,
		NodesFailed:        run.NodesFailed,
		DriftFindings:      run.DriftFindings,
		MeanApplyLatencyMS: run.MeanApplyLatencyMS,
		FailureRatio:       failureRatio(*run),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.baselines[item.ID] = &item
	return cloneScenarioBaseline(item), nil
}

func (s *ScenarioTestStore) ListBaselines() []ScenarioBaseline {
	s.mu.RLock()
	out := make([]ScenarioBaseline, 0, len(s.baselines))
	for _, item := range s.baselines {
		out = append(out, cloneScenarioBaseline(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ScenarioTestStore) GetBaseline(id string) (ScenarioBaseline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScenarioBaseline{}, errors.New("baseline id is required")
	}
	s.mu.RLock()
	item, ok := s.baselines[id]
	s.mu.RUnlock()
	if !ok {
		return ScenarioBaseline{}, errors.New("baseline not found")
	}
	return cloneScenarioBaseline(*item), nil
}

func (s *ScenarioTestStore) CompareRunToBaseline(runID, baselineID string) (ScenarioRegressionReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compareScenarioRunWithBaseline(strings.TrimSpace(runID), strings.TrimSpace(baselineID))
}

func (s *ScenarioTestStore) compareScenarioRunWithBaseline(runID, baselineID string) (ScenarioRegressionReport, error) {
	if runID == "" || baselineID == "" {
		return ScenarioRegressionReport{}, errors.New("run_id and baseline_id are required")
	}
	run, ok := s.runs[runID]
	if !ok {
		return ScenarioRegressionReport{}, errors.New("scenario run not found")
	}
	baseline, ok := s.baselines[baselineID]
	if !ok {
		return ScenarioRegressionReport{}, errors.New("baseline not found")
	}
	if baseline.ScenarioID != run.ScenarioID {
		return ScenarioRegressionReport{}, errors.New("run and baseline scenario mismatch")
	}
	reasons := make([]string, 0)
	failureDelta := failureRatio(*run) - baseline.FailureRatio
	if failureDelta > 0.015 {
		reasons = append(reasons, "failure ratio regression over baseline")
	}
	driftDelta := run.DriftFindings - baseline.DriftFindings
	if driftDelta > maxScenarioInt(3, int(float64(maxScenarioInt(1, baseline.DriftFindings))*0.20)) {
		reasons = append(reasons, "drift findings exceeded baseline tolerance")
	}
	latencyDelta := run.MeanApplyLatencyMS - baseline.MeanApplyLatencyMS
	if latencyDelta > maxScenarioInt(40, int(float64(maxScenarioInt(1, baseline.MeanApplyLatencyMS))*0.25)) {
		reasons = append(reasons, "mean apply latency exceeded baseline tolerance")
	}
	score := 0.0
	if failureDelta > 0 {
		score += failureDelta * 100
	}
	if driftDelta > 0 {
		score += float64(driftDelta) * 0.75
	}
	if latencyDelta > 0 {
		score += float64(latencyDelta) / 20.0
	}
	report := ScenarioRegressionReport{
		RunID:              run.ID,
		BaselineID:         baseline.ID,
		RegressionDetected: len(reasons) > 0,
		Reasons:            reasons,
		FailureDelta:       failureDelta,
		DriftDelta:         driftDelta,
		LatencyDeltaMS:     latencyDelta,
		RegressionScore:    score,
	}
	run.BaselineID = baseline.ID
	run.RegressionDetected = report.RegressionDetected
	run.RegressionReasons = cloneStringSlice(report.Reasons)
	run.RegressionScore = report.RegressionScore
	s.runs[run.ID] = run
	return report, nil
}

func deterministicScenarioScore(scenarioID string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(scenarioID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func failureRatio(run ScenarioRun) float64 {
	if run.NodesTotal <= 0 {
		return 0
	}
	return float64(run.NodesFailed) / float64(run.NodesTotal)
}

func cloneScenarioDefinition(in ScenarioDefinition) ScenarioDefinition {
	return in
}

func cloneScenarioRun(in ScenarioRun) ScenarioRun {
	in.RegressionReasons = cloneStringSlice(in.RegressionReasons)
	return in
}

func cloneScenarioBaseline(in ScenarioBaseline) ScenarioBaseline {
	return in
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func maxScenarioInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
