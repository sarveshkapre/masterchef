package testimpact

import "testing"

func TestAnalyzeKnownPaths(t *testing.T) {
	report := Analyze([]string{"internal/control/queue.go", "internal/server/server_test.go"})
	if report.FallbackToAll {
		t.Fatalf("did not expect fallback for known paths")
	}
	if len(report.ImpactedPackages) == 0 {
		t.Fatalf("expected impacted packages")
	}
	foundControl := false
	foundServer := false
	for _, pkg := range report.ImpactedPackages {
		if pkg == "./internal/control" {
			foundControl = true
		}
		if pkg == "./internal/server" {
			foundServer = true
		}
	}
	if !foundControl || !foundServer {
		t.Fatalf("expected impacted control and server packages: %#v", report.ImpactedPackages)
	}
}

func TestAnalyzeFallback(t *testing.T) {
	report := Analyze([]string{"README.md"})
	if !report.FallbackToAll {
		t.Fatalf("expected fallback for unknown mapping")
	}
	if len(report.ImpactedPackages) == 0 {
		t.Fatalf("expected fallback packages")
	}
}
