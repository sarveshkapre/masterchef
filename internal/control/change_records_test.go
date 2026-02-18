package control

import "testing"

func TestChangeRecordLifecycle(t *testing.T) {
	store := NewChangeRecordStore()
	rec, err := store.Create(ChangeRecord{
		Summary:      "database config rollout",
		TicketSystem: "jira",
		TicketID:     "OPS-123",
		TicketURL:    "https://tickets.example/OPS-123",
		ConfigPath:   "db-rollout.yaml",
		RequestedBy:  "sre-user",
	})
	if err != nil {
		t.Fatalf("create change record failed: %v", err)
	}
	if rec.Status != ChangeRecordProposed {
		t.Fatalf("expected proposed status, got %s", rec.Status)
	}

	rec, err = store.Approve(rec.ID, "approver-1", "looks good")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if rec.Status != ChangeRecordApproved || len(rec.Approvals) != 1 {
		t.Fatalf("expected approved status with one approval, got %+v", rec)
	}

	rec, err = store.AttachJob(rec.ID, "job-42")
	if err != nil {
		t.Fatalf("attach job failed: %v", err)
	}
	if rec.Status != ChangeRecordExecuting || rec.LinkedJobID != "job-42" {
		t.Fatalf("expected executing status with linked job, got %+v", rec)
	}

	rec, err = store.MarkCompleted(rec.ID)
	if err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}
	if rec.Status != ChangeRecordCompleted {
		t.Fatalf("expected completed status, got %+v", rec)
	}
}
