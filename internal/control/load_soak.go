package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type LoadSoakSuite struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	TargetComponent      string    `json:"target_component"`
	Mode                 string    `json:"mode"` // load|soak
	DurationMinutes      int       `json:"duration_minutes"`
	Concurrency          int       `json:"concurrency"`
	TargetThroughputRPS  float64   `json:"target_throughput_rps"`
	ExpectedP95LatencyMS int64     `json:"expected_p95_latency_ms"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type LoadSoakRunInput struct {
	SuiteID     string `json:"suite_id"`
	Seed        int64  `json:"seed,omitempty"`
	TriggeredBy string `json:"triggered_by,omitempty"`
}

type LoadSoakRun struct {
	ID               string    `json:"id"`
	SuiteID          string    `json:"suite_id"`
	SuiteName        string    `json:"suite_name"`
	TargetComponent  string    `json:"target_component"`
	Mode             string    `json:"mode"`
	Status           string    `json:"status"` // pass|degraded|fail
	Concurrency      int       `json:"concurrency"`
	DurationMinutes  int       `json:"duration_minutes"`
	ThroughputRPS    float64   `json:"throughput_rps"`
	P95LatencyMS     int64     `json:"p95_latency_ms"`
	ErrorRatePercent float64   `json:"error_rate_percent"`
	Findings         []string  `json:"findings,omitempty"`
	TriggeredBy      string    `json:"triggered_by,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
}

type LoadSoakStore struct {
	mu      sync.RWMutex
	nextID  int64
	suites  map[string]*LoadSoakSuite
	runs    map[string]*LoadSoakRun
	runList []string
}

func NewLoadSoakStore() *LoadSoakStore {
	s := &LoadSoakStore{
		suites:  map[string]*LoadSoakSuite{},
		runs:    map[string]*LoadSoakRun{},
		runList: make([]string, 0),
	}
	defaults := []LoadSoakSuite{
		{
			ID:                   "load-control-plane",
			Name:                 "Control Plane Load Profile",
			TargetComponent:      "control-plane",
			Mode:                 "load",
			DurationMinutes:      30,
			Concurrency:          200,
			TargetThroughputRPS:  350,
			ExpectedP95LatencyMS: 1200,
		},
		{
			ID:                   "soak-scheduler",
			Name:                 "Scheduler Soak Profile",
			TargetComponent:      "scheduler",
			Mode:                 "soak",
			DurationMinutes:      240,
			Concurrency:          150,
			TargetThroughputRPS:  220,
			ExpectedP95LatencyMS: 1500,
		},
		{
			ID:                   "load-execution-workers",
			Name:                 "Execution Worker Load Profile",
			TargetComponent:      "execution-workers",
			Mode:                 "load",
			DurationMinutes:      45,
			Concurrency:          300,
			TargetThroughputRPS:  500,
			ExpectedP95LatencyMS: 1000,
		},
	}
	for _, item := range defaults {
		_, _ = s.UpsertSuite(item)
	}
	return s
}

