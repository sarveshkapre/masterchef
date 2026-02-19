package control

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RoleDefinition struct {
	Name               string         `json:"name"`
	Description        string         `json:"description,omitempty"`
	Profiles           []string       `json:"profiles,omitempty"`
	RunList            []string       `json:"run_list,omitempty"`
	PolicyGroup        string         `json:"policy_group,omitempty"`
	DefaultAttributes  map[string]any `json:"default_attributes,omitempty"`
	OverrideAttributes map[string]any `json:"override_attributes,omitempty"`
	UpdatedAt          time.Time      `json:"updated_at"`
	Source             string         `json:"source"` // api|file
}

type EnvironmentDefinition struct {
	Name               string              `json:"name"`
	Description        string              `json:"description,omitempty"`
	PolicyGroup        string              `json:"policy_group,omitempty"`
	DefaultAttributes  map[string]any      `json:"default_attributes,omitempty"`
	OverrideAttributes map[string]any      `json:"override_attributes,omitempty"`
	RunListOverrides   map[string][]string `json:"run_list_overrides,omitempty"` // role -> run list
	PolicyOverrides    map[string]any      `json:"policy_overrides,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
	Source             string              `json:"source"` // api|file
}

type RoleEnvironmentResolution struct {
	Role        string         `json:"role"`
	Environment string         `json:"environment"`
	RunList     []string       `json:"run_list"`
	Attributes  map[string]any `json:"attributes"`
	PolicyGroup string         `json:"policy_group,omitempty"`
	Precedence  []string       `json:"precedence"`
	ResolvedAt  time.Time      `json:"resolved_at"`
}

type RoleEnvironmentStore struct {
	mu           sync.RWMutex
	roles        map[string]RoleDefinition
	environments map[string]EnvironmentDefinition
	rolesDir     string
	envDir       string
}

func NewRoleEnvironmentStore(baseDir string) *RoleEnvironmentStore {
	root := filepath.Join(baseDir, ".masterchef", "policy")
	rolesDir := filepath.Join(root, "roles")
	envDir := filepath.Join(root, "environments")
	_ = os.MkdirAll(rolesDir, 0o755)
	_ = os.MkdirAll(envDir, 0o755)

	s := &RoleEnvironmentStore{
		roles:        map[string]RoleDefinition{},
		environments: map[string]EnvironmentDefinition{},
		rolesDir:     rolesDir,
		envDir:       envDir,
	}
	s.loadFromDisk()
	return s
}

func (s *RoleEnvironmentStore) UpsertRole(role RoleDefinition) (RoleDefinition, error) {
	name := normalizeRoleEnvName(role.Name)
	if name == "" {
		return RoleDefinition{}, errors.New("name is required")
	}
	role.Name = name
	role.Profiles = normalizeRoleProfiles(role.Profiles)
	role.RunList = normalizeRunList(role.RunList)
	role.PolicyGroup = strings.TrimSpace(role.PolicyGroup)
	role.DefaultAttributes = cloneRoleEnvMap(role.DefaultAttributes)
	role.OverrideAttributes = cloneRoleEnvMap(role.OverrideAttributes)
	role.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(role.Source) == "" {
		role.Source = "api"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.roles[name] = cloneRole(role)
	if err := writeRoleEnvJSON(filepath.Join(s.rolesDir, name+".json"), role); err != nil {
		return RoleDefinition{}, err
	}
	return cloneRole(role), nil
}

func (s *RoleEnvironmentStore) UpsertEnvironment(env EnvironmentDefinition) (EnvironmentDefinition, error) {
	name := normalizeRoleEnvName(env.Name)
	if name == "" {
		return EnvironmentDefinition{}, errors.New("name is required")
	}
	env.Name = name
	env.PolicyGroup = strings.TrimSpace(env.PolicyGroup)
	env.DefaultAttributes = cloneRoleEnvMap(env.DefaultAttributes)
	env.OverrideAttributes = cloneRoleEnvMap(env.OverrideAttributes)
	env.PolicyOverrides = cloneRoleEnvMap(env.PolicyOverrides)
	env.RunListOverrides = normalizeRunListOverrides(env.RunListOverrides)
	env.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(env.Source) == "" {
		env.Source = "api"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.environments[name] = cloneEnvironment(env)
	if err := writeRoleEnvJSON(filepath.Join(s.envDir, name+".json"), env); err != nil {
		return EnvironmentDefinition{}, err
	}
	return cloneEnvironment(env), nil
}

func (s *RoleEnvironmentStore) GetRole(name string) (RoleDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[normalizeRoleEnvName(name)]
	if !ok {
		return RoleDefinition{}, errors.New("role not found")
	}
	return cloneRole(role), nil
}

func (s *RoleEnvironmentStore) GetEnvironment(name string) (EnvironmentDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	env, ok := s.environments[normalizeRoleEnvName(name)]
	if !ok {
		return EnvironmentDefinition{}, errors.New("environment not found")
	}
	return cloneEnvironment(env), nil
}

func (s *RoleEnvironmentStore) ListRoles() []RoleDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RoleDefinition, 0, len(s.roles))
	for _, role := range s.roles {
		out = append(out, cloneRole(role))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *RoleEnvironmentStore) ListEnvironments() []EnvironmentDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EnvironmentDefinition, 0, len(s.environments))
	for _, env := range s.environments {
		out = append(out, cloneEnvironment(env))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *RoleEnvironmentStore) DeleteRole(name string) bool {
	name = normalizeRoleEnvName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[name]; !ok {
		return false
	}
	delete(s.roles, name)
	_ = os.Remove(filepath.Join(s.rolesDir, name+".json"))
	return true
}

func (s *RoleEnvironmentStore) DeleteEnvironment(name string) bool {
	name = normalizeRoleEnvName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.environments[name]; !ok {
		return false
	}
	delete(s.environments, name)
	_ = os.Remove(filepath.Join(s.envDir, name+".json"))
	return true
}

func (s *RoleEnvironmentStore) Resolve(roleName, envName string) (RoleEnvironmentResolution, error) {
	s.mu.RLock()
	roleName = normalizeRoleEnvName(roleName)
	role, ok := s.roles[roleName]
	if !ok {
		s.mu.RUnlock()
		return RoleEnvironmentResolution{}, errors.New("role not found")
	}
	env, ok := s.environments[normalizeRoleEnvName(envName)]
	if !ok {
		s.mu.RUnlock()
		return RoleEnvironmentResolution{}, errors.New("environment not found")
	}
	hier, err := s.resolveRoleHierarchyLocked(role.Name, map[string]struct{}{}, map[string]roleHierarchy{})
	s.mu.RUnlock()
	if err != nil {
		return RoleEnvironmentResolution{}, err
	}
	runList := append([]string{}, hier.RunList...)
	if override := env.RunListOverrides[role.Name]; len(override) > 0 {
		runList = append([]string{}, override...)
	}
	attrs := map[string]any{}
	mergeRoleEnvMap(attrs, hier.DefaultAttributes)
	mergeRoleEnvMap(attrs, env.DefaultAttributes)
	mergeRoleEnvMap(attrs, hier.OverrideAttributes)
	mergeRoleEnvMap(attrs, env.OverrideAttributes)
	mergeRoleEnvMap(attrs, env.PolicyOverrides)

	policyGroup := strings.TrimSpace(env.PolicyGroup)
	if policyGroup == "" {
		policyGroup = strings.TrimSpace(hier.PolicyGroup)
	}
	return RoleEnvironmentResolution{
		Role:        role.Name,
		Environment: env.Name,
		RunList:     runList,
		Attributes:  attrs,
		PolicyGroup: policyGroup,
		Precedence: append(append([]string{}, hier.Precedence...),
			"environment.default_attributes",
			"environment.override_attributes",
			"environment.policy_overrides",
		),
		ResolvedAt: time.Now().UTC(),
	}, nil
}

func (s *RoleEnvironmentStore) loadFromDisk() {
	loadRoles := func() {
		files, err := filepath.Glob(filepath.Join(s.rolesDir, "*.json"))
		if err != nil {
			return
		}
		for _, file := range files {
			var role RoleDefinition
			if !readRoleEnvJSON(file, &role) {
				continue
			}
			role.Name = normalizeRoleEnvName(role.Name)
			if role.Name == "" {
				continue
			}
			role.Profiles = normalizeRoleProfiles(role.Profiles)
			role.RunList = normalizeRunList(role.RunList)
			role.DefaultAttributes = cloneRoleEnvMap(role.DefaultAttributes)
			role.OverrideAttributes = cloneRoleEnvMap(role.OverrideAttributes)
			if strings.TrimSpace(role.Source) == "" {
				role.Source = "file"
			}
			s.roles[role.Name] = role
		}
	}
	loadEnvs := func() {
		files, err := filepath.Glob(filepath.Join(s.envDir, "*.json"))
		if err != nil {
			return
		}
		for _, file := range files {
			var env EnvironmentDefinition
			if !readRoleEnvJSON(file, &env) {
				continue
			}
			env.Name = normalizeRoleEnvName(env.Name)
			if env.Name == "" {
				continue
			}
			env.DefaultAttributes = cloneRoleEnvMap(env.DefaultAttributes)
			env.OverrideAttributes = cloneRoleEnvMap(env.OverrideAttributes)
			env.PolicyOverrides = cloneRoleEnvMap(env.PolicyOverrides)
			env.RunListOverrides = normalizeRunListOverrides(env.RunListOverrides)
			if strings.TrimSpace(env.Source) == "" {
				env.Source = "file"
			}
			s.environments[env.Name] = env
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	loadRoles()
	loadEnvs()
}

func writeRoleEnvJSON(path string, v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o644)
}

func readRoleEnvJSON(path string, v any) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func cloneRole(in RoleDefinition) RoleDefinition {
	out := in
	out.Profiles = append([]string{}, in.Profiles...)
	out.RunList = append([]string{}, in.RunList...)
	out.DefaultAttributes = cloneRoleEnvMap(in.DefaultAttributes)
	out.OverrideAttributes = cloneRoleEnvMap(in.OverrideAttributes)
	return out
}

func cloneEnvironment(in EnvironmentDefinition) EnvironmentDefinition {
	out := in
	out.DefaultAttributes = cloneRoleEnvMap(in.DefaultAttributes)
	out.OverrideAttributes = cloneRoleEnvMap(in.OverrideAttributes)
	out.PolicyOverrides = cloneRoleEnvMap(in.PolicyOverrides)
	out.RunListOverrides = normalizeRunListOverrides(in.RunListOverrides)
	return out
}

func normalizeRoleEnvName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeRunList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeRunListOverrides(in map[string][]string) map[string][]string {
	out := map[string][]string{}
	for role, runList := range in {
		roleName := normalizeRoleEnvName(role)
		if roleName == "" {
			continue
		}
		out[roleName] = normalizeRunList(runList)
	}
	return out
}

func normalizeRoleProfiles(profiles []string) []string {
	out := make([]string, 0, len(profiles))
	seen := map[string]struct{}{}
	for _, profile := range profiles {
		name := normalizeRoleEnvName(profile)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

type roleHierarchy struct {
	RunList            []string
	DefaultAttributes  map[string]any
	OverrideAttributes map[string]any
	PolicyGroup        string
	Precedence         []string
}

func (s *RoleEnvironmentStore) resolveRoleHierarchyLocked(name string, visiting map[string]struct{}, cache map[string]roleHierarchy) (roleHierarchy, error) {
	name = normalizeRoleEnvName(name)
	if cached, ok := cache[name]; ok {
		return cloneRoleHierarchy(cached), nil
	}
	if _, ok := visiting[name]; ok {
		return roleHierarchy{}, errors.New("role profile inheritance cycle detected")
	}
	role, ok := s.roles[name]
	if !ok {
		return roleHierarchy{}, errors.New("role not found")
	}
	visiting[name] = struct{}{}

	out := roleHierarchy{
		RunList:            []string{},
		DefaultAttributes:  map[string]any{},
		OverrideAttributes: map[string]any{},
		PolicyGroup:        "",
		Precedence:         []string{},
	}
	for _, parent := range role.Profiles {
		parentResolved, err := s.resolveRoleHierarchyLocked(parent, visiting, cache)
		if err != nil {
			delete(visiting, name)
			return roleHierarchy{}, err
		}
		out.RunList = append(out.RunList, parentResolved.RunList...)
		mergeRoleEnvMap(out.DefaultAttributes, parentResolved.DefaultAttributes)
		mergeRoleEnvMap(out.OverrideAttributes, parentResolved.OverrideAttributes)
		if out.PolicyGroup == "" {
			out.PolicyGroup = strings.TrimSpace(parentResolved.PolicyGroup)
		}
		out.Precedence = append(out.Precedence, parentResolved.Precedence...)
	}
	out.RunList = append(out.RunList, role.RunList...)
	mergeRoleEnvMap(out.DefaultAttributes, role.DefaultAttributes)
	mergeRoleEnvMap(out.OverrideAttributes, role.OverrideAttributes)
	if strings.TrimSpace(role.PolicyGroup) != "" {
		out.PolicyGroup = strings.TrimSpace(role.PolicyGroup)
	}
	out.Precedence = append(out.Precedence,
		"role["+role.Name+"].default_attributes",
		"role["+role.Name+"].override_attributes",
	)

	cache[name] = cloneRoleHierarchy(out)
	delete(visiting, name)
	return out, nil
}

func cloneRoleHierarchy(in roleHierarchy) roleHierarchy {
	out := in
	out.RunList = append([]string{}, in.RunList...)
	out.DefaultAttributes = cloneRoleEnvMap(in.DefaultAttributes)
	out.OverrideAttributes = cloneRoleEnvMap(in.OverrideAttributes)
	out.Precedence = append([]string{}, in.Precedence...)
	return out
}

func cloneRoleEnvMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	buf, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(buf, &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func cloneRoleEnvAny(v any) any {
	buf, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(buf, &out)
	return out
}

func mergeRoleEnvMap(dst, src map[string]any) {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			existingMap, okExisting := existing.(map[string]any)
			srcMap, okSrc := v.(map[string]any)
			if okExisting && okSrc {
				mergeRoleEnvMap(existingMap, srcMap)
				dst[k] = existingMap
				continue
			}
		}
		dst[k] = cloneRoleEnvAny(v)
	}
}
