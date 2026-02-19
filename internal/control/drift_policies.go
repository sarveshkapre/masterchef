package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DriftSuppression struct {
	ID         string    `json:"id"`
	ScopeType  string    `json:"scope_type"` // all|host|resource_type|resource_id
	ScopeValue string    `json:"scope_value,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Until      time.Time `json:"until"`
}

type DriftSuppressionInput struct {
	ScopeType  string    `json:"scope_type"`
	ScopeValue string    `json:"scope_value,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	Until      time.Time `json:"until"`
}

type DriftAllowlistEntry struct {
	ID         string    `json:"id"`
	ScopeType  string    `json:"scope_type"` // all|host|resource_type|resource_id
	ScopeValue string    `json:"scope_value,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

type DriftAllowlistInput struct {
	ScopeType  string    `json:"scope_type"`
	ScopeValue string    `json:"scope_value,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

type DriftPolicyStore struct {
	mu              sync.RWMutex
	nextSuppression int64
	nextAllowlist   int64
	suppressions    map[string]DriftSuppression
	allowlist       map[string]DriftAllowlistEntry
}

func NewDriftPolicyStore() *DriftPolicyStore {
	return &DriftPolicyStore{
		suppressions: map[string]DriftSuppression{},
		allowlist:    map[string]DriftAllowlistEntry{},
	}
}

func (s *DriftPolicyStore) AddSuppression(in DriftSuppressionInput) (DriftSuppression, error) {
	scopeType, scopeValue, err := normalizeDriftScope(in.ScopeType, in.ScopeValue)
	if err != nil {
		return DriftSuppression{}, err
	}
	until := in.Until.UTC()
	if until.IsZero() || !until.After(time.Now().UTC()) {
		return DriftSuppression{}, errors.New("until must be in the future")
	}
	item := DriftSuppression{
		ScopeType:  scopeType,
		ScopeValue: scopeValue,
		Reason:     strings.TrimSpace(in.Reason),
		CreatedBy:  strings.TrimSpace(in.CreatedBy),
		CreatedAt:  time.Now().UTC(),
		Until:      until,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSuppression++
	item.ID = "drift-sup-" + itoa(s.nextSuppression)
	s.suppressions[item.ID] = item
	return item, nil
}

func (s *DriftPolicyStore) DeleteSuppression(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.suppressions[id]; !ok {
		return false
	}
	delete(s.suppressions, id)
	return true
}

func (s *DriftPolicyStore) ListSuppressions(includeExpired bool) []DriftSuppression {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	out := make([]DriftSuppression, 0, len(s.suppressions))
	for _, item := range s.suppressions {
		if !includeExpired && !item.Until.After(now) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *DriftPolicyStore) AddAllowlist(in DriftAllowlistInput) (DriftAllowlistEntry, error) {
	scopeType, scopeValue, err := normalizeDriftScope(in.ScopeType, in.ScopeValue)
	if err != nil {
		return DriftAllowlistEntry{}, err
	}
	expires := in.ExpiresAt.UTC()
	if !expires.IsZero() && !expires.After(time.Now().UTC()) {
		return DriftAllowlistEntry{}, errors.New("expires_at must be in the future")
	}
	item := DriftAllowlistEntry{
		ScopeType:  scopeType,
		ScopeValue: scopeValue,
		Reason:     strings.TrimSpace(in.Reason),
		CreatedBy:  strings.TrimSpace(in.CreatedBy),
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  expires,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextAllowlist++
	item.ID = "drift-allow-" + itoa(s.nextAllowlist)
	s.allowlist[item.ID] = item
	return item, nil
}

func (s *DriftPolicyStore) DeleteAllowlist(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.allowlist[id]; !ok {
		return false
	}
	delete(s.allowlist, id)
	return true
}

func (s *DriftPolicyStore) ListAllowlist(includeExpired bool) []DriftAllowlistEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	out := make([]DriftAllowlistEntry, 0, len(s.allowlist))
	for _, item := range s.allowlist {
		if !includeExpired && !item.ExpiresAt.IsZero() && !item.ExpiresAt.After(now) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *DriftPolicyStore) IsSuppressed(host, resourceType, resourceID string, at time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.suppressions {
		if !item.Until.After(at.UTC()) {
			continue
		}
		if driftScopeMatches(item.ScopeType, item.ScopeValue, host, resourceType, resourceID) {
			return true
		}
	}
	return false
}

func (s *DriftPolicyStore) IsAllowlisted(host, resourceType, resourceID string, at time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.allowlist {
		if !item.ExpiresAt.IsZero() && !item.ExpiresAt.After(at.UTC()) {
			continue
		}
		if driftScopeMatches(item.ScopeType, item.ScopeValue, host, resourceType, resourceID) {
			return true
		}
	}
	return false
}

func normalizeDriftScope(scopeType, scopeValue string) (string, string, error) {
	typ := strings.ToLower(strings.TrimSpace(scopeType))
	if typ == "" {
		typ = "all"
	}
	switch typ {
	case "all", "host", "resource_type", "resource_id":
	default:
		return "", "", errors.New("scope_type must be one of all, host, resource_type, resource_id")
	}
	val := strings.ToLower(strings.TrimSpace(scopeValue))
	if typ != "all" && val == "" {
		return "", "", errors.New("scope_value is required for scoped entries")
	}
	if typ == "all" {
		val = ""
	}
	return typ, val, nil
}

func driftScopeMatches(scopeType, scopeValue, host, resourceType, resourceID string) bool {
	scopeType = strings.ToLower(strings.TrimSpace(scopeType))
	scopeValue = strings.ToLower(strings.TrimSpace(scopeValue))
	host = strings.ToLower(strings.TrimSpace(host))
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	resourceID = strings.ToLower(strings.TrimSpace(resourceID))
	switch scopeType {
	case "all":
		return true
	case "host":
		return host == scopeValue
	case "resource_type":
		return resourceType == scopeValue
	case "resource_id":
		return resourceID == scopeValue
	default:
		return false
	}
}
