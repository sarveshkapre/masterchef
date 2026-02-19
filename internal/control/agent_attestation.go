package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AgentAttestationPolicy struct {
	RequireBeforeCert bool      `json:"require_before_cert"`
	AllowedProviders  []string  `json:"allowed_providers,omitempty"`
	MaxAgeMinutes     int       `json:"max_age_minutes"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AgentAttestationInput struct {
	AgentID  string            `json:"agent_id"`
	Provider string            `json:"provider"` // tpm|aws_iid|gcp_shielded|azure_imds
	Nonce    string            `json:"nonce"`
	Claims   map[string]string `json:"claims,omitempty"`
}

type AgentAttestationEvidence struct {
	ID           string            `json:"id"`
	AgentID      string            `json:"agent_id"`
	Provider     string            `json:"provider"`
	EvidenceHash string            `json:"evidence_hash"`
	Claims       map[string]string `json:"claims,omitempty"`
	Verified     bool              `json:"verified"`
	Reason       string            `json:"reason,omitempty"`
	CollectedAt  time.Time         `json:"collected_at"`
	ExpiresAt    time.Time         `json:"expires_at"`
}

type AgentAttestationCheck struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type AgentAttestationStore struct {
	mu       sync.RWMutex
	nextID   int64
	policy   AgentAttestationPolicy
	evidence map[string]*AgentAttestationEvidence
}

func NewAgentAttestationStore() *AgentAttestationStore {
	return &AgentAttestationStore{
		policy: AgentAttestationPolicy{
			RequireBeforeCert: false,
			AllowedProviders:  []string{"tpm", "aws_iid", "gcp_shielded", "azure_imds"},
			MaxAgeMinutes:     60,
			UpdatedAt:         time.Now().UTC(),
		},
		evidence: map[string]*AgentAttestationEvidence{},
	}
}

func (s *AgentAttestationStore) Policy() AgentAttestationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAttestationPolicy(s.policy)
}

func (s *AgentAttestationStore) SetPolicy(policy AgentAttestationPolicy) AgentAttestationPolicy {
	allowed := make([]string, 0, len(policy.AllowedProviders))
	for _, item := range normalizeStringSlice(policy.AllowedProviders) {
		switch item {
		case "tpm", "aws_iid", "gcp_shielded", "azure_imds":
			allowed = append(allowed, item)
		}
	}
	if len(allowed) == 0 {
		allowed = []string{"tpm", "aws_iid", "gcp_shielded", "azure_imds"}
	}
	maxAge := policy.MaxAgeMinutes
	if maxAge <= 0 {
		maxAge = 60
	}
	if maxAge > 24*60 {
		maxAge = 24 * 60
	}
	item := AgentAttestationPolicy{
		RequireBeforeCert: policy.RequireBeforeCert,
		AllowedProviders:  allowed,
		MaxAgeMinutes:     maxAge,
		UpdatedAt:         time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = item
	s.mu.Unlock()
	return cloneAttestationPolicy(item)
}

func (s *AgentAttestationStore) Submit(in AgentAttestationInput) (AgentAttestationEvidence, error) {
	agentID := strings.TrimSpace(in.AgentID)
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	nonce := strings.TrimSpace(in.Nonce)
	if agentID == "" || provider == "" || nonce == "" {
		return AgentAttestationEvidence{}, errors.New("agent_id, provider, and nonce are required")
	}
	claims := map[string]string{}
	for k, v := range in.Claims {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		claims[key] = strings.TrimSpace(v)
	}
	now := time.Now().UTC()
	hashInput := agentID + "|" + provider + "|" + nonce
	sum := sha256.Sum256([]byte(hashInput))
	evidenceHash := "sha256:" + hex.EncodeToString(sum[:])

	policy := s.Policy()
	verified := false
	reason := ""
	if providerAllowed(provider, policy.AllowedProviders) {
		verified = true
	} else {
		reason = "provider not allowed by policy"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := AgentAttestationEvidence{
		ID:           "attestation-" + itoa(s.nextID),
		AgentID:      agentID,
		Provider:     provider,
		EvidenceHash: evidenceHash,
		Claims:       claims,
		Verified:     verified,
		Reason:       reason,
		CollectedAt:  now,
		ExpiresAt:    now.Add(time.Duration(policy.MaxAgeMinutes) * time.Minute),
	}
	s.evidence[item.ID] = &item
	return cloneAttestationEvidence(item), nil
}

func (s *AgentAttestationStore) List() []AgentAttestationEvidence {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireLocked(now)
	out := make([]AgentAttestationEvidence, 0, len(s.evidence))
	for _, item := range s.evidence {
		out = append(out, cloneAttestationEvidence(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CollectedAt.After(out[j].CollectedAt) })
	return out
}

func (s *AgentAttestationStore) Get(id string) (AgentAttestationEvidence, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	item, ok := s.evidence[strings.TrimSpace(id)]
	if !ok {
		return AgentAttestationEvidence{}, false
	}
	return cloneAttestationEvidence(*item), true
}

func (s *AgentAttestationStore) CheckForCertificate(agentID string) AgentAttestationCheck {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentAttestationCheck{Allowed: false, Reason: "agent_id is required"}
	}
	policy := s.Policy()
	if !policy.RequireBeforeCert {
		return AgentAttestationCheck{Allowed: true}
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	var latest *AgentAttestationEvidence
	for _, item := range s.evidence {
		if item.AgentID != agentID || !item.Verified {
			continue
		}
		if latest == nil || item.CollectedAt.After(latest.CollectedAt) {
			cp := *item
			latest = &cp
		}
	}
	if latest == nil {
		return AgentAttestationCheck{Allowed: false, Reason: "no verified attestation found"}
	}
	if !now.Before(latest.ExpiresAt) {
		return AgentAttestationCheck{Allowed: false, Reason: "latest attestation expired"}
	}
	return AgentAttestationCheck{Allowed: true}
}

func (s *AgentAttestationStore) expireLocked(now time.Time) {
	for id, item := range s.evidence {
		if !now.Before(item.ExpiresAt) {
			delete(s.evidence, id)
		}
	}
}

func providerAllowed(provider string, allowed []string) bool {
	for _, item := range allowed {
		if provider == item {
			return true
		}
	}
	return false
}

func cloneAttestationPolicy(in AgentAttestationPolicy) AgentAttestationPolicy {
	out := in
	out.AllowedProviders = append([]string{}, in.AllowedProviders...)
	return out
}

func cloneAttestationEvidence(in AgentAttestationEvidence) AgentAttestationEvidence {
	out := in
	out.Claims = map[string]string{}
	for k, v := range in.Claims {
		out.Claims[k] = v
	}
	return out
}
