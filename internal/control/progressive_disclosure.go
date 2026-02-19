package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProgressiveDisclosureProfile struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	DefaultControls  []string  `json:"default_controls"`
	AdvancedControls []string  `json:"advanced_controls,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ProgressiveDisclosureState struct {
	ProfileID        string              `json:"profile_id"`
	ProfileName      string              `json:"profile_name"`
	VisibleControls  []string            `json:"visible_controls"`
	RevealedByFlow   map[string][]string `json:"revealed_by_flow,omitempty"`
	LastWorkflowHint string              `json:"last_workflow_hint,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type ProgressiveDisclosureStore struct {
	mu       sync.RWMutex
	profiles map[string]*ProgressiveDisclosureProfile
	state    ProgressiveDisclosureState
}

func NewProgressiveDisclosureStore() *ProgressiveDisclosureStore {
	now := time.Now().UTC()
	profiles := map[string]*ProgressiveDisclosureProfile{
		"simple": {
			ID:              "simple",
			Name:            "Simple",
			Description:     "Show only core controls and defer advanced options.",
			DefaultControls: []string{"plan", "apply", "drift", "health", "rollback"},
			AdvancedControls: []string{
				"failure-thresholds",
				"disruption-budgets",
				"concurrency-override",
				"custom-selector",
			},
			UpdatedAt: now,
		},
		"balanced": {
			ID:              "balanced",
			Name:            "Balanced",
			Description:     "Show most common advanced controls in context.",
			DefaultControls: []string{"plan", "apply", "drift", "health", "rollback", "rollout-strategy", "batch-size"},
			AdvancedControls: []string{
				"failure-thresholds",
				"disruption-budgets",
				"concurrency-override",
				"custom-selector",
				"policy-simulation-min-confidence",
			},
			UpdatedAt: now,
		},
		"advanced": {
			ID:              "advanced",
			Name:            "Advanced",
			Description:     "Show all controls by default for expert operators.",
			DefaultControls: []string{"plan", "apply", "drift", "health", "rollback", "rollout-strategy", "batch-size", "failure-thresholds", "disruption-budgets", "concurrency-override", "custom-selector", "policy-simulation-min-confidence"},
			UpdatedAt:       now,
		},
	}
	state := ProgressiveDisclosureState{
		ProfileID:       "simple",
		ProfileName:     profiles["simple"].Name,
		VisibleControls: append([]string{}, profiles["simple"].DefaultControls...),
		RevealedByFlow:  map[string][]string{},
		UpdatedAt:       now,
	}
	return &ProgressiveDisclosureStore{
		profiles: profiles,
		state:    state,
	}
}

func (s *ProgressiveDisclosureStore) ListProfiles() []ProgressiveDisclosureProfile {
	s.mu.RLock()
	out := make([]ProgressiveDisclosureProfile, 0, len(s.profiles))
	for _, item := range s.profiles {
		copied := *item
		copied.DefaultControls = append([]string{}, item.DefaultControls...)
		copied.AdvancedControls = append([]string{}, item.AdvancedControls...)
		out = append(out, copied)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *ProgressiveDisclosureStore) ActiveState() ProgressiveDisclosureState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProgressiveState(s.state)
}

func (s *ProgressiveDisclosureStore) SetProfile(profileID, workflowHint string) (ProgressiveDisclosureState, error) {
	id := strings.TrimSpace(strings.ToLower(profileID))
	if id == "" {
		return ProgressiveDisclosureState{}, errors.New("profile_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[id]
	if !ok {
		return ProgressiveDisclosureState{}, errors.New("profile not found")
	}
	s.state.ProfileID = profile.ID
	s.state.ProfileName = profile.Name
	s.state.VisibleControls = append([]string{}, profile.DefaultControls...)
	s.state.RevealedByFlow = map[string][]string{}
	s.state.LastWorkflowHint = strings.TrimSpace(workflowHint)
	s.state.UpdatedAt = time.Now().UTC()
	return cloneProgressiveState(s.state), nil
}

func (s *ProgressiveDisclosureStore) RevealForWorkflow(workflow string, controls []string) (ProgressiveDisclosureState, error) {
	workflow = strings.TrimSpace(strings.ToLower(workflow))
	if workflow == "" {
		return ProgressiveDisclosureState{}, errors.New("workflow is required")
	}
	normalized := normalizeProgressiveControls(controls)
	if len(normalized) == 0 {
		return ProgressiveDisclosureState{}, errors.New("controls must contain at least one control")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.RevealedByFlow == nil {
		s.state.RevealedByFlow = map[string][]string{}
	}
	s.state.RevealedByFlow[workflow] = mergeUniqueStrings(s.state.RevealedByFlow[workflow], normalized)
	s.state.VisibleControls = mergeUniqueStrings(s.state.VisibleControls, normalized)
	s.state.LastWorkflowHint = workflow
	s.state.UpdatedAt = time.Now().UTC()
	return cloneProgressiveState(s.state), nil
}

func normalizeProgressiveControls(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		key := strings.TrimSpace(strings.ToLower(item))
		if key == "" {
			continue
		}
		key = strings.ReplaceAll(key, " ", "-")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func mergeUniqueStrings(left, right []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(left)+len(right))
	for _, item := range left {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	for _, item := range right {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func cloneProgressiveState(in ProgressiveDisclosureState) ProgressiveDisclosureState {
	out := in
	out.VisibleControls = append([]string{}, in.VisibleControls...)
	out.RevealedByFlow = map[string][]string{}
	for workflow, controls := range in.RevealedByFlow {
		out.RevealedByFlow[workflow] = append([]string{}, controls...)
	}
	return out
}
