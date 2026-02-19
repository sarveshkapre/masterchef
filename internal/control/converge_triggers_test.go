package control

import "testing"

func TestConvergeTriggerStore_RecordAndList(t *testing.T) {
	store := NewConvergeTriggerStore(10)

	first, err := store.NewTrigger(ConvergeTriggerInput{
		Source:      "policy",
		ConfigPath:  "prod.yaml",
		Priority:    "high",
		AutoEnqueue: true,
		Payload:     map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("record first trigger failed: %v", err)
	}
	if first.ID == "" || first.Status != ConvergeTriggerRecorded || first.Priority != "high" {
		t.Fatalf("unexpected first trigger: %+v", first)
	}
	updated, ok := store.UpdateOutcome(first.ID, ConvergeTriggerQueued, "job-1", "")
	if !ok {
		t.Fatalf("expected update outcome to succeed")
	}
	if updated.Status != ConvergeTriggerQueued || updated.JobID != "job-1" {
		t.Fatalf("unexpected updated trigger: %+v", updated)
	}

	second, err := store.NewTrigger(ConvergeTriggerInput{
		Source:      "security",
		ConfigPath:  "prod.yaml",
		Priority:    "normal",
		AutoEnqueue: false,
	})
	if err != nil {
		t.Fatalf("record second trigger failed: %v", err)
	}
	if second.AutoEnqueue {
		t.Fatalf("expected auto_enqueue=false to persist")
	}

	items := store.List(10)
	if len(items) != 2 {
		t.Fatalf("expected two trigger records, got %d", len(items))
	}
	if items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("expected reverse chronological list order, got %+v", items)
	}
}

func TestConvergeTriggerStore_Validation(t *testing.T) {
	store := NewConvergeTriggerStore(10)
	if _, err := store.NewTrigger(ConvergeTriggerInput{Source: "unknown", ConfigPath: "c.yaml"}); err == nil {
		t.Fatalf("expected invalid source to fail")
	}
	if _, err := store.NewTrigger(ConvergeTriggerInput{Source: "policy"}); err == nil {
		t.Fatalf("expected missing config path to fail")
	}
}
