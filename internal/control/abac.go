package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ABACPolicy struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Effect     string            `json:"effect"` // allow|deny
	Subject    string            `json:"subject"`
	Resource   string            `json:"resource"`
	Action     string            `json:"action"`
	Conditions map[string]string `json:"conditions,omitempty"`
	Priority   int               `json:"priority"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type ABACPolicyInput struct {
	Name       string            `json:"name"`
	Effect     string            `json:"effect"`
	Subject    string            `json:"subject"`
	Resource   string            `json:"resource"`
	Action     string            `json:"action"`
	Conditions map[string]string `json:"conditions,omitempty"`
	Priority   int               `json:"priority,omitempty"`
}

type ABACCheckInput struct {
	Subject  string            `json:"subject"`
	Resource string            `json:"resource"`
	Action   string            `json:"action"`
	Context  map[string]string `json:"context,omitempty"`
}

type ABACCheckResult struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason,omitempty"`
	MatchedPolicy string `json:"matched_policy,omitempty"`
}

type ABACStore struct {
	mu     sync.RWMutex
	nextID int64
	policy map[string]*ABACPolicy
}

func NewABACStore() *ABACStore {
	return &ABACStore{
		policy: map[string]*ABACPolicy{},
	}
}

func (s *ABACStore) CreatePolicy(in ABACPolicyInput) (ABACPolicy, error) {
	name := strings.TrimSpace(in.Name)
	effect := strings.ToLower(strings.TrimSpace(in.Effect))
	subject := strings.TrimSpace(in.Subject)
	resource := strings.TrimSpace(in.Resource)
	action := strings.TrimSpace(in.Action)
	if name == "" || effect == "" || subject == "" || resource == "" || action == "" {
		return ABACPolicy{}, errors.New("name, effect, subject, resource, and action are required")
	}
	if effect != "allow" && effect != "deny" {
		return ABACPolicy{}, errors.New("effect must be allow or deny")
	}
	conditions := map[string]string{}
	for k, v := range in.Conditions {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		conditions[key] = strings.TrimSpace(v)
	}
	now := time.Now().UTC()
	item := ABACPolicy{
		Name:       name,
		Effect:     effect,
		Subject:    subject,
		Resource:   resource,
		Action:     action,
		Conditions: conditions,
		Priority:   in.Priority,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item.ID = "abac-policy-" + itoa(s.nextID)
	s.policy[item.ID] = &item
	return cloneABACPolicy(item), nil
}

func (s *ABACStore) ListPolicies() []ABACPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ABACPolicy, 0, len(s.policy))
	for _, item := range s.policy {
		out = append(out, cloneABACPolicy(*item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ABACStore) Check(in ABACCheckInput) ABACCheckResult {
	subject := strings.TrimSpace(in.Subject)
	resource := strings.TrimSpace(in.Resource)
	action := strings.TrimSpace(in.Action)
	if subject == "" || resource == "" || action == "" {
		return ABACCheckResult{Allowed: false, Reason: "subject, resource, and action are required"}
	}
	policies := s.ListPolicies()
	for _, policy := range policies {
		if !abacTokenMatch(policy.Subject, subject) {
			continue
		}
		if !abacTokenMatch(policy.Resource, resource) {
			continue
		}
		if !abacTokenMatch(policy.Action, action) {
			continue
		}
		if !abacConditionsMatch(policy.Conditions, in.Context) {
			continue
		}
		if policy.Effect == "deny" {
			return ABACCheckResult{
				Allowed:       false,
				Reason:        "denied by policy",
				MatchedPolicy: policy.ID,
			}
		}
		return ABACCheckResult{
			Allowed:       true,
			MatchedPolicy: policy.ID,
		}
	}
	return ABACCheckResult{Allowed: false, Reason: "no matching abac policy"}
}

func abacTokenMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "*" {
		return true
	}
	return pattern == value
}

func abacConditionsMatch(conditions, context map[string]string) bool {
	for key, expected := range conditions {
		if strings.TrimSpace(context[key]) != expected {
			return false
		}
	}
	return true
}

func cloneABACPolicy(in ABACPolicy) ABACPolicy {
	out := in
	out.Conditions = map[string]string{}
	for k, v := range in.Conditions {
		out.Conditions[k] = v
	}
	return out
}
