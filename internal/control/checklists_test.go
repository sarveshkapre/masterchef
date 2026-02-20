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

func TestChecklistStoreEvaluateGate(t *testing.T) {
	store := NewChecklistStore()
	run, err := store.Create("gate test", "high", nil)
	if err != nil {
		t.Fatalf("create checklist failed: %v", err)
	}

	preGate, err := store.EvaluateGate(run.ID, "pre")
	if err != nil {
		t.Fatalf("evaluate pre gate failed: %v", err)
	}
	if preGate.Allowed || len(preGate.Blockers) == 0 {
		t.Fatalf("expected pre gate to block before completion, got %+v", preGate)
	}

	for _, item := range run.Items {
		if item.Phase != "pre" || !item.Required {
			continue
		}
		if _, err := store.CompleteItem(run.ID, item.ID, "done"); err != nil {
			t.Fatalf("complete pre item failed: %v", err)
		}
	}
	preGate, err = store.EvaluateGate(run.ID, "pre")
	if err != nil {
		t.Fatalf("evaluate pre gate after completion failed: %v", err)
	}
	if !preGate.Allowed {
		t.Fatalf("expected pre gate to allow after required pre items, got %+v", preGate)
	}

	postGate, err := store.EvaluateGate(run.ID, "post")
	if err != nil {
		t.Fatalf("evaluate post gate failed: %v", err)
	}
	if postGate.Allowed || len(postGate.Blockers) == 0 {
		t.Fatalf("expected post gate to block before post completion, got %+v", postGate)
	}
}
