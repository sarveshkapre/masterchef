package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDoctor(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	content := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: f1
    type: file
    host: localhost
    path: /tmp/x
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runDoctor([]string{"-f", cfg, "-format", "json"}); err != nil {
		t.Fatalf("runDoctor failed: %v", err)
	}
}
