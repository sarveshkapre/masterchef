package planner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestSaveAndCompareSnapshot(t *testing.T) {
	cfg := &config.Config{
		Version: "v0",
		Inventory: config.Inventory{
			Hosts: []config.Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []config.Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a"},
			{ID: "b", Type: "file", Host: "localhost", Path: "/tmp/b", DependsOn: []string{"a"}},
		},
	}
	plan, err := Build(cfg)
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	tmp := t.TempDir()
	snapshotPath := filepath.Join(tmp, "plan.snapshot.json")
	if err := SaveSnapshot(snapshotPath, plan); err != nil {
		t.Fatalf("save snapshot failed: %v", err)
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}

	diff, err := CompareSnapshot(snapshotPath, plan)
	if err != nil {
		t.Fatalf("compare snapshot failed: %v", err)
	}
	if !diff.Match {
		t.Fatalf("expected snapshot to match: %+v", diff)
	}
}

func TestDiffPlansDetectsChanges(t *testing.T) {
	base := &Plan{Steps: []Step{
		{Order: 1, Resource: config.Resource{ID: "a", Type: "file", Host: "h1", Path: "/tmp/a"}},
		{Order: 2, Resource: config.Resource{ID: "b", Type: "file", Host: "h1", Path: "/tmp/b"}},
	}}
	current := &Plan{Steps: []Step{
		{Order: 1, Resource: config.Resource{ID: "a", Type: "file", Host: "h1", Path: "/tmp/a-updated"}},
		{Order: 2, Resource: config.Resource{ID: "c", Type: "file", Host: "h1", Path: "/tmp/c"}},
	}}
	diff := DiffPlans(base, current)
	if diff.Match {
		t.Fatalf("expected diff mismatch")
	}
	if len(diff.AddedSteps) != 1 || diff.AddedSteps[0] != "c" {
		t.Fatalf("expected added step c, got %+v", diff.AddedSteps)
	}
	if len(diff.RemovedSteps) != 1 || diff.RemovedSteps[0] != "b" {
		t.Fatalf("expected removed step b, got %+v", diff.RemovedSteps)
	}
	if len(diff.ChangedSteps) != 1 || diff.ChangedSteps[0] != "a" {
		t.Fatalf("expected changed step a, got %+v", diff.ChangedSteps)
	}
}
