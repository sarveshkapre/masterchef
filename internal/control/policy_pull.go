package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	PolicyPullSourceTypeControlPlane = "control_plane"
	PolicyPullSourceTypeGitSigned    = "git_signed"
)

type PolicyPullSourceInput struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	URL              string `json:"url,omitempty"`
	Branch           string `json:"branch,omitempty"`
	PublicKeyPath    string `json:"public_key_path,omitempty"`
	RequireSignature bool   `json:"require_signature,omitempty"`
	Enabled          bool   `json:"enabled"`
}

type PolicyPullSource struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	URL              string    `json:"url,omitempty"`
	Branch           string    `json:"branch,omitempty"`
	PublicKeyPath    string    `json:"public_key_path,omitempty"`
	RequireSignature bool      `json:"require_signature,omitempty"`
	Enabled          bool      `json:"enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PolicyBundle struct {
	ConfigPath string `json:"config_path"`
	ConfigSHA  string `json:"config_sha"`
	Signature  string `json:"signature,omitempty"`
}

type PolicyPullExecuteRequest struct {
	SourceID   string       `json:"source_id"`
	ConfigPath string       `json:"config_path,omitempty"`
	Revision   string       `json:"revision,omitempty"`
	Bundle     PolicyBundle `json:"bundle"`
}

type PolicyPullResult struct {
	ID               string    `json:"id"`
	SourceID         string    `json:"source_id"`
	SourceType       string    `json:"source_type"`
	Environment      string    `json:"environment,omitempty"`
	Revision         string    `json:"revision,omitempty"`
	ConfigPath       string    `json:"config_path,omitempty"`
	ConfigSHA        string    `json:"config_sha,omitempty"`
	Status           string    `json:"status"`
	Verified         bool      `json:"verified"`
	Message          string    `json:"message,omitempty"`
	PulledAt         time.Time `json:"pulled_at"`
	RequireSignature bool      `json:"require_signature,omitempty"`
}

type PolicyPullResultInput struct {
	SourceID         string
	SourceType       string
	Environment      string
	Revision         string
	ConfigPath       string
	ConfigSHA        string
	Status           string
	Verified         bool
	Message          string
	RequireSignature bool
}

type PolicyPullStore struct {
	mu         sync.RWMutex
	nextSource int64
	nextResult int64
	sources    map[string]*PolicyPullSource
	results    []PolicyPullResult
}

func NewPolicyPullStore() *PolicyPullStore {
	return &PolicyPullStore{
		sources: map[string]*PolicyPullSource{},
		results: make([]PolicyPullResult, 0, 512),
	}
}

func (s *PolicyPullStore) CreateSource(in PolicyPullSourceInput) (PolicyPullSource, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return PolicyPullSource{}, errors.New("name is required")
	}
	typeName := strings.ToLower(strings.TrimSpace(in.Type))
	switch typeName {
	case PolicyPullSourceTypeControlPlane, PolicyPullSourceTypeGitSigned:
	default:
		return PolicyPullSource{}, errors.New("type must be control_plane or git_signed")
	}
	url := strings.TrimSpace(in.URL)
	branch := strings.TrimSpace(in.Branch)
	pubPath := strings.TrimSpace(in.PublicKeyPath)
	requireSig := in.RequireSignature
	if typeName == PolicyPullSourceTypeControlPlane {
		url = ""
		branch = ""
		requireSig = false
		pubPath = ""
	} else {
		if url == "" {
			return PolicyPullSource{}, errors.New("url is required for git_signed source")
		}
		if branch == "" {
			branch = "main"
		}
		if requireSig && pubPath == "" {
			return PolicyPullSource{}, errors.New("public_key_path is required when require_signature=true")
		}
	}
	item := PolicyPullSource{
		Name:             name,
		Type:             typeName,
		URL:              url,
		Branch:           branch,
		PublicKeyPath:    pubPath,
		RequireSignature: requireSig,
		Enabled:          in.Enabled,
		UpdatedAt:        time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextSource++
	item.ID = "policy-source-" + itoa(s.nextSource)
	s.sources[item.ID] = &item
	s.mu.Unlock()
	return clonePolicyPullSource(item), nil
}

func (s *PolicyPullStore) ListSources() []PolicyPullSource {
	s.mu.RLock()
	out := make([]PolicyPullSource, 0, len(s.sources))
	for _, item := range s.sources {
		out = append(out, clonePolicyPullSource(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *PolicyPullStore) GetSource(id string) (PolicyPullSource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.sources[strings.TrimSpace(id)]
	if !ok {
		return PolicyPullSource{}, false
	}
	return clonePolicyPullSource(*item), true
}

func (s *PolicyPullStore) RecordResult(in PolicyPullResultInput) PolicyPullResult {
	item := PolicyPullResult{
		SourceID:         strings.TrimSpace(in.SourceID),
		SourceType:       strings.TrimSpace(in.SourceType),
		Environment:      strings.TrimSpace(in.Environment),
		Revision:         strings.TrimSpace(in.Revision),
		ConfigPath:       strings.TrimSpace(in.ConfigPath),
		ConfigSHA:        strings.TrimSpace(in.ConfigSHA),
		Status:           strings.TrimSpace(in.Status),
		Verified:         in.Verified,
		Message:          strings.TrimSpace(in.Message),
		PulledAt:         time.Now().UTC(),
		RequireSignature: in.RequireSignature,
	}
	s.mu.Lock()
	s.nextResult++
	item.ID = "policy-pull-" + itoa(s.nextResult)
	s.results = append(s.results, item)
	if len(s.results) > 3000 {
		s.results = s.results[len(s.results)-3000:]
	}
	s.mu.Unlock()
	return item
}

func (s *PolicyPullStore) ListResults(limit int) []PolicyPullResult {
	s.mu.RLock()
	out := append([]PolicyPullResult{}, s.results...)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].PulledAt.After(out[j].PulledAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func clonePolicyPullSource(in PolicyPullSource) PolicyPullSource {
	return in
}
