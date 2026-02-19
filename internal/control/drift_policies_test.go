package control

import (
	"testing"
	"time"
)

func TestDriftPolicyStore_SuppressionAndAllowlistMatching(t *testing.T) {
	store := NewDriftPolicyStore()

	sup, err := store.AddSuppression(DriftSuppressionInput{
		ScopeType:  "host",
		ScopeValue: "node-a",
		Reason:     "maintenance",
		CreatedBy:  "sre",
		Until:      time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("add suppression failed: %v", err)
	}
	if sup.ID == "" || sup.ScopeType != "host" || sup.ScopeValue != "node-a" {
		t.Fatalf("unexpected suppression: %+v", sup)
	}
	if !store.IsSuppressed("node-a", "command", "r1", time.Now().UTC()) {
		t.Fatalf("expected host suppression match")
	}
	if store.IsSuppressed("node-b", "command", "r1", time.Now().UTC()) {
		t.Fatalf("did not expect suppression match for other host")
	}

	allow, err := store.AddAllowlist(DriftAllowlistInput{
		ScopeType:  "resource_type",
		ScopeValue: "file",
		Reason:     "known benign",
		CreatedBy:  "platform",
	})
	if err != nil {
		t.Fatalf("add allowlist failed: %v", err)
	}
	if allow.ID == "" || allow.ScopeType != "resource_type" {
		t.Fatalf("unexpected allowlist: %+v", allow)
	}
	if !store.IsAllowlisted("node-b", "file", "r2", time.Now().UTC()) {
		t.Fatalf("expected allowlist match for resource type")
	}
	if store.IsAllowlisted("node-b", "command", "r2", time.Now().UTC()) {
		t.Fatalf("did not expect allowlist match for different resource type")
	}
}

func TestDriftPolicyStore_Validation(t *testing.T) {
	store := NewDriftPolicyStore()

	if _, err := store.AddSuppression(DriftSuppressionInput{
		ScopeType: "host",
		Until:     time.Now().UTC().Add(5 * time.Minute),
	}); err == nil {
		t.Fatalf("expected scoped suppression without scope value to fail")
	}
	if _, err := store.AddSuppression(DriftSuppressionInput{
		ScopeType:  "host",
		ScopeValue: "node-a",
		Until:      time.Now().UTC().Add(-1 * time.Minute),
	}); err == nil {
		t.Fatalf("expected past suppression until to fail")
	}
	if _, err := store.AddAllowlist(DriftAllowlistInput{
		ScopeType: "invalid",
	}); err == nil {
		t.Fatalf("expected invalid scope type to fail")
	}
	if _, err := store.AddAllowlist(DriftAllowlistInput{
		ScopeType:  "resource_id",
		ScopeValue: "r1",
		ExpiresAt:  time.Now().UTC().Add(-1 * time.Minute),
	}); err == nil {
		t.Fatalf("expected past allowlist expiry to fail")
	}
}
