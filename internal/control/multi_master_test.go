package control

import (
	"testing"
	"time"
)

func TestMultiMasterStoreNodesAndCache(t *testing.T) {
	store := NewMultiMasterStore()
	node, err := store.UpsertNode(MultiMasterNodeInput{
		NodeID:  "cp-us-1",
		Region:  "us-east-1",
		Address: "10.0.0.1",
		Role:    "primary",
		Status:  "active",
	})
	if err != nil {
		t.Fatalf("upsert node failed: %v", err)
	}
	if node.NodeID == "" || node.Role != "primary" {
		t.Fatalf("unexpected node %+v", node)
	}
	if _, err := store.Heartbeat("cp-us-1", "degraded"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	got, ok := store.GetNode("cp-us-1")
	if !ok {
		t.Fatalf("expected node")
	}
	if got.Status != "degraded" {
		t.Fatalf("expected degraded status, got %+v", got)
	}

	jobs := []Job{{
		ID:         "job-1",
		ConfigPath: "c.yaml",
		Priority:   "high",
		Status:     JobPending,
		CreatedAt:  time.Now().UTC(),
	}}
	events := []Event{{
		Time:    time.Now().UTC(),
		Type:    "job.pending",
		Message: "queued",
		Fields:  map[string]any{"job_id": "job-1"},
	}}
	result := store.SyncCentralCache(jobs, events, 100)
	if result.SyncedJobs != 1 || result.SyncedEvents != 1 {
		t.Fatalf("unexpected sync result %+v", result)
	}
	if len(store.ListCentralCache("job", 10)) != 1 {
		t.Fatalf("expected one job cache entry")
	}
	if len(store.ListCentralCache("event", 10)) != 1 {
		t.Fatalf("expected one event cache entry")
	}
}
