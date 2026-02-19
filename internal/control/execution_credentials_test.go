package control

import (
	"testing"
	"time"
)

func TestExecutionCredentialLifecycle(t *testing.T) {
	store := NewExecutionCredentialStore()
	issued, err := store.Issue(ExecutionCredentialIssueInput{
		Subject:    "worker@staging",
		Scopes:     []string{"run:execute", "artifact:read"},
		TTLSeconds: 120,
	})
	if err != nil {
		t.Fatalf("issue credential failed: %v", err)
	}
	if issued.Credential.ID == "" || issued.Token == "" {
		t.Fatalf("expected issued credential id and token")
	}

	ok := store.validateAt(ExecutionCredentialValidationInput{
		Token:          issued.Token,
		RequiredScopes: []string{"run:execute"},
	}, issued.Credential.IssuedAt.Add(30*time.Second))
	if !ok.Allowed {
		t.Fatalf("expected issued credential to validate: %+v", ok)
	}

	missingScope := store.validateAt(ExecutionCredentialValidationInput{
		Token:          issued.Token,
		RequiredScopes: []string{"admin:all"},
	}, issued.Credential.IssuedAt.Add(31*time.Second))
	if missingScope.Allowed {
		t.Fatalf("expected scope validation failure")
	}

	expired := store.validateAt(ExecutionCredentialValidationInput{
		Token: issued.Token,
	}, issued.Credential.ExpiresAt.Add(1*time.Second))
	if expired.Allowed {
		t.Fatalf("expected expiry validation failure")
	}

	revokedCred, err := store.Revoke(issued.Credential.ID)
	if err != nil {
		t.Fatalf("revoke credential failed: %v", err)
	}
	if revokedCred.RevokedAt == nil {
		t.Fatalf("expected revoked timestamp")
	}

	revoked := store.Validate(ExecutionCredentialValidationInput{Token: issued.Token})
	if revoked.Allowed {
		t.Fatalf("expected revoked credential to fail validation")
	}
}

func TestExecutionCredentialIssueValidation(t *testing.T) {
	store := NewExecutionCredentialStore()
	if _, err := store.Issue(ExecutionCredentialIssueInput{Subject: "", TTLSeconds: 120}); err == nil {
		t.Fatalf("expected missing subject to fail")
	}
	if _, err := store.Issue(ExecutionCredentialIssueInput{Subject: "x", TTLSeconds: 1}); err == nil {
		t.Fatalf("expected low ttl to fail")
	}
	if _, err := store.Issue(ExecutionCredentialIssueInput{Subject: "x", TTLSeconds: 4000}); err == nil {
		t.Fatalf("expected high ttl to fail")
	}
}
