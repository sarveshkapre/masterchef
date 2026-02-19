package testimpact

import "testing"

func TestAnalyzeKnownPaths(t *testing.T) {
	report := Analyze([]string{"internal/control/queue.go", "internal/server/server_test.go"})
	if report.FallbackToAll {
		t.Fatalf("did not expect fallback for known paths")
	}
	if report.Scope != "targeted" {
		t.Fatalf("expected targeted scope, got %s", report.Scope)
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
	if report.RecommendedTest == "" {
		t.Fatalf("expected recommended test command")
	}
}

func TestAnalyzeFallback(t *testing.T) {
	report := Analyze([]string{"README.md"})
	if !report.FallbackToAll {
		t.Fatalf("expected fallback for unknown mapping")
	}
	if report.Scope != "safe-fallback" {
		t.Fatalf("expected safe-fallback scope, got %s", report.Scope)
	}
	if len(report.ImpactedPackages) == 0 {
		t.Fatalf("expected fallback packages")
	}
}

func TestAnalyzeWithOptionsMaxTargetedFallback(t *testing.T) {
	report := AnalyzeWithOptions(
		[]string{
			"internal/config/config.go",
			"internal/planner/plan.go",
			"internal/executor/executor.go",
			"internal/server/server.go",
			"internal/control/queue.go",
			"internal/storage/storage.go",
		},
		AnalyzeOptions{MaxTargetedPackage: 2},
	)
	if !report.FallbackToAll {
		t.Fatalf("expected fallback when targeted package count exceeds threshold")
	}
	if report.Scope != "safe-fallback" {
		t.Fatalf("expected safe-fallback scope, got %s", report.Scope)
	}
}
