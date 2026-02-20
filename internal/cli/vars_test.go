package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunVarsExplain(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "vars.layers.yaml")
	content := `layers:
  - name: defaults
    data:
      app:
        replicas: 2
  - name: prod
    data:
      app:
        replicas: 3
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runVars([]string{"explain", "-f", path, "-format", "json"}); err != nil {
		t.Fatalf("runVars explain failed: %v", err)
	}
}

func TestRunVarsExplainHardFail(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "vars.layers.yaml")
	content := `layers:
  - name: defaults
    data:
      image:
        tag: "1.0"
  - name: prod
    data:
      image:
        tag: "2.0"
hard_fail: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runVars([]string{"explain", "-f", path, "-format", "json"})
	if err == nil {
		t.Fatalf("expected hard-fail conflict error")
	}
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 4 {
		t.Fatalf("expected exit code 4, got %d", exitErr.Code)
	}
}
