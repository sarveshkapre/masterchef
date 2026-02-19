package control

import (
	"testing"
	"time"
)

func TestDelegationTokenLifecycle(t *testing.T) {
	store := NewDelegationTokenStore()
	issued, err := store.Issue(DelegationTokenIssueInput{
		Grantor:    "platform-admin",
		Delegatee:  "pipeline:release-42",
		PipelineID: "release-42",
		Scopes:     []string{"run:apply", "run:approve"},
		TTLSeconds: 120,
		MaxUses:    2,
	})
	if err != nil {
		t.Fatalf("issue delegation token failed: %v", err)
	}
	if issued.Token == "" || issued.Delegation.ID == "" {
		t.Fatalf("expected token and delegation id")
	}

	first := store.validateAt(DelegationTokenValidationInput{
		Token:         issued.Token,
		RequiredScope: "run:apply",
	}, issued.Delegation.CreatedAt.Add(10*time.Second))
	if !first.Allowed || first.UsesRemaining != 1 {
		t.Fatalf("expected first validation to pass: %+v", first)
	}

	second := store.validateAt(DelegationTokenValidationInput{
		Token:         issued.Token,
		RequiredScope: "run:approve",
	}, issued.Delegation.CreatedAt.Add(11*time.Second))
	if !second.Allowed || second.UsesRemaining != 0 {
		t.Fatalf("expected second validation to pass: %+v", second)
	}

	exhausted := store.validateAt(DelegationTokenValidationInput{
		Token:         issued.Token,
		RequiredScope: "run:apply",
	}, issued.Delegation.CreatedAt.Add(12*time.Second))
	if exhausted.Allowed {
		t.Fatalf("expected exhausted token to fail")
	}

	revoked, err := store.Revoke(issued.Delegation.ID)
	if err != nil {
		t.Fatalf("revoke token failed: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected revoked timestamp")
	}
}

func TestDelegationTokenIssueValidation(t *testing.T) {
	store := NewDelegationTokenStore()
	if _, err := store.Issue(DelegationTokenIssueInput{Grantor: "", Delegatee: "x", Scopes: []string{"run:apply"}}); err == nil {
		t.Fatalf("expected missing grantor to fail")
	}
	if _, err := store.Issue(DelegationTokenIssueInput{Grantor: "a", Delegatee: "x", TTLSeconds: 10, Scopes: []string{"run:apply"}}); err == nil {
		t.Fatalf("expected too-low ttl to fail")
	}
	if _, err := store.Issue(DelegationTokenIssueInput{Grantor: "a", Delegatee: "x", TTLSeconds: 120, Scopes: nil}); err == nil {
		t.Fatalf("expected missing scopes to fail")
	}
}
