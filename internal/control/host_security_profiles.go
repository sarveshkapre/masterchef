package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type HostSecurityProfileInput struct {
	ID          string   `json:"id,omitempty"`
	Mode        string   `json:"mode"` // selinux|apparmor
	TargetKind  string   `json:"target_kind"`
	Target      string   `json:"target"`
	State       string   `json:"state"` // enforcing|permissive|complain|disabled
	Contexts    []string `json:"contexts,omitempty"`
	Profiles    []string `json:"profiles,omitempty"`
	Description string   `json:"description,omitempty"`
}

type HostSecurityProfile struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`
	TargetKind  string    `json:"target_kind"`
	Target      string    `json:"target"`
	State       string    `json:"state"`
	Contexts    []string  `json:"contexts,omitempty"`
	Profiles    []string  `json:"profiles,omitempty"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type HostSecurityEvaluateInput struct {
	Mode           string `json:"mode"`
	TargetKind     string `json:"target_kind"`
	Target         string `json:"target"`
	RequestedState string `json:"requested_state"`
}

type HostSecurityDecision struct {
	Allowed        bool   `json:"allowed"`
	ProfileID      string `json:"profile_id,omitempty"`
	Mode           string `json:"mode"`
	CurrentState   string `json:"current_state,omitempty"`
	RequestedState string `json:"requested_state"`
	EffectiveState string `json:"effective_state,omitempty"`
	Reason         string `json:"reason"`
}

type HostSecurityProfileStore struct {
	mu       sync.RWMutex
	nextID   int64
	profiles map[string]*HostSecurityProfile
}

func NewHostSecurityProfileStore() *HostSecurityProfileStore {
	return &HostSecurityProfileStore{profiles: map[string]*HostSecurityProfile{}}
}

func (s *HostSecurityProfileStore) Upsert(in HostSecurityProfileInput) (HostSecurityProfile, error) {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode != "selinux" && mode != "apparmor" {
		return HostSecurityProfile{}, errors.New("mode must be selinux or apparmor")
	}
	targetKind := strings.ToLower(strings.TrimSpace(in.TargetKind))
	target := strings.ToLower(strings.TrimSpace(in.Target))
	if targetKind == "" || target == "" {
		return HostSecurityProfile{}, errors.New("target_kind and target are required")
	}
	state := strings.ToLower(strings.TrimSpace(in.State))
	if !validHostSecurityState(mode, state) {
		return HostSecurityProfile{}, errors.New("invalid state for selected mode")
	}
	item := HostSecurityProfile{
		Mode:        mode,
		TargetKind:  targetKind,
		Target:      target,
		State:       state,
		Contexts:    normalizeStringList(in.Contexts),
		Profiles:    normalizeStringList(in.Profiles),
		Description: strings.TrimSpace(in.Description),
		UpdatedAt:   time.Now().UTC(),
	}
	id := strings.ToLower(strings.TrimSpace(in.ID))
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" {
		item.ID = id
		s.profiles[id] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "host-security-profile-" + itoa(s.nextID)
	s.profiles[item.ID] = &item
	return item, nil
}

func (s *HostSecurityProfileStore) List() []HostSecurityProfile {
	s.mu.RLock()
	out := make([]HostSecurityProfile, 0, len(s.profiles))
	for _, item := range s.profiles {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mode == out[j].Mode {
			if out[i].TargetKind == out[j].TargetKind {
				return out[i].Target < out[j].Target
			}
			return out[i].TargetKind < out[j].TargetKind
		}
		return out[i].Mode < out[j].Mode
	})
	return out
}

func (s *HostSecurityProfileStore) Evaluate(in HostSecurityEvaluateInput) HostSecurityDecision {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	targetKind := strings.ToLower(strings.TrimSpace(in.TargetKind))
	target := strings.ToLower(strings.TrimSpace(in.Target))
	requested := strings.ToLower(strings.TrimSpace(in.RequestedState))
	if mode == "" || targetKind == "" || target == "" || requested == "" {
		return HostSecurityDecision{
			Allowed:        false,
			Mode:           mode,
			RequestedState: requested,
			Reason:         "mode, target_kind, target, and requested_state are required",
		}
	}
	if !validHostSecurityState(mode, requested) {
		return HostSecurityDecision{
			Allowed:        false,
			Mode:           mode,
			RequestedState: requested,
			Reason:         "requested_state is invalid for selected mode",
		}
	}

	for _, item := range s.List() {
		if item.Mode != mode || item.TargetKind != targetKind || item.Target != target {
			continue
		}
		decision := HostSecurityDecision{
			Allowed:        true,
			ProfileID:      item.ID,
			Mode:           mode,
			CurrentState:   item.State,
			RequestedState: requested,
			EffectiveState: requested,
			Reason:         "request is compliant with host security profile",
		}
		if item.State == "disabled" && requested != "disabled" {
			decision.Allowed = false
			decision.EffectiveState = item.State
			decision.Reason = "profile is disabled; state transitions are blocked"
		}
		if mode == "selinux" && item.State == "enforcing" && requested == "permissive" {
			decision.Allowed = false
			decision.EffectiveState = item.State
			decision.Reason = "downgrade from enforcing to permissive blocked by profile"
		}
		if mode == "apparmor" && item.State == "enforcing" && requested == "complain" {
			decision.Allowed = false
			decision.EffectiveState = item.State
			decision.Reason = "downgrade from enforcing to complain blocked by profile"
		}
		return decision
	}

	return HostSecurityDecision{
		Allowed:        true,
		Mode:           mode,
		RequestedState: requested,
		EffectiveState: requested,
		Reason:         "no profile matched target; default allow",
	}
}

func validHostSecurityState(mode, state string) bool {
	switch mode {
	case "selinux":
		return state == "enforcing" || state == "permissive" || state == "disabled"
	case "apparmor":
		return state == "enforcing" || state == "complain" || state == "disabled"
	default:
		return false
	}
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		item := strings.ToLower(strings.TrimSpace(raw))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
