package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DelegatedAdminGrantInput struct {
	Tenant      string   `json:"tenant"`
	Environment string   `json:"environment"`
	Principal   string   `json:"principal"`
	Scopes      []string `json:"scopes"`
	Delegator   string   `json:"delegator,omitempty"`
}

type DelegatedAdminGrant struct {
	ID          string    `json:"id"`
	Tenant      string    `json:"tenant"`
	Environment string    `json:"environment"`
	Principal   string    `json:"principal"`
	Scopes      []string  `json:"scopes"`
	Delegator   string    `json:"delegator,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type DelegatedAdminAuthorizeInput struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment"`
	Principal   string `json:"principal"`
	Action      string `json:"action"`
}

type DelegatedAdminAuthorizeDecision struct {
	Allowed      bool   `json:"allowed"`
	GrantID      string `json:"grant_id,omitempty"`
	MatchedScope string `json:"matched_scope,omitempty"`
	Reason       string `json:"reason"`
}

type DelegatedAdminStore struct {
	mu     sync.RWMutex
	nextID int64
	grants map[string]*DelegatedAdminGrant
}

func NewDelegatedAdminStore() *DelegatedAdminStore {
	return &DelegatedAdminStore{grants: map[string]*DelegatedAdminGrant{}}
}

func (s *DelegatedAdminStore) Create(in DelegatedAdminGrantInput) (DelegatedAdminGrant, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	principal := strings.ToLower(strings.TrimSpace(in.Principal))
	if tenant == "" || environment == "" || principal == "" {
		return DelegatedAdminGrant{}, errors.New("tenant, environment, and principal are required")
	}

	scopes := normalizeScopes(in.Scopes)
	if len(scopes) == 0 {
		return DelegatedAdminGrant{}, errors.New("at least one scope is required")
	}
	item := DelegatedAdminGrant{
		Tenant:      tenant,
		Environment: environment,
		Principal:   principal,
		Scopes:      scopes,
		Delegator:   strings.TrimSpace(in.Delegator),
		CreatedAt:   time.Now().UTC(),
	}

	s.mu.Lock()
	s.nextID++
	item.ID = "delegated-admin-grant-" + itoa(s.nextID)
	s.grants[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *DelegatedAdminStore) List() []DelegatedAdminGrant {
	s.mu.RLock()
	out := make([]DelegatedAdminGrant, 0, len(s.grants))
	for _, item := range s.grants {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant == out[j].Tenant {
			if out[i].Environment == out[j].Environment {
				return out[i].Principal < out[j].Principal
			}
			return out[i].Environment < out[j].Environment
		}
		return out[i].Tenant < out[j].Tenant
	})
	return out
}

func (s *DelegatedAdminStore) Authorize(in DelegatedAdminAuthorizeInput) DelegatedAdminAuthorizeDecision {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	principal := strings.ToLower(strings.TrimSpace(in.Principal))
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if tenant == "" || environment == "" || principal == "" || action == "" {
		return DelegatedAdminAuthorizeDecision{Allowed: false, Reason: "tenant, environment, principal, and action are required"}
	}

	s.mu.RLock()
	grants := make([]DelegatedAdminGrant, 0, len(s.grants))
	for _, item := range s.grants {
		grants = append(grants, *item)
	}
	s.mu.RUnlock()

	for _, grant := range grants {
		if grant.Tenant != tenant {
			continue
		}
		if grant.Environment != "*" && grant.Environment != environment {
			continue
		}
		if grant.Principal != principal {
			continue
		}
		for _, scope := range grant.Scopes {
			if matchesScope(scope, action) {
				return DelegatedAdminAuthorizeDecision{
					Allowed:      true,
					GrantID:      grant.ID,
					MatchedScope: scope,
					Reason:       "delegated admin grant allows action",
				}
			}
		}
	}
	return DelegatedAdminAuthorizeDecision{
		Allowed: false,
		Reason:  "no delegated admin grant matched action",
	}
}

func normalizeScopes(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		scope := strings.ToLower(strings.TrimSpace(raw))
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func matchesScope(scope, action string) bool {
	if scope == "*" || scope == action {
		return true
	}
	if strings.HasSuffix(scope, ".*") {
		prefix := strings.TrimSuffix(scope, ".*")
		return strings.HasPrefix(action, prefix+".")
	}
	return false
}
