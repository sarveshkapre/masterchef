package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunApply_WritesReport(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	report := filepath.Join(tmp, "out", "run-report.json")
	target := filepath.Join(tmp, "file.txt")

	content := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: f1
    type: file
    host: localhost
    path: ` + target + `
    content: "ok\n"
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if err := runApply([]string{"-f", cfg, "-yes", "-report", report}); err != nil {
		t.Fatalf("runApply failed: %v", err)
	}
	if _, err := os.Stat(report); err != nil {
		t.Fatalf("expected report file, got error: %v", err)
	}
}
