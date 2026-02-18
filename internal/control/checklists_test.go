package control

import "testing"

func TestChecklistStoreCreateAndComplete(t *testing.T) {
	store := NewChecklistStore()
	run, err := store.Create("high risk db migration", "high", map[string]any{"change_id": "cr-1"})
	if err != nil {
		t.Fatalf("create checklist failed: %v", err)
	}
	if run.Status != ChecklistOpen {
		t.Fatalf("expected open checklist status")
	}
	if len(run.Items) < 6 {
		t.Fatalf("expected high risk checklist to include extended prompts")
	}

	for _, item := range run.Items {
		if !item.Required {
			continue
		}
		run, err = store.CompleteItem(run.ID, item.ID, "done")
		if err != nil {
			t.Fatalf("complete checklist item failed: %v", err)
		}
	}
	if run.Status != ChecklistCompleted {
		t.Fatalf("expected completed status after required items done, got %s", run.Status)
	}
}
