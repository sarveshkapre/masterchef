package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDeploy_CLITrigger(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "deploy.yaml")
	target := filepath.Join(tmp, "deployed.txt")

	content := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: deploy-file
    type: file
    host: localhost
    path: ` + target + `
    content: "deployed\n"
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if err := runDeploy([]string{"-f", cfg, "-env", "staging", "-branch", "env/staging", "-yes"}); err != nil {
		t.Fatalf("runDeploy failed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected deploy target file to be created: %v", err)
	}
}
