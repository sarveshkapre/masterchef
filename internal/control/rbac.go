package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RBACPermission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope,omitempty"`
}

type RBACRole struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Permissions []RBACPermission `json:"permissions"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type RBACRoleInput struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Permissions []RBACPermission `json:"permissions"`
}

type RBACBinding struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	RoleID    string    `json:"role_id"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RBACBindingInput struct {
	Subject string `json:"subject"`
	RoleID  string `json:"role_id"`
	Scope   string `json:"scope,omitempty"`
}

type RBACAccessCheckInput struct {
	Subject  string `json:"subject"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope,omitempty"`
}

type RBACAccessCheckResult struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason,omitempty"`
	MatchedRoleID string `json:"matched_role_id,omitempty"`
	MatchedBindID string `json:"matched_binding_id,omitempty"`
}

type RBACStore struct {
	mu         sync.RWMutex
	nextRoleID int64
	nextBindID int64
	roles      map[string]*RBACRole
	bindings   map[string]*RBACBinding
}

func NewRBACStore() *RBACStore {
	return &RBACStore{
		roles:    map[string]*RBACRole{},
		bindings: map[string]*RBACBinding{},
	}
}

func (s *RBACStore) CreateRole(in RBACRoleInput) (RBACRole, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return RBACRole{}, errors.New("name is required")
	}
	permissions, err := normalizeRBACPermissions(in.Permissions)
	if err != nil {
		return RBACRole{}, err
	}
	now := time.Now().UTC()
	item := RBACRole{
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Permissions: permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRoleID++
	item.ID = "rbac-role-" + itoa(s.nextRoleID)
	s.roles[item.ID] = &item
	return cloneRBACRole(item), nil
}

func (s *RBACStore) ListRoles() []RBACRole {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RBACRole, 0, len(s.roles))
	for _, item := range s.roles {
		out = append(out, cloneRBACRole(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *RBACStore) GetRole(id string) (RBACRole, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.roles[strings.TrimSpace(id)]
	if !ok {
		return RBACRole{}, false
	}
	return cloneRBACRole(*item), true
}

func (s *RBACStore) CreateBinding(in RBACBindingInput) (RBACBinding, error) {
	subject := strings.TrimSpace(in.Subject)
	roleID := strings.TrimSpace(in.RoleID)
	scope := strings.TrimSpace(in.Scope)
	if subject == "" || roleID == "" {
		return RBACBinding{}, errors.New("subject and role_id are required")
	}
	if scope == "" {
		scope = "*"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[roleID]; !ok {
		return RBACBinding{}, errors.New("role not found")
	}
	now := time.Now().UTC()
	item := RBACBinding{
		Subject:   subject,
		RoleID:    roleID,
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.nextBindID++
	item.ID = "rbac-binding-" + itoa(s.nextBindID)
	s.bindings[item.ID] = &item
	return cloneRBACBinding(item), nil
}

func (s *RBACStore) ListBindings() []RBACBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RBACBinding, 0, len(s.bindings))
	for _, item := range s.bindings {
		out = append(out, cloneRBACBinding(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *RBACStore) CheckAccess(in RBACAccessCheckInput) RBACAccessCheckResult {
	subject := strings.TrimSpace(in.Subject)
	resource := strings.TrimSpace(in.Resource)
	action := strings.TrimSpace(in.Action)
	scope := strings.TrimSpace(in.Scope)
	if subject == "" || resource == "" || action == "" {
		return RBACAccessCheckResult{Allowed: false, Reason: "subject, resource, and action are required"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, binding := range s.bindings {
		if binding.Subject != subject {
			continue
		}
		if !rbacScopeMatches(binding.Scope, scope) {
			continue
		}
		role, ok := s.roles[binding.RoleID]
		if !ok {
			continue
		}
		for _, permission := range role.Permissions {
			if !rbacTokenMatches(permission.Resource, resource) {
				continue
			}
			if !rbacTokenMatches(permission.Action, action) {
				continue
			}
			perScope := strings.TrimSpace(permission.Scope)
			if perScope != "" && !rbacScopeMatches(perScope, scope) {
				continue
			}
			return RBACAccessCheckResult{
				Allowed:       true,
				MatchedRoleID: role.ID,
				MatchedBindID: binding.ID,
			}
		}
	}
	return RBACAccessCheckResult{Allowed: false, Reason: "no matching role binding permission"}
}

func normalizeRBACPermissions(in []RBACPermission) ([]RBACPermission, error) {
	if len(in) == 0 {
		return nil, errors.New("at least one permission is required")
	}
	out := make([]RBACPermission, 0, len(in))
	for _, item := range in {
		resource := strings.TrimSpace(item.Resource)
		action := strings.TrimSpace(item.Action)
		if resource == "" || action == "" {
			return nil, errors.New("permission resource and action are required")
		}
		out = append(out, RBACPermission{
			Resource: resource,
			Action:   action,
			Scope:    strings.TrimSpace(item.Scope),
		})
	}
	return out, nil
}

func rbacTokenMatches(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "*" {
		return true
	}
	return pattern == value
}

func rbacScopeMatches(pattern, scope string) bool {
	pattern = strings.TrimSpace(pattern)
	scope = strings.TrimSpace(scope)
	if pattern == "" || pattern == "*" {
		return true
	}
	if scope == pattern {
		return true
	}
	return strings.HasPrefix(scope, pattern+"/")
}

func cloneRBACRole(in RBACRole) RBACRole {
	out := in
	out.Permissions = append([]RBACPermission{}, in.Permissions...)
	return out
}

func cloneRBACBinding(in RBACBinding) RBACBinding {
	return in
}
