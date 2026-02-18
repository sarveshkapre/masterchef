package control

import "testing"

func TestRunbookStoreLifecycle(t *testing.T) {
	store := NewRunbookStore()

	rb, err := store.Create(Runbook{
		Name:       "DB emergency rollback",
		TargetType: RunbookTargetConfig,
		ConfigPath: "rollback.yaml",
		RiskLevel:  "high",
		Tags:       []string{"db", "rollback", "db"},
	})
	if err != nil {
		t.Fatalf("create runbook failed: %v", err)
	}
	if rb.Status != RunbookDraft {
		t.Fatalf("expected draft status, got %s", rb.Status)
	}
	if len(rb.Tags) != 2 {
		t.Fatalf("expected deduplicated tags")
	}

	rb, err = store.Approve(rb.ID)
	if err != nil {
		t.Fatalf("approve runbook failed: %v", err)
	}
	if rb.Status != RunbookApproved {
		t.Fatalf("expected approved status, got %s", rb.Status)
	}

	rb, err = store.Deprecate(rb.ID)
	if err != nil {
		t.Fatalf("deprecate runbook failed: %v", err)
	}
	if rb.Status != RunbookDeprecated {
		t.Fatalf("expected deprecated status, got %s", rb.Status)
	}
}
