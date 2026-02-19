package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	PolicyEnforcementAudit               = "audit"
	PolicyEnforcementApplyAndMonitor     = "apply-and-monitor"
	PolicyEnforcementApplyAndAutocorrect = "apply-and-autocorrect"
)

type PolicyEnforcementModeInput struct {
	PolicyRef string `json:"policy_ref"`
	Mode      string `json:"mode"`
	Reason    string `json:"reason,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

type PolicyEnforcementMode struct {
	ID        string    `json:"id"`
	PolicyRef string    `json:"policy_ref"`
	Mode      string    `json:"mode"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PolicyEnforcementStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]PolicyEnforcementMode
}

func NewPolicyEnforcementStore() *PolicyEnforcementStore {
	return &PolicyEnforcementStore{items: map[string]PolicyEnforcementMode{}}
}

func (s *PolicyEnforcementStore) Upsert(in PolicyEnforcementModeInput) (PolicyEnforcementMode, error) {
	policyRef := strings.TrimSpace(in.PolicyRef)
	if policyRef == "" {
		return PolicyEnforcementMode{}, errors.New("policy_ref is required")
	}
	mode := normalizePolicyMode(in.Mode)
	if mode == "" {
		return PolicyEnforcementMode{}, errors.New("mode must be one of audit, apply-and-monitor, apply-and-autocorrect")
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.items[policyRef]
	if !ok {
		s.nextID++
		current = PolicyEnforcementMode{
			ID:        "polmode-" + itoa(s.nextID),
			PolicyRef: policyRef,
		}
	}
	current.Mode = mode
	current.Reason = strings.TrimSpace(in.Reason)
	current.UpdatedBy = strings.TrimSpace(in.UpdatedBy)
	current.UpdatedAt = now
	s.items[policyRef] = current
	return current, nil
}

func (s *PolicyEnforcementStore) Get(policyRef string) (PolicyEnforcementMode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[strings.TrimSpace(policyRef)]
	if !ok {
		return PolicyEnforcementMode{}, false
	}
	return item, true
}

func (s *PolicyEnforcementStore) List() []PolicyEnforcementMode {
	s.mu.RLock()
	out := make([]PolicyEnforcementMode, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].PolicyRef < out[j].PolicyRef
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func normalizePolicyMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case PolicyEnforcementAudit, PolicyEnforcementApplyAndMonitor, PolicyEnforcementApplyAndAutocorrect:
		return mode
	default:
		return ""
	}
}
