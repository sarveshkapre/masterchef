package cli

import "testing"

func TestRunTestImpact(t *testing.T) {
	if err := runTestImpact([]string{"-changes", "internal/control/queue.go,internal/server/server.go", "-format", "json"}); err != nil {
		t.Fatalf("runTestImpact failed: %v", err)
	}
}
