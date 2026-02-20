package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RunbookStatus string

const (
	RunbookDraft      RunbookStatus = "draft"
	RunbookApproved   RunbookStatus = "approved"
	RunbookDeprecated RunbookStatus = "deprecated"
)

type RunbookTargetType string

const (
	RunbookTargetTemplate RunbookTargetType = "template"
	RunbookTargetWorkflow RunbookTargetType = "workflow"
	RunbookTargetConfig   RunbookTargetType = "config"
)

type Runbook struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	TargetType  RunbookTargetType `json:"target_type"`
	TargetID    string            `json:"target_id,omitempty"`
	ConfigPath  string            `json:"config_path,omitempty"`
	RiskLevel   string            `json:"risk_level,omitempty"` // low|medium|high
	Owner       string            `json:"owner,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Status      RunbookStatus     `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type RunbookStore struct {
	mu       sync.RWMutex
	nextID   int64
	runbooks map[string]*Runbook
}

type RunbookCatalogQuery struct {
	Owner        string `json:"owner,omitempty"`
	Tag          string `json:"tag,omitempty"`
	MaxRiskLevel string `json:"max_risk_level,omitempty"` // low|medium|high
	Limit        int    `json:"limit,omitempty"`
}

func NewRunbookStore() *RunbookStore {
	return &RunbookStore{
		runbooks: map[string]*Runbook{},
	}
}

func (s *RunbookStore) Create(in Runbook) (Runbook, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Runbook{}, errors.New("runbook name is required")
	}
	targetType := normalizeRunbookTargetType(string(in.TargetType))
	if targetType == "" {
		return Runbook{}, errors.New("target_type must be template, workflow, or config")
	}
	in.TargetType = RunbookTargetType(targetType)
	in.RiskLevel = normalizeRiskLevel(in.RiskLevel)
	switch in.TargetType {
	case RunbookTargetTemplate, RunbookTargetWorkflow:
		if strings.TrimSpace(in.TargetID) == "" {
			return Runbook{}, errors.New("target_id is required for template/workflow runbooks")
		}
	case RunbookTargetConfig:
		if strings.TrimSpace(in.ConfigPath) == "" {
			return Runbook{}, errors.New("config_path is required for config runbooks")
		}
	}
	in.Tags = normalizeTags(in.Tags)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := time.Now().UTC()
	in.ID = "rb-" + itoa(s.nextID)
	in.Status = RunbookDraft
	in.CreatedAt = now
	in.UpdatedAt = now
	cp := cloneRunbook(in)
	s.runbooks[in.ID] = &cp
	return cp, nil
}

func (s *RunbookStore) List() []Runbook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Runbook, 0, len(s.runbooks))
	for _, rb := range s.runbooks {
		out = append(out, cloneRunbook(*rb))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *RunbookStore) Catalog(query RunbookCatalogQuery) []Runbook {
	owner := strings.ToLower(strings.TrimSpace(query.Owner))
	tag := strings.ToLower(strings.TrimSpace(query.Tag))
	maxRisk := normalizeRiskLevel(query.MaxRiskLevel)
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	s.mu.RLock()
	out := make([]Runbook, 0, len(s.runbooks))
	for _, rb := range s.runbooks {
		if rb.Status != RunbookApproved {
			continue
		}
		if owner != "" && strings.ToLower(strings.TrimSpace(rb.Owner)) != owner {
			continue
		}
		if tag != "" && !containsRunbookTag(rb.Tags, tag) {
			continue
		}
		if riskLevelRank(rb.RiskLevel) > riskLevelRank(maxRisk) {
			continue
		}
		out = append(out, cloneRunbook(*rb))
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

func (s *RunbookStore) Get(id string) (Runbook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rb, ok := s.runbooks[strings.TrimSpace(id)]
	if !ok {
		return Runbook{}, errors.New("runbook not found")
	}
	return cloneRunbook(*rb), nil
}

func (s *RunbookStore) Approve(id string) (Runbook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rb, ok := s.runbooks[strings.TrimSpace(id)]
	if !ok {
		return Runbook{}, errors.New("runbook not found")
	}
	rb.Status = RunbookApproved
	rb.UpdatedAt = time.Now().UTC()
	return cloneRunbook(*rb), nil
}

func (s *RunbookStore) Deprecate(id string) (Runbook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rb, ok := s.runbooks[strings.TrimSpace(id)]
	if !ok {
		return Runbook{}, errors.New("runbook not found")
	}
	rb.Status = RunbookDeprecated
	rb.UpdatedAt = time.Now().UTC()
	return cloneRunbook(*rb), nil
}

func normalizeRunbookTargetType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "template":
		return "template"
	case "workflow":
		return "workflow"
	case "config":
		return "config"
	default:
		return ""
	}
}

func normalizeRiskLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func riskLevelRank(level string) int {
	switch normalizeRiskLevel(level) {
	case "low":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func normalizeTags(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, tag := range in {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func cloneRunbook(in Runbook) Runbook {
	out := in
	out.Tags = append([]string{}, in.Tags...)
	return out
}

func containsRunbookTag(tags []string, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	for _, item := range tags {
		if strings.ToLower(strings.TrimSpace(item)) == tag {
			return true
		}
	}
	return false
}
