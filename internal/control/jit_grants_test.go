package control

import (
	"testing"
	"time"
)

func TestJITAccessGrantLifecycle(t *testing.T) {
	store := NewJITAccessGrantStore()
	issued, err := store.Issue(JITAccessGrantIssueInput{
		Subject:    "oncall-sre",
		Resource:   "cluster/prod-a",
		Action:     "restart-service",
		IssuedBy:   "incident-commander",
		Reason:     "mitigate outage",
		TTLSeconds: 120,
	})
	if err != nil {
		t.Fatalf("issue jit grant failed: %v", err)
	}
	if issued.Token == "" || issued.Grant.ID == "" {
		t.Fatalf("expected issued token and grant id")
	}

	ok := store.validateAt(JITAccessGrantValidationInput{
		Token:    issued.Token,
		Resource: "cluster/prod-a",
		Action:   "restart-service",
	}, issued.Grant.CreatedAt.Add(30*time.Second))
	if !ok.Allowed {
		t.Fatalf("expected jit grant validation success: %+v", ok)
	}

	mismatch := store.validateAt(JITAccessGrantValidationInput{
		Token:    issued.Token,
		Resource: "cluster/prod-b",
		Action:   "restart-service",
	}, issued.Grant.CreatedAt.Add(31*time.Second))
	if mismatch.Allowed {
		t.Fatalf("expected resource mismatch failure")
	}

	expired := store.validateAt(JITAccessGrantValidationInput{
		Token:    issued.Token,
		Resource: "cluster/prod-a",
		Action:   "restart-service",
	}, issued.Grant.ExpiresAt.Add(time.Second))
	if expired.Allowed {
		t.Fatalf("expected expired grant failure")
	}

	revoked, err := store.Revoke(issued.Grant.ID)
	if err != nil {
		t.Fatalf("revoke jit grant failed: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected revoked timestamp")
	}
}

func TestJITAccessGrantIssueValidation(t *testing.T) {
	store := NewJITAccessGrantStore()
	if _, err := store.Issue(JITAccessGrantIssueInput{
		Subject:  "",
		Resource: "x",
		Action:   "y",
		IssuedBy: "z",
		Reason:   "r",
	}); err == nil {
		t.Fatalf("expected missing subject validation failure")
	}
	if _, err := store.Issue(JITAccessGrantIssueInput{
		Subject:    "a",
		Resource:   "x",
		Action:     "y",
		IssuedBy:   "z",
		Reason:     "r",
		TTLSeconds: 10,
	}); err == nil {
		t.Fatalf("expected low ttl validation failure")
	}
}
