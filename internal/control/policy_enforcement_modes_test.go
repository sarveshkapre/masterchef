package control

import "testing"

func TestPolicyEnforcementStoreUpsertAndList(t *testing.T) {
	store := NewPolicyEnforcementStore()
	item, err := store.Upsert(PolicyEnforcementModeInput{
		PolicyRef: "policy/prod",
		Mode:      "apply-and-monitor",
		Reason:    "prod rollout",
		UpdatedBy: "sre",
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if item.ID == "" || item.Mode != PolicyEnforcementApplyAndMonitor {
		t.Fatalf("unexpected upsert item: %+v", item)
	}
	item, err = store.Upsert(PolicyEnforcementModeInput{
		PolicyRef: "policy/prod",
		Mode:      "apply-and-autocorrect",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if item.Mode != PolicyEnforcementApplyAndAutocorrect {
		t.Fatalf("expected updated mode, got %+v", item)
	}
	items := store.List()
	if len(items) != 1 || items[0].PolicyRef != "policy/prod" {
		t.Fatalf("unexpected list result: %+v", items)
	}
}

func TestPolicyEnforcementStoreValidation(t *testing.T) {
	store := NewPolicyEnforcementStore()
	if _, err := store.Upsert(PolicyEnforcementModeInput{PolicyRef: "", Mode: "audit"}); err == nil {
		t.Fatalf("expected empty policy_ref validation error")
	}
	if _, err := store.Upsert(PolicyEnforcementModeInput{PolicyRef: "policy/prod", Mode: "invalid"}); err == nil {
		t.Fatalf("expected invalid mode validation error")
	}
}
