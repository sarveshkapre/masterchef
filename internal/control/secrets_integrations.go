package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SecretsIntegration struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"` // vault|aws_sm|gcp_sm|azure_kv|inline
	Config    map[string]string `json:"config,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type SecretsIntegrationInput struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config,omitempty"`
	Enabled  *bool             `json:"enabled,omitempty"`
}

type SecretResolveInput struct {
	IntegrationID string `json:"integration_id"`
	Path          string `json:"path"`
	Version       string `json:"version,omitempty"`
	UsedBy        string `json:"used_by,omitempty"`
}

type SecretResolveResult struct {
	IntegrationID string    `json:"integration_id"`
	Path          string    `json:"path"`
	Version       string    `json:"version,omitempty"`
	Value         string    `json:"value"`
	ResolvedAt    time.Time `json:"resolved_at"`
}

type SecretUsageTrace struct {
	ID            string    `json:"id"`
	IntegrationID string    `json:"integration_id"`
	Path          string    `json:"path"`
	Version       string    `json:"version,omitempty"`
	UsedBy        string    `json:"used_by,omitempty"`
	RedactedValue string    `json:"redacted_value"`
	ResolvedAt    time.Time `json:"resolved_at"`
}

type SecretsIntegrationStore struct {
	mu              sync.RWMutex
	nextIntegration int64
	nextTrace       int64
	integrations    map[string]*SecretsIntegration
	secrets         map[string]map[string]string
	traces          []SecretUsageTrace
}

func NewSecretsIntegrationStore() *SecretsIntegrationStore {
	return &SecretsIntegrationStore{
		integrations: map[string]*SecretsIntegration{},
		secrets:      map[string]map[string]string{},
		traces:       make([]SecretUsageTrace, 0, 128),
	}
}

func (s *SecretsIntegrationStore) Upsert(in SecretsIntegrationInput) (SecretsIntegration, error) {
	name := strings.TrimSpace(in.Name)
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if name == "" || provider == "" {
		return SecretsIntegration{}, errors.New("name and provider are required")
	}
	switch provider {
	case "vault", "aws_sm", "gcp_sm", "azure_kv", "inline":
	default:
		return SecretsIntegration{}, errors.New("provider must be one of vault, aws_sm, gcp_sm, azure_kv, inline")
	}

	cfg := map[string]string{}
	for k, v := range in.Config {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		cfg[key] = strings.TrimSpace(v)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.integrations {
		if strings.EqualFold(existing.Name, name) {
			existing.Provider = provider
			existing.Config = cfg
			existing.Enabled = enabled
			existing.UpdatedAt = now
			s.secrets[existing.ID] = extractInlineSecrets(cfg)
			return cloneSecretsIntegration(*existing), nil
		}
	}

	s.nextIntegration++
	item := SecretsIntegration{
		ID:        "secret-integration-" + itoa(s.nextIntegration),
		Name:      name,
		Provider:  provider,
		Config:    cfg,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.integrations[item.ID] = &item
	s.secrets[item.ID] = extractInlineSecrets(cfg)
	return cloneSecretsIntegration(item), nil
}

func (s *SecretsIntegrationStore) List() []SecretsIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SecretsIntegration, 0, len(s.integrations))
	for _, item := range s.integrations {
		out = append(out, cloneSecretsIntegration(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *SecretsIntegrationStore) Resolve(in SecretResolveInput) (SecretResolveResult, error) {
	integrationID := strings.TrimSpace(in.IntegrationID)
	path := strings.TrimSpace(in.Path)
	if integrationID == "" || path == "" {
		return SecretResolveResult{}, errors.New("integration_id and path are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.integrations[integrationID]
	if !ok {
		return SecretResolveResult{}, errors.New("secret integration not found")
	}
	if !item.Enabled {
		return SecretResolveResult{}, errors.New("secret integration is disabled")
	}
	secrets := s.secrets[integrationID]
	value, ok := secrets[path]
	if !ok {
		return SecretResolveResult{}, errors.New("secret path not found")
	}
	result := SecretResolveResult{
		IntegrationID: integrationID,
		Path:          path,
		Version:       strings.TrimSpace(in.Version),
		Value:         value,
		ResolvedAt:    now,
	}
	s.nextTrace++
	s.traces = append(s.traces, SecretUsageTrace{
		ID:            "secret-trace-" + itoa(s.nextTrace),
		IntegrationID: integrationID,
		Path:          path,
		Version:       result.Version,
		UsedBy:        strings.TrimSpace(in.UsedBy),
		RedactedValue: "<redacted>",
		ResolvedAt:    now,
	})
	return result, nil
}

func (s *SecretsIntegrationStore) ListUsageTraces(limit int) []SecretUsageTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 200
	}
	if limit > len(s.traces) {
		limit = len(s.traces)
	}
	out := make([]SecretUsageTrace, 0, limit)
	for i := len(s.traces) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.traces[i])
	}
	return out
}

func extractInlineSecrets(cfg map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range cfg {
		key := strings.TrimSpace(k)
		if strings.HasPrefix(key, "secret.") {
			path := strings.TrimPrefix(key, "secret.")
			if path == "" {
				continue
			}
			out[path] = v
		}
	}
	return out
}

func cloneSecretsIntegration(in SecretsIntegration) SecretsIntegration {
	out := in
	out.Config = map[string]string{}
	for k, v := range in.Config {
		out.Config[k] = v
	}
	return out
}
