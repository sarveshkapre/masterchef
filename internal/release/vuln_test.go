package release

import "testing"

func TestEvaluateCVEPolicy(t *testing.T) {
	deps := []Dependency{{Path: "github.com/example/lib", Version: "v1.2.3"}}
	advisories := []Advisory{
		{ID: "CVE-2026-0001", Module: "github.com/example/lib", Severity: "high", AffectedVersion: "v1.2.3", FixedVersion: "v1.2.4"},
		{ID: "CVE-2026-0002", Module: "github.com/example/lib", Severity: "low", AffectedVersion: "v1.2.3"},
	}
	report := EvaluateCVEPolicy(deps, advisories, CVEPolicy{BlockedSeverities: []string{"high"}})
	if report.Pass {
		t.Fatalf("expected violations for blocked high advisory")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected one violation, got %d", len(report.Violations))
	}

	report = EvaluateCVEPolicy(deps, advisories, CVEPolicy{BlockedSeverities: []string{"high"}, AllowIDs: []string{"CVE-2026-0001"}})
	if !report.Pass || len(report.Violations) != 0 {
		t.Fatalf("expected allowlist to suppress violation, got %+v", report)
	}
}
