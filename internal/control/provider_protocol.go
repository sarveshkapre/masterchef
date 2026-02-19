package control

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProviderProtocolDescriptor struct {
	ID                   string          `json:"id"`
	Provider             string          `json:"provider"`
	ProtocolVersion      string          `json:"protocol_version"`
	MinControllerVersion string          `json:"min_controller_version"`
	MaxControllerVersion string          `json:"max_controller_version"`
	Capabilities         []string        `json:"capabilities"`
	FeatureFlags         map[string]bool `json:"feature_flags,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type ProviderNegotiationInput struct {
	Provider              string   `json:"provider"`
	ControllerVersion     string   `json:"controller_version"`
	RequestedCapabilities []string `json:"requested_capabilities,omitempty"`
	RequiredFeatureFlags  []string `json:"required_feature_flags,omitempty"`
}

type ProviderNegotiationResult struct {
	Provider               string   `json:"provider"`
	ControllerVersion      string   `json:"controller_version"`
	ProtocolVersion        string   `json:"protocol_version"`
	Compatible             bool     `json:"compatible"`
	NegotiatedCapabilities []string `json:"negotiated_capabilities,omitempty"`
	MissingCapabilities    []string `json:"missing_capabilities,omitempty"`
	UnsupportedFlags       []string `json:"unsupported_flags,omitempty"`
	Reason                 string   `json:"reason,omitempty"`
}

type ProviderProtocolStore struct {
	mu          sync.RWMutex
	descriptors map[string]*ProviderProtocolDescriptor
}

func NewProviderProtocolStore() *ProviderProtocolStore {
	s := &ProviderProtocolStore{
		descriptors: map[string]*ProviderProtocolDescriptor{},
	}
	defaults := []ProviderProtocolDescriptor{
		{
			ID:                   "provider-protocol-file-v1",
			Provider:             "file",
			ProtocolVersion:      "v1.2",
			MinControllerVersion: "v1.0",
			MaxControllerVersion: "v2.0",
			Capabilities:         []string{"idempotent_apply", "drift_detect", "rollback"},
			FeatureFlags: map[string]bool{
				"signed-content": true,
				"strict-mode":    true,
			},
		},
		{
			ID:                   "provider-protocol-package-v1",
			Provider:             "package",
			ProtocolVersion:      "v1.1",
			MinControllerVersion: "v1.0",
			MaxControllerVersion: "v2.0",
			Capabilities:         []string{"version_pin", "rollback", "hold_unhold"},
			FeatureFlags: map[string]bool{
				"canary-eval": true,
			},
		},
	}
	for _, item := range defaults {
		_, _ = s.UpsertDescriptor(item)
	}
	return s
}

func (s *ProviderProtocolStore) UpsertDescriptor(in ProviderProtocolDescriptor) (ProviderProtocolDescriptor, error) {
	id := strings.TrimSpace(in.ID)
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if id == "" || provider == "" {
		return ProviderProtocolDescriptor{}, errors.New("id and provider are required")
	}
	if strings.TrimSpace(in.ProtocolVersion) == "" || strings.TrimSpace(in.MinControllerVersion) == "" || strings.TrimSpace(in.MaxControllerVersion) == "" {
		return ProviderProtocolDescriptor{}, errors.New("protocol_version and controller version bounds are required")
	}
	caps := normalizeStringSlice(in.Capabilities)
	if len(caps) == 0 {
		return ProviderProtocolDescriptor{}, errors.New("capabilities are required")
	}
	flags := cloneBoolMap(in.FeatureFlags)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.descriptors[provider]
	if !ok {
		out := ProviderProtocolDescriptor{
			ID:                   id,
			Provider:             provider,
			ProtocolVersion:      strings.TrimSpace(in.ProtocolVersion),
			MinControllerVersion: strings.TrimSpace(in.MinControllerVersion),
			MaxControllerVersion: strings.TrimSpace(in.MaxControllerVersion),
			Capabilities:         caps,
			FeatureFlags:         flags,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		s.descriptors[provider] = &out
		return cloneProviderProtocolDescriptor(out), nil
	}
	item.ID = id
	item.Provider = provider
	item.ProtocolVersion = strings.TrimSpace(in.ProtocolVersion)
	item.MinControllerVersion = strings.TrimSpace(in.MinControllerVersion)
	item.MaxControllerVersion = strings.TrimSpace(in.MaxControllerVersion)
	item.Capabilities = caps
	item.FeatureFlags = flags
	item.UpdatedAt = now
	return cloneProviderProtocolDescriptor(*item), nil
}

func (s *ProviderProtocolStore) ListDescriptors() []ProviderProtocolDescriptor {
	s.mu.RLock()
	out := make([]ProviderProtocolDescriptor, 0, len(s.descriptors))
	for _, item := range s.descriptors {
		out = append(out, cloneProviderProtocolDescriptor(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

func (s *ProviderProtocolStore) Negotiate(in ProviderNegotiationInput) (ProviderNegotiationResult, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	ctrlVersion := strings.TrimSpace(in.ControllerVersion)
	if provider == "" || ctrlVersion == "" {
		return ProviderNegotiationResult{}, errors.New("provider and controller_version are required")
	}
	s.mu.RLock()
	descriptor, ok := s.descriptors[provider]
	s.mu.RUnlock()
	if !ok {
		return ProviderNegotiationResult{}, errors.New("provider protocol descriptor not found")
	}
	if !versionWithin(ctrlVersion, descriptor.MinControllerVersion, descriptor.MaxControllerVersion) {
		return ProviderNegotiationResult{
			Provider:          provider,
			ControllerVersion: ctrlVersion,
			ProtocolVersion:   descriptor.ProtocolVersion,
			Compatible:        false,
			Reason:            "controller version outside provider protocol compatibility range",
		}, nil
	}

	capSet := make(map[string]struct{}, len(descriptor.Capabilities))
	for _, capItem := range descriptor.Capabilities {
		capSet[strings.ToLower(capItem)] = struct{}{}
	}
	requested := normalizeStringSlice(in.RequestedCapabilities)
	negotiated := make([]string, 0)
	missingCaps := make([]string, 0)
	for _, capItem := range requested {
		if _, ok := capSet[strings.ToLower(capItem)]; ok {
			negotiated = append(negotiated, capItem)
		} else {
			missingCaps = append(missingCaps, capItem)
		}
	}

	reqFlags := normalizeStringSlice(in.RequiredFeatureFlags)
	unsupported := make([]string, 0)
	for _, flag := range reqFlags {
		if !descriptor.FeatureFlags[flag] {
			unsupported = append(unsupported, flag)
		}
	}

	compatible := len(missingCaps) == 0 && len(unsupported) == 0
	reason := ""
	if !compatible {
		reason = "provider capability/feature negotiation failed"
	}
	return ProviderNegotiationResult{
		Provider:               provider,
		ControllerVersion:      ctrlVersion,
		ProtocolVersion:        descriptor.ProtocolVersion,
		Compatible:             compatible,
		NegotiatedCapabilities: negotiated,
		MissingCapabilities:    missingCaps,
		UnsupportedFlags:       unsupported,
		Reason:                 reason,
	}, nil
}

func cloneProviderProtocolDescriptor(in ProviderProtocolDescriptor) ProviderProtocolDescriptor {
	in.Capabilities = cloneStringSlice(in.Capabilities)
	in.FeatureFlags = cloneBoolMap(in.FeatureFlags)
	return in
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func versionWithin(current, minVersion, maxVersion string) bool {
	return compareVersion(current, minVersion) >= 0 && compareVersion(current, maxVersion) <= 0
}

func compareVersion(a, b string) int {
	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			n, _ := strconv.Atoi(part)
			out[i] = n
		}
		return out
	}
	av := parse(a)
	bv := parse(b)
	l := len(av)
	if len(bv) > l {
		l = len(bv)
	}
	for i := 0; i < l; i++ {
		ai := 0
		if i < len(av) {
			ai = av[i]
		}
		bi := 0
		if i < len(bv) {
			bi = bv[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}
