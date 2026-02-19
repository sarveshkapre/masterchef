package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type MutationPolicy struct {
	MinKillRate       float64   `json:"min_kill_rate"`
	MinMutantsCovered int       `json:"min_mutants_covered"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type MutationSuite struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	Name          string    `json:"name"`
	CriticalPaths []string  `json:"critical_paths"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MutationRunInput struct {
	SuiteID     string `json:"suite_id"`
	Seed        int64  `json:"seed,omitempty"`
	TriggeredBy string `json:"triggered_by,omitempty"`
}

type MutationRun struct {
	ID              string    `json:"id"`
	SuiteID         string    `json:"suite_id"`
	Provider        string    `json:"provider"`
	Status          string    `json:"status"` // pass|fail
	MutantsTotal    int       `json:"mutants_total"`
	MutantsKilled   int       `json:"mutants_killed"`
	MutantsSurvived int       `json:"mutants_survived"`
	KillRate        float64   `json:"kill_rate"`
	BlockReasons    []string  `json:"block_reasons,omitempty"`
	TriggeredBy     string    `json:"triggered_by,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
}

type MutationStore struct {
	mu      sync.RWMutex
	nextID  int64
	policy  MutationPolicy
	suites  map[string]*MutationSuite
	runs    map[string]*MutationRun
	runList []string
}

func NewMutationStore() *MutationStore {
	s := &MutationStore{
		policy: MutationPolicy{
			MinKillRate:       0.75,
			MinMutantsCovered: 80,
			UpdatedAt:         time.Now().UTC(),
		},
		suites:  map[string]*MutationSuite{},
		runs:    map[string]*MutationRun{},
		runList: make([]string, 0),
	}
	defaults := []MutationSuite{
		{
			ID:            "mutation-file-provider",
			Provider:      "file",
			Name:          "File Provider Critical Path Mutations",
			CriticalPaths: []string{"converge/write", "converge/permissions", "rollback/filebucket"},
		},
		{
			ID:            "mutation-package-provider",
			Provider:      "package",
			Name:          "Package Provider Critical Path Mutations",
			CriticalPaths: []string{"install/version-pin", "upgrade/rollback", "hold-unhold"},
		},
	}
	for _, item := range defaults {
		_, _ = s.UpsertSuite(item)
	}
	return s
}

func (s *MutationStore) Policy() MutationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *MutationStore) SetPolicy(in MutationPolicy) (MutationPolicy, error) {
	if in.MinKillRate <= 0 || in.MinKillRate > 1 {
		return MutationPolicy{}, errors.New("min_kill_rate must be between 0 and 1")
	}
	if in.MinMutantsCovered <= 0 {
		return MutationPolicy{}, errors.New("min_mutants_covered must be greater than zero")
	}
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = in
	s.mu.Unlock()
	return in, nil
}

func (s *MutationStore) UpsertSuite(in MutationSuite) (MutationSuite, error) {
	id := strings.TrimSpace(in.ID)
	provider := strings.TrimSpace(in.Provider)
	name := strings.TrimSpace(in.Name)
	paths := normalizeStringSlice(in.CriticalPaths)
	if id == "" || provider == "" || name == "" {
		return MutationSuite{}, errors.New("id, provider, and name are required")
	}
	if len(paths) == 0 {
		return MutationSuite{}, errors.New("critical_paths are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.suites[id]
	if !ok {
		item := MutationSuite{
			ID:            id,
			Provider:      provider,
			Name:          name,
			CriticalPaths: paths,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		s.suites[id] = &item
		return cloneMutationSuite(item), nil
	}
	existing.Provider = provider
	existing.Name = name
	existing.CriticalPaths = paths
	existing.UpdatedAt = now
	return cloneMutationSuite(*existing), nil
}

func (s *MutationStore) ListSuites() []MutationSuite {
	s.mu.RLock()
	out := make([]MutationSuite, 0, len(s.suites))
	for _, item := range s.suites {
		out = append(out, cloneMutationSuite(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *MutationStore) Run(in MutationRunInput) (MutationRun, error) {
	suiteID := strings.TrimSpace(in.SuiteID)
	if suiteID == "" {
		return MutationRun{}, errors.New("suite_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	suite, ok := s.suites[suiteID]
	if !ok {
		return MutationRun{}, errors.New("suite not found")
	}
	seed := in.Seed
	if seed == 0 {
		seed = time.Now().UTC().UnixNano()
	}
	score := deterministicMutationScore(suite.ID, seed)
	total := 100 + int(score%80)
	killed := int(float64(total) * (0.60 + float64(score%35)/100.0))
	if killed > total {
		killed = total
	}
	survived := total - killed
	killRate := 0.0
	if total > 0 {
		killRate = float64(killed) / float64(total)
	}
	policy := s.policy
	reasons := make([]string, 0)
	if killRate < policy.MinKillRate {
		reasons = append(reasons, "kill rate below minimum threshold")
	}
	if total < policy.MinMutantsCovered {
		reasons = append(reasons, "mutants covered below minimum threshold")
	}
	status := "pass"
	if len(reasons) > 0 {
		status = "fail"
	}
	started := time.Now().UTC()
	completed := started.Add(2 * time.Minute)
	s.nextID++
	item := MutationRun{
		ID:              "mutation-run-" + itoa(s.nextID),
		SuiteID:         suite.ID,
		Provider:        suite.Provider,
		Status:          status,
		MutantsTotal:    total,
		MutantsKilled:   killed,
		MutantsSurvived: survived,
		KillRate:        killRate,
		BlockReasons:    reasons,
		TriggeredBy:     strings.TrimSpace(in.TriggeredBy),
		StartedAt:       started,
		CompletedAt:     completed,
	}
	s.runs[item.ID] = &item
	s.runList = append(s.runList, item.ID)
	return cloneMutationRun(item), nil
}

func (s *MutationStore) ListRuns(suiteID string, limit int) []MutationRun {
	suiteID = strings.TrimSpace(suiteID)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]MutationRun, 0, len(s.runList))
	for i := len(s.runList) - 1; i >= 0; i-- {
		item := s.runs[s.runList[i]]
		if item == nil {
			continue
		}
		if suiteID != "" && !strings.EqualFold(item.SuiteID, suiteID) {
			continue
		}
		out = append(out, cloneMutationRun(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *MutationStore) GetRun(id string) (MutationRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MutationRun{}, errors.New("run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return MutationRun{}, errors.New("run not found")
	}
	return cloneMutationRun(*item), nil
}

func deterministicMutationScore(suiteID string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(suiteID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func cloneMutationSuite(in MutationSuite) MutationSuite {
	in.CriticalPaths = cloneStringSlice(in.CriticalPaths)
	return in
}

func cloneMutationRun(in MutationRun) MutationRun {
	in.BlockReasons = cloneStringSlice(in.BlockReasons)
	return in
}