func (s *LoadSoakStore) UpsertSuite(in LoadSoakSuite) (LoadSoakSuite, error) {
	id := strings.TrimSpace(in.ID)
	name := strings.TrimSpace(in.Name)
	target := strings.TrimSpace(in.TargetComponent)
	mode := normalizeLoadSoakMode(in.Mode)
	if id == "" || name == "" || target == "" || mode == "" {
		return LoadSoakSuite{}, errors.New("id, name, target_component, and mode are required")
	}
	if in.DurationMinutes <= 0 || in.Concurrency <= 0 || in.TargetThroughputRPS <= 0 || in.ExpectedP95LatencyMS <= 0 {
		return LoadSoakSuite{}, errors.New("duration_minutes, concurrency, target_throughput_rps, and expected_p95_latency_ms must be greater than zero")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.suites[id]
	if !ok {
		item := LoadSoakSuite{
			ID:                   id,
			Name:                 name,
			TargetComponent:      target,
			Mode:                 mode,
			DurationMinutes:      in.DurationMinutes,
			Concurrency:          in.Concurrency,
			TargetThroughputRPS:  in.TargetThroughputRPS,
			ExpectedP95LatencyMS: in.ExpectedP95LatencyMS,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		s.suites[id] = &item
		return cloneLoadSoakSuite(item), nil
	}
	existing.Name = name
	existing.TargetComponent = target
	existing.Mode = mode
	existing.DurationMinutes = in.DurationMinutes
	existing.Concurrency = in.Concurrency
	existing.TargetThroughputRPS = in.TargetThroughputRPS
	existing.ExpectedP95LatencyMS = in.ExpectedP95LatencyMS
	existing.UpdatedAt = now
	return cloneLoadSoakSuite(*existing), nil
}

func (s *LoadSoakStore) ListSuites() []LoadSoakSuite {
	s.mu.RLock()
	out := make([]LoadSoakSuite, 0, len(s.suites))
	for _, item := range s.suites {
		out = append(out, cloneLoadSoakSuite(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *LoadSoakStore) Run(in LoadSoakRunInput) (LoadSoakRun, error) {
	suiteID := strings.TrimSpace(in.SuiteID)
	if suiteID == "" {
		return LoadSoakRun{}, errors.New("suite_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	suite, ok := s.suites[suiteID]
	if !ok {
		return LoadSoakRun{}, errors.New("suite not found")
	}
	seed := in.Seed
	if seed == 0 {
		seed = time.Now().UTC().UnixNano()
	}
	score := deterministicLoadSoakScore(suite.ID, seed)
	target := suite.TargetThroughputRPS
	throughput := target * (0.78 + (float64(score%40) / 100.0))
	latency := int64(float64(suite.ExpectedP95LatencyMS) * (0.8 + (float64(score%55) / 100.0)))
	errorRate := 0.05 + (float64(score%35) / 20.0)
	findings := make([]string, 0)
	status := "pass"
	if throughput < target {
		findings = append(findings, "throughput below target profile")
	}
	if latency > suite.ExpectedP95LatencyMS {
		findings = append(findings, "p95 latency above expected threshold")
	}
	if errorRate > 1.0 {
		findings = append(findings, "error rate exceeded 1% threshold")
	}
	if len(findings) > 0 {
		status = "degraded"
	}
	if throughput < (target*0.80) || latency > (suite.ExpectedP95LatencyMS*130/100) || errorRate > 2.0 {
		status = "fail"
	}
	started := time.Now().UTC()
	completed := started.Add(time.Duration(suite.DurationMinutes) * time.Minute)
	s.nextID++
	item := LoadSoakRun{
		ID:               "load-soak-run-" + itoa(s.nextID),
		SuiteID:          suite.ID,
		SuiteName:        suite.Name,
		TargetComponent:  suite.TargetComponent,
		Mode:             suite.Mode,
		Status:           status,
		Concurrency:      suite.Concurrency,
		DurationMinutes:  suite.DurationMinutes,
		ThroughputRPS:    throughput,
		P95LatencyMS:     latency,
		ErrorRatePercent: errorRate,
		Findings:         findings,
		TriggeredBy:      strings.TrimSpace(in.TriggeredBy),
		StartedAt:        started,
		CompletedAt:      completed,
	}
	s.runs[item.ID] = &item
	s.runList = append(s.runList, item.ID)
	return cloneLoadSoakRun(item), nil
}

func (s *LoadSoakStore) ListRuns(suiteID string, limit int) []LoadSoakRun {
	suiteID = strings.TrimSpace(suiteID)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]LoadSoakRun, 0, len(s.runList))
	for i := len(s.runList) - 1; i >= 0; i-- {
		item := s.runs[s.runList[i]]
		if item == nil {
			continue
		}
		if suiteID != "" && !strings.EqualFold(item.SuiteID, suiteID) {
			continue
		}
		out = append(out, cloneLoadSoakRun(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *LoadSoakStore) GetRun(id string) (LoadSoakRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LoadSoakRun{}, errors.New("run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return LoadSoakRun{}, errors.New("run not found")
	}
	return cloneLoadSoakRun(*item), nil
}

func normalizeLoadSoakMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "load":
		return "load"
	case "soak":
		return "soak"
	default:
		return ""
	}
}

func deterministicLoadSoakScore(suiteID string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(suiteID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func cloneLoadSoakSuite(in LoadSoakSuite) LoadSoakSuite {
	return in
}

func cloneLoadSoakRun(in LoadSoakRun) LoadSoakRun {
	in.Findings = cloneStringSlice(in.Findings)
	return in
}
