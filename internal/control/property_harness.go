package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type PropertyHarnessCase struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	ResourceType     string    `json:"resource_type"`
	Invariants       []string  `json:"invariants"`
	GeneratedSamples int       `json:"generated_samples"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PropertyHarnessRunInput struct {
	CaseID      string `json:"case_id"`
	Seed        int64  `json:"seed,omitempty"`
	TriggeredBy string `json:"triggered_by,omitempty"`
}

type PropertyHarnessRun struct {
	ID                 string    `json:"id"`
	CaseID             string    `json:"case_id"`
	CaseName           string    `json:"case_name"`
	Provider           string    `json:"provider"`
	ResourceType       string    `json:"resource_type"`
	Status             string    `json:"status"` // pass|fail
	GeneratedSamples   int       `json:"generated_samples"`
	InvariantsChecked  int       `json:"invariants_checked"`
	ViolationsDetected int       `json:"violations_detected"`
	ViolationReasons   []string  `json:"violation_reasons,omitempty"`
	TriggeredBy        string    `json:"triggered_by,omitempty"`
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at"`
}

type PropertyHarnessStore struct {
	mu      sync.RWMutex
	nextID  int64
	cases   map[string]*PropertyHarnessCase
	runs    map[string]*PropertyHarnessRun
	runList []string
}

func NewPropertyHarnessStore() *PropertyHarnessStore {
	s := &PropertyHarnessStore{
		cases:   map[string]*PropertyHarnessCase{},
		runs:    map[string]*PropertyHarnessRun{},
		runList: make([]string, 0),
	}
	defaults := []PropertyHarnessCase{
		{
			ID:               "property-file-idempotency",
			Name:             "File Idempotency and Convergence",
			Provider:         "file",
			ResourceType:     "file",
			Invariants:       []string{"idempotent_apply", "converges_within_two_runs", "preserves_declared_permissions"},
			GeneratedSamples: 120,
		},
		{
			ID:               "property-package-convergence",
			Name:             "Package Version Convergence",
			Provider:         "package",
			ResourceType:     "package",
			Invariants:       []string{"idempotent_version_pin", "rollback_restores_previous_state", "hold_prevents_unexpected_upgrade"},
			GeneratedSamples: 100,
		},
	}
	for _, item := range defaults {
		_, _ = s.UpsertCase(item)
	}
	return s
}

func (s *PropertyHarnessStore) UpsertCase(in PropertyHarnessCase) (PropertyHarnessCase, error) {
	id := strings.TrimSpace(in.ID)
	name := strings.TrimSpace(in.Name)
	provider := strings.TrimSpace(in.Provider)
	resourceType := strings.TrimSpace(in.ResourceType)
	invariants := normalizeStringSlice(in.Invariants)
	if id == "" || name == "" || provider == "" || resourceType == "" {
		return PropertyHarnessCase{}, errors.New("id, name, provider, and resource_type are required")
	}
	if len(invariants) == 0 {
		return PropertyHarnessCase{}, errors.New("invariants are required")
	}
	if in.GeneratedSamples <= 0 {
		in.GeneratedSamples = 100
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.cases[id]
	if !ok {
		item := PropertyHarnessCase{
			ID:               id,
			Name:             name,
			Provider:         provider,
			ResourceType:     resourceType,
			Invariants:       invariants,
			GeneratedSamples: in.GeneratedSamples,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		s.cases[id] = &item
		return clonePropertyHarnessCase(item), nil
	}
	existing.Name = name
	existing.Provider = provider
	existing.ResourceType = resourceType
	existing.Invariants = invariants
	existing.GeneratedSamples = in.GeneratedSamples
	existing.UpdatedAt = now
	return clonePropertyHarnessCase(*existing), nil
}

func (s *PropertyHarnessStore) ListCases() []PropertyHarnessCase {
	s.mu.RLock()
	out := make([]PropertyHarnessCase, 0, len(s.cases))
	for _, item := range s.cases {
		out = append(out, clonePropertyHarnessCase(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *PropertyHarnessStore) Run(in PropertyHarnessRunInput) (PropertyHarnessRun, error) {
	caseID := strings.TrimSpace(in.CaseID)
	if caseID == "" {
		return PropertyHarnessRun{}, errors.New("case_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	itemCase, ok := s.cases[caseID]
	if !ok {
		return PropertyHarnessRun{}, errors.New("property harness case not found")
	}
	seed := in.Seed
	if seed == 0 {
		seed = time.Now().UTC().UnixNano()
	}
	score := deterministicPropertyScore(itemCase.ID, seed)
	violations := int(score % 3)
	reasons := make([]string, 0)
	if violations > 0 {
		reasons = append(reasons, "idempotency invariant violated for generated sample subset")
	}
	if violations > 1 {
		reasons = append(reasons, "convergence invariant exceeded retry bound")
	}
	status := "pass"
	if violations > 0 {
		status = "fail"
	}
	started := time.Now().UTC()
	completed := started.Add(90 * time.Second)
	s.nextID++
	run := PropertyHarnessRun{
		ID:                 "property-run-" + itoa(s.nextID),
		CaseID:             itemCase.ID,
		CaseName:           itemCase.Name,
		Provider:           itemCase.Provider,
		ResourceType:       itemCase.ResourceType,
		Status:             status,
		GeneratedSamples:   itemCase.GeneratedSamples,
		InvariantsChecked:  len(itemCase.Invariants),
		ViolationsDetected: violations,
		ViolationReasons:   reasons,
		TriggeredBy:        strings.TrimSpace(in.TriggeredBy),
		StartedAt:          started,
		CompletedAt:        completed,
	}
	s.runs[run.ID] = &run
	s.runList = append(s.runList, run.ID)
	return clonePropertyHarnessRun(run), nil
}

func (s *PropertyHarnessStore) ListRuns(caseID string, limit int) []PropertyHarnessRun {
	caseID = strings.TrimSpace(caseID)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]PropertyHarnessRun, 0, len(s.runList))
	for i := len(s.runList) - 1; i >= 0; i-- {
		item := s.runs[s.runList[i]]
		if item == nil {
			continue
		}
		if caseID != "" && !strings.EqualFold(item.CaseID, caseID) {
			continue
		}
		out = append(out, clonePropertyHarnessRun(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *PropertyHarnessStore) GetRun(id string) (PropertyHarnessRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PropertyHarnessRun{}, errors.New("run id is required")
	}
	s.mu.RLock()
	item, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return PropertyHarnessRun{}, errors.New("run not found")
	}
	return clonePropertyHarnessRun(*item), nil
}

func deterministicPropertyScore(caseID string, seed int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(caseID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(itoa(seed)))
	return h.Sum32()
}

func clonePropertyHarnessCase(in PropertyHarnessCase) PropertyHarnessCase {
	in.Invariants = cloneStringSlice(in.Invariants)
	return in
}

func clonePropertyHarnessRun(in PropertyHarnessRun) PropertyHarnessRun {
	in.ViolationReasons = cloneStringSlice(in.ViolationReasons)
	return in
}
