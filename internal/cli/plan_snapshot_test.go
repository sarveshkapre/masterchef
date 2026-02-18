package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunPlanSnapshotUpdateAndCheck(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	snapshot := filepath.Join(tmp, "plan.snapshot.json")

	content := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: a
    type: file
    host: localhost
    path: /tmp/a
  - id: b
    type: file
    host: localhost
    path: /tmp/b
    depends_on: [a]
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runPlan([]string{"-f", cfg, "-snapshot", snapshot, "-update-snapshot"}); err != nil {
		t.Fatalf("runPlan snapshot update failed: %v", err)
	}
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
	if err := runPlan([]string{"-f", cfg, "-snapshot", snapshot}); err != nil {
		t.Fatalf("runPlan snapshot check failed: %v", err)
	}
}

func TestRunPlanSnapshotDetectsRegression(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	snapshot := filepath.Join(tmp, "plan.snapshot.json")

	original := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: a
    type: file
    host: localhost
    path: /tmp/a
`
	if err := os.WriteFile(cfg, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runPlan([]string{"-f", cfg, "-snapshot", snapshot, "-update-snapshot"}); err != nil {
		t.Fatalf("runPlan snapshot update failed: %v", err)
	}

	updated := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: a
    type: file
    host: localhost
    path: /tmp/a
  - id: b
    type: command
    host: localhost
    command: "echo changed"
`
	if err := os.WriteFile(cfg, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runPlan([]string{"-f", cfg, "-snapshot", snapshot})
	if err == nil {
		t.Fatalf("expected snapshot regression error")
	}
	ec, ok := err.(ExitError)
	if !ok || ec.Code != 9 {
		t.Fatalf("expected ExitError code 9, got %v", err)
	}
}
