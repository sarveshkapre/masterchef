package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProviderSandboxProfileInput struct {
	Provider        string   `json:"provider"`
	Runtime         string   `json:"runtime"` // wasi|native
	Capabilities    []string `json:"capabilities,omitempty"`
	FilesystemScope []string `json:"filesystem_scope,omitempty"`
	NetworkScope    []string `json:"network_scope,omitempty"`
	AllowHostAccess bool     `json:"allow_host_access,omitempty"`
}

type ProviderSandboxProfile struct {
	Provider        string    `json:"provider"`
	Runtime         string    `json:"runtime"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	FilesystemScope []string  `json:"filesystem_scope,omitempty"`
	NetworkScope    []string  `json:"network_scope,omitempty"`
	AllowHostAccess bool      `json:"allow_host_access"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ProviderSandboxEvaluateInput struct {
	Provider             string   `json:"provider"`
	Untrusted            bool     `json:"untrusted"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	RequireFilesystem    bool     `json:"require_filesystem,omitempty"`
	RequireNetwork       bool     `json:"require_network,omitempty"`
}

type ProviderSandboxEvaluation struct {
	Allowed             bool     `json:"allowed"`
	Provider            string   `json:"provider"`
	Runtime             string   `json:"runtime,omitempty"`
	MissingCapabilities []string `json:"missing_capabilities,omitempty"`
	Violations          []string `json:"violations,omitempty"`
	Reason              string   `json:"reason"`
}

type ProviderSandboxStore struct {
	mu       sync.RWMutex
	profiles map[string]*ProviderSandboxProfile
}

func NewProviderSandboxStore() *ProviderSandboxStore {
	return &ProviderSandboxStore{profiles: map[string]*ProviderSandboxProfile{}}
}

func (s *ProviderSandboxStore) UpsertProfile(in ProviderSandboxProfileInput) (ProviderSandboxProfile, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider == "" {
		return ProviderSandboxProfile{}, errors.New("provider is required")
	}
	runtime := strings.ToLower(strings.TrimSpace(in.Runtime))
	if runtime == "" {
		runtime = "wasi"
	}
	if runtime != "wasi" && runtime != "native" {
		return ProviderSandboxProfile{}, errors.New("runtime must be wasi or native")
	}
	item := ProviderSandboxProfile{
		Provider:        provider,
		Runtime:         runtime,
		Capabilities:    normalizeStringList(in.Capabilities),
		FilesystemScope: normalizeStringList(in.FilesystemScope),
		NetworkScope:    normalizeStringList(in.NetworkScope),
		AllowHostAccess: in.AllowHostAccess,
		UpdatedAt:       time.Now().UTC(),
	}
	s.mu.Lock()
	s.profiles[provider] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *ProviderSandboxStore) ListProfiles() []ProviderSandboxProfile {
	s.mu.RLock()
	out := make([]ProviderSandboxProfile, 0, len(s.profiles))
	for _, item := range s.profiles {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

func (s *ProviderSandboxStore) GetProfile(provider string) (ProviderSandboxProfile, bool) {
	s.mu.RLock()
	item, ok := s.profiles[strings.ToLower(strings.TrimSpace(provider))]
	s.mu.RUnlock()
	if !ok {
		return ProviderSandboxProfile{}, false
	}
	return *item, true
}

func (s *ProviderSandboxStore) Evaluate(in ProviderSandboxEvaluateInput) ProviderSandboxEvaluation {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider == "" {
		return ProviderSandboxEvaluation{Allowed: false, Reason: "provider is required"}
	}
	profile, ok := s.GetProfile(provider)
	if !ok {
		return ProviderSandboxEvaluation{
			Allowed:  false,
			Provider: provider,
			Reason:   "provider sandbox profile not found",
		}
	}
	missing := make([]string, 0)
	requiredCaps := normalizeStringList(in.RequiredCapabilities)
	if len(requiredCaps) > 0 {
		actual := map[string]struct{}{}
		for _, capability := range profile.Capabilities {
			actual[capability] = struct{}{}
		}
		for _, capability := range requiredCaps {
			if _, ok := actual[capability]; !ok {
				missing = append(missing, capability)
			}
		}
	}
	violations := make([]string, 0)
	if in.Untrusted && profile.Runtime != "wasi" {
		violations = append(violations, "untrusted provider must run in wasi sandbox")
	}
	if in.Untrusted && profile.AllowHostAccess {
		violations = append(violations, "untrusted provider cannot have direct host access")
	}
	if in.RequireFilesystem && len(profile.FilesystemScope) == 0 {
		violations = append(violations, "filesystem scope is required but not configured")
	}
	if in.RequireNetwork && len(profile.NetworkScope) == 0 {
		violations = append(violations, "network scope is required but not configured")
	}
	allowed := len(missing) == 0 && len(violations) == 0
	reason := "provider sandbox profile satisfies least-privilege policy"
	if !allowed {
		reason = "provider sandbox policy violations detected"
	}
	return ProviderSandboxEvaluation{
		Allowed:             allowed,
		Provider:            provider,
		Runtime:             profile.Runtime,
		MissingCapabilities: missing,
		Violations:          violations,
		Reason:              reason,
	}
}
