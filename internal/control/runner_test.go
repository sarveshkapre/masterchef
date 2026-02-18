package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunner_ApplyPath(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "masterchef.yaml")
	outPath := filepath.Join(tmp, "out.txt")

	cfg := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: write-file
    type: file
    host: localhost
    path: ` + outPath + `
    content: "ok\n"
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	r := NewRunner(tmp)
	if err := r.ApplyPath(cfgPath); err != nil {
		t.Fatalf("apply path failed: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected out file: %v", err)
	}
}
