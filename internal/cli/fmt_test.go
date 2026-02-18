package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFmt_WritesCanonicalConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	out := filepath.Join(tmp, "canonical.yaml")

	content := `version: v0
inventory:
  hosts:
    - name: b
      transport: local
    - name: a
      transport: local
resources:
  - id: z
    type: command
    host: a
    command: "echo z"
    depends_on: [a]
  - id: a
    type: file
    host: b
    path: /tmp/a
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runFmt([]string{"-f", cfg, "-o", out}); err != nil {
		t.Fatalf("runFmt failed: %v", err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read canonical output failed: %v", err)
	}
	s := string(b)
	if strings.Index(s, "name: a") > strings.Index(s, "name: b") {
		t.Fatalf("expected host ordering to be canonical, got: %s", s)
	}
	if strings.Index(s, "id: a") > strings.Index(s, "id: z") {
		t.Fatalf("expected resource ordering to be canonical, got: %s", s)
	}
}
