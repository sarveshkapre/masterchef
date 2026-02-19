package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ModulePolicyHarnessAssertionInput struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
}

type ModulePolicyHarnessCaseInput struct {
	ID         string                              `json:"id,omitempty"`
	Name       string                              `json:"name"`
	Kind       string                              `json:"kind"` // module|policy
	Assertions []ModulePolicyHarnessAssertionInput `json:"assertions"`
}

type ModulePolicyHarnessAssertion struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
}

type ModulePolicyHarnessCase struct {
	ID         string                         `json:"id"`
	Name       string                         `json:"name"`
	Kind       string                         `json:"kind"`
	Assertions []ModulePolicyHarnessAssertion `json:"assertions"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

type ModulePolicyHarnessRunInput struct {
	CaseID   string            `json:"case_id"`
	Observed map[string]string `json:"observed"`
}

type ModulePolicyHarnessCheck struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Observed string `json:"observed,omitempty"`
	Passed   bool   `json:"passed"`
}

type ModulePolicyHarnessRun struct {
	ID        string                     `json:"id"`
	CaseID    string                     `json:"case_id"`
	Kind      string                     `json:"kind"`
	Status    string                     `json:"status"`
	Passed    int                        `json:"passed"`
	Failed    int                        `json:"failed"`
	Checks    []ModulePolicyHarnessCheck `json:"checks"`
	CreatedAt time.Time                  `json:"created_at"`
}

type ModulePolicyHarnessStore struct {
	mu     sync.RWMutex
	nextID int64
	cases  map[string]*ModulePolicyHarnessCase
	runs   map[string]*ModulePolicyHarnessRun
}

func NewModulePolicyHarnessStore() *ModulePolicyHarnessStore {
	return &ModulePolicyHarnessStore{
		cases: map[string]*ModulePolicyHarnessCase{},
		runs:  map[string]*ModulePolicyHarnessRun{},
	}
}

func (s *ModulePolicyHarnessStore) UpsertCase(in ModulePolicyHarnessCaseInput) (ModulePolicyHarnessCase, error) {
	name := strings.TrimSpace(in.Name)
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if name == "" {
		return ModulePolicyHarnessCase{}, errors.New("name is required")
	}
	if kind != "module" && kind != "policy" {
		return ModulePolicyHarnessCase{}, errors.New("kind must be module or policy")
	}
	if len(in.Assertions) == 0 {
		return ModulePolicyHarnessCase{}, errors.New("at least one assertion is required")
	}
	assertions := make([]ModulePolicyHarnessAssertion, 0, len(in.Assertions))
	for _, a := range in.Assertions {
		field := strings.ToLower(strings.TrimSpace(a.Field))
		if field == "" {
			return ModulePolicyHarnessCase{}, errors.New("assertion field is required")
		}
		assertions = append(assertions, ModulePolicyHarnessAssertion{
			Field:    field,
			Expected: strings.TrimSpace(a.Expected),
		})
	}
	caseID := strings.ToLower(strings.TrimSpace(in.ID))
	if caseID == "" {
		caseID = strings.ToLower("harness-case-" + strings.ReplaceAll(kind+"-"+strings.ReplaceAll(name, " ", "-"), "--", "-"))
	}
	item := ModulePolicyHarnessCase{
		ID:         caseID,
		Name:       name,
		Kind:       kind,
		Assertions: assertions,
		UpdatedAt:  time.Now().UTC(),
	}
	s.mu.Lock()
	s.cases[caseID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *ModulePolicyHarnessStore) ListCases() []ModulePolicyHarnessCase {
	s.mu.RLock()
	out := make([]ModulePolicyHarnessCase, 0, len(s.cases))
	for _, item := range s.cases {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *ModulePolicyHarnessStore) Run(in ModulePolicyHarnessRunInput) (ModulePolicyHarnessRun, error) {
	caseID := strings.ToLower(strings.TrimSpace(in.CaseID))
	if caseID == "" {
		return ModulePolicyHarnessRun{}, errors.New("case_id is required")
	}

	s.mu.RLock()
	item, ok := s.cases[caseID]
	s.mu.RUnlock()
	if !ok {
		return ModulePolicyHarnessRun{}, errors.New("harness case not found")
	}
	observed := map[string]string{}
	for k, v := range in.Observed {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		observed[key] = strings.TrimSpace(v)
	}
	checks := make([]ModulePolicyHarnessCheck, 0, len(item.Assertions))
	passed := 0
	failed := 0
	for _, assertion := range item.Assertions {
		got := observed[assertion.Field]
		check := ModulePolicyHarnessCheck{
			Field:    assertion.Field,
			Expected: assertion.Expected,
			Observed: got,
			Passed:   strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(assertion.Expected)),
		}
		if check.Passed {
			passed++
		} else {
			failed++
		}
		checks = append(checks, check)
	}
	status := "passed"
	if failed > 0 {
		status = "failed"
	}
	run := ModulePolicyHarnessRun{
		CaseID:    caseID,
		Kind:      item.Kind,
		Status:    status,
		Passed:    passed,
		Failed:    failed,
		Checks:    checks,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextID++
	run.ID = "harness-run-" + itoa(s.nextID)
	s.runs[run.ID] = &run
	s.mu.Unlock()
	return run, nil
}

func (s *ModulePolicyHarnessStore) ListRuns(limit int) []ModulePolicyHarnessRun {
	if limit <= 0 {
		limit = 25
	}
	s.mu.RLock()
	out := make([]ModulePolicyHarnessRun, 0, len(s.runs))
	for _, item := range s.runs {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *ModulePolicyHarnessStore) GetRun(id string) (ModulePolicyHarnessRun, bool) {
	s.mu.RLock()
	item, ok := s.runs[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ModulePolicyHarnessRun{}, false
	}
	return *item, true
}
