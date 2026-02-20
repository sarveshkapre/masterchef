package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChecklistStatus string

const (
	ChecklistOpen      ChecklistStatus = "open"
	ChecklistCompleted ChecklistStatus = "completed"
)

type ChecklistItem struct {
	ID          string    `json:"id"`
	Phase       string    `json:"phase"` // pre|post
	Prompt      string    `json:"prompt"`
	Required    bool      `json:"required"`
	Completed   bool      `json:"completed"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}

type ChecklistRun struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	RiskLevel string          `json:"risk_level"` // low|medium|high
	Context   map[string]any  `json:"context,omitempty"`
	Status    ChecklistStatus `json:"status"`
	Items     []ChecklistItem `json:"items"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ChecklistGateResult struct {
	ChecklistID       string          `json:"checklist_id"`
	Phase             string          `json:"phase"` // pre|post
	Allowed           bool            `json:"allowed"`
	Blockers          []ChecklistItem `json:"blockers,omitempty"`
	CheckedRequired   int             `json:"checked_required"`
	CompletedRequired int             `json:"completed_required"`
	EvaluatedAt       time.Time       `json:"evaluated_at"`
}

type ChecklistStore struct {
	mu         sync.RWMutex
	nextID     int64
	checklists map[string]*ChecklistRun
}

func NewChecklistStore() *ChecklistStore {
	return &ChecklistStore{checklists: map[string]*ChecklistRun{}}
}

func (s *ChecklistStore) Create(name, riskLevel string, context map[string]any) (ChecklistRun, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ChecklistRun{}, errors.New("checklist name is required")
	}
	risk := normalizeRiskLevel(riskLevel)
	items := defaultChecklistItems(risk)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := time.Now().UTC()
	id := "checklist-" + itoa(s.nextID)
	run := ChecklistRun{
		ID:        id,
		Name:      name,
		RiskLevel: risk,
		Context:   copyContext(context),
		Status:    ChecklistOpen,
		Items:     items,
		CreatedAt: now,
		UpdatedAt: now,
	}
	cp := cloneChecklist(run)
	s.checklists[id] = &cp
	return cp, nil
}

func (s *ChecklistStore) List() []ChecklistRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ChecklistRun, 0, len(s.checklists))
	for _, item := range s.checklists {
		out = append(out, cloneChecklist(*item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ChecklistStore) Get(id string) (ChecklistRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.checklists[strings.TrimSpace(id)]
	if !ok {
		return ChecklistRun{}, errors.New("checklist not found")
	}
	return cloneChecklist(*item), nil
}

func (s *ChecklistStore) CompleteItem(checklistID, itemID, notes string) (ChecklistRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.checklists[strings.TrimSpace(checklistID)]
	if !ok {
		return ChecklistRun{}, errors.New("checklist not found")
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return ChecklistRun{}, errors.New("item_id is required")
	}
	found := false
	now := time.Now().UTC()
	for i := range item.Items {
		if item.Items[i].ID != itemID {
			continue
		}
		found = true
		item.Items[i].Completed = true
		item.Items[i].CompletedAt = now
		item.Items[i].Notes = strings.TrimSpace(notes)
		break
	}
	if !found {
		return ChecklistRun{}, errors.New("checklist item not found")
	}
	item.UpdatedAt = now
	item.Status = ChecklistOpen
	allRequiredDone := true
	for _, c := range item.Items {
		if c.Required && !c.Completed {
			allRequiredDone = false
			break
		}
	}
	if allRequiredDone {
		item.Status = ChecklistCompleted
	}
	return cloneChecklist(*item), nil
}

func (s *ChecklistStore) EvaluateGate(checklistID, phase string) (ChecklistGateResult, error) {
	s.mu.RLock()
	item, ok := s.checklists[strings.TrimSpace(checklistID)]
	if !ok {
		s.mu.RUnlock()
		return ChecklistGateResult{}, errors.New("checklist not found")
	}
	run := cloneChecklist(*item)
	s.mu.RUnlock()

	normalizedPhase := normalizeChecklistPhase(phase)
	if normalizedPhase == "" {
		return ChecklistGateResult{}, errors.New("phase must be pre or post")
	}

	blockers := make([]ChecklistItem, 0)
	checkedRequired := 0
	completedRequired := 0
	for _, c := range run.Items {
		if c.Phase != normalizedPhase || !c.Required {
			continue
		}
		checkedRequired++
		if c.Completed {
			completedRequired++
			continue
		}
		blockers = append(blockers, c)
	}

	return ChecklistGateResult{
		ChecklistID:       run.ID,
		Phase:             normalizedPhase,
		Allowed:           len(blockers) == 0,
		Blockers:          blockers,
		CheckedRequired:   checkedRequired,
		CompletedRequired: completedRequired,
		EvaluatedAt:       time.Now().UTC(),
	}, nil
}

func defaultChecklistItems(risk string) []ChecklistItem {
	base := []ChecklistItem{
		{
			ID:       "pre-change-review",
			Phase:    "pre",
			Prompt:   "Confirm change scope, blast radius, and rollback path are reviewed.",
			Required: true,
		},
		{
			ID:       "pre-health-baseline",
			Phase:    "pre",
			Prompt:   "Capture baseline health metrics and canary status before execution.",
			Required: true,
		},
		{
			ID:       "post-verification",
			Phase:    "post",
			Prompt:   "Validate service health, error rates, and run outcome after execution.",
			Required: true,
		},
		{
			ID:       "post-handoff",
			Phase:    "post",
			Prompt:   "Update handoff notes with risks, blockers, and follow-up actions.",
			Required: true,
		},
	}
	if risk == "high" {
		base = append(base,
			ChecklistItem{
				ID:       "pre-approval-quorum",
				Phase:    "pre",
				Prompt:   "Confirm high-risk approval quorum and maintenance window are active.",
				Required: true,
			},
			ChecklistItem{
				ID:       "post-rollback-readiness",
				Phase:    "post",
				Prompt:   "Confirm rollback command path remains available until stability window closes.",
				Required: true,
			},
		)
	}
	for i := range base {
		base[i].CompletedAt = time.Time{}
		base[i].Notes = ""
		if base[i].ID == "" {
			base[i].ID = "item-" + itoa(int64(i+1))
		}
	}
	return base
}

func cloneChecklist(in ChecklistRun) ChecklistRun {
	out := in
	out.Items = append([]ChecklistItem{}, in.Items...)
	out.Context = copyContext(in.Context)
	return out
}

func copyContext(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeChecklistPhase(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pre":
		return "pre"
	case "post":
		return "post"
	default:
		return ""
	}
}
