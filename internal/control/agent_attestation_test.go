package control

import (
	"strings"
	"testing"
	"time"
)

func TestAgentAttestationSetPolicyValidation(t *testing.T) {
	store := NewAgentAttestationStore()
	policy := store.SetPolicy(AgentAttestationPolicy{
		RequireBeforeCert: true,
		AllowedProviders:  []string{"bad", "TPM", "aws_iid"},
		MaxAgeMinutes:     -5,
	})
	if !policy.RequireBeforeCert {
		t.Fatalf("expected require_before_cert=true")
	}
	if policy.MaxAgeMinutes != 60 {
		t.Fatalf("expected default max_age_minutes=60, got %d", policy.MaxAgeMinutes)
	}
	if len(policy.AllowedProviders) != 2 {
		t.Fatalf("expected 2 allowed providers, got %v", policy.AllowedProviders)
	}
}

func TestAgentAttestationSubmitAndCheckForCertificate(t *testing.T) {
	store := NewAgentAttestationStore()
	store.SetPolicy(AgentAttestationPolicy{
		RequireBeforeCert: true,
		AllowedProviders:  []string{"tpm"},
		MaxAgeMinutes:     30,
	})

	denied := store.CheckForCertificate("agent-1")
	if denied.Allowed || denied.Reason == "" {
		t.Fatalf("expected denied pre-check, got %+v", denied)
	}

	evidence, err := store.Submit(AgentAttestationInput{
		AgentID:  "agent-1",
		Provider: "tpm",
		Nonce:    "nonce-1",
		Claims: map[string]string{
			"pcr0": "abcd",
		},
	})
	if err != nil {
		t.Fatalf("submit attestation failed: %v", err)
	}
	if !evidence.Verified {
		t.Fatalf("expected verified evidence, got %+v", evidence)
	}
	if !strings.HasPrefix(evidence.EvidenceHash, "sha256:") {
		t.Fatalf("expected sha256 evidence hash, got %q", evidence.EvidenceHash)
	}

	allowed := store.CheckForCertificate("agent-1")
	if !allowed.Allowed {
		t.Fatalf("expected allowed check, got %+v", allowed)
	}

	unverified, err := store.Submit(AgentAttestationInput{
		AgentID:  "agent-2",
		Provider: "azure_imds",
		Nonce:    "nonce-2",
	})
	if err != nil {
		t.Fatalf("submit disallowed provider failed: %v", err)
	}
	if unverified.Verified {
		t.Fatalf("expected unverified evidence for disallowed provider")
	}
	if check := store.CheckForCertificate("agent-2"); check.Allowed {
		t.Fatalf("expected denied check for agent-2, got %+v", check)
	}
}

func TestAgentAttestationExpiryPruning(t *testing.T) {
	store := NewAgentAttestationStore()
	store.SetPolicy(AgentAttestationPolicy{
		RequireBeforeCert: true,
		AllowedProviders:  []string{"tpm"},
		MaxAgeMinutes:     60,
	})
	evidence, err := store.Submit(AgentAttestationInput{
		AgentID:  "agent-exp",
		Provider: "tpm",
		Nonce:    "nonce-exp",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	store.mu.Lock()
	item := store.evidence[evidence.ID]
	item.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	store.mu.Unlock()

	if check := store.CheckForCertificate("agent-exp"); check.Allowed {
		t.Fatalf("expected check denied after expiry, got %+v", check)
	}
	if _, ok := store.Get(evidence.ID); ok {
		t.Fatalf("expected evidence pruned after expiry")
	}
}
