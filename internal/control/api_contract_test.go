package control

import "testing"

func TestDiffAPISpec(t *testing.T) {
	base := APISpec{Version: "v1", Endpoints: []string{"GET /a", "POST /b"}}
	cur := APISpec{Version: "v2", Endpoints: []string{"GET /a", "POST /b", "GET /c"}}
	report := DiffAPISpec(base, cur)
	if len(report.Added) != 1 || report.Added[0] != "GET /c" {
		t.Fatalf("unexpected added endpoints: %#v", report.Added)
	}
	if !report.BackwardCompatible {
		t.Fatalf("expected backward compatibility when no removals")
	}
	if report.ForwardCompatible {
		t.Fatalf("expected forward incompatibility when added endpoints exist")
	}
	if !report.DeprecationLifecyclePass {
		t.Fatalf("expected no lifecycle violations when no endpoint removals")
	}
}

func TestDiffAPISpecDeprecationLifecycle(t *testing.T) {
	base := APISpec{
		Version:   "v1",
		Endpoints: []string{"GET /a", "POST /b"},
		Deprecations: []APIDeprecation{
			{
				Endpoint:           "POST /b",
				AnnouncedVersion:   "v1",
				RemoveAfterVersion: "v3",
				Replacement:        "POST /b2",
			},
		},
	}

	tooEarly := APISpec{Version: "v2", Endpoints: []string{"GET /a"}}
	tooEarlyReport := DiffAPISpec(base, tooEarly)
	if tooEarlyReport.DeprecationLifecyclePass {
		t.Fatalf("expected lifecycle violation for early endpoint removal")
	}
	if len(tooEarlyReport.DeprecationViolations) == 0 {
		t.Fatalf("expected deprecation violations for early removal")
	}

	onTime := APISpec{Version: "v3", Endpoints: []string{"GET /a"}}
	onTimeReport := DiffAPISpec(base, onTime)
	if !onTimeReport.DeprecationLifecyclePass {
		t.Fatalf("expected lifecycle pass when removal is at declared remove_after_version")
	}

	noDepBase := APISpec{Version: "v1", Endpoints: []string{"GET /a", "POST /z"}}
	noDepCur := APISpec{Version: "v2", Endpoints: []string{"GET /a"}}
	noDepReport := DiffAPISpec(noDepBase, noDepCur)
	if noDepReport.DeprecationLifecyclePass {
		t.Fatalf("expected lifecycle violation when removed endpoint has no prior deprecation")
	}
}

func TestGenerateUpgradeAdvice(t *testing.T) {
	report := APIDiffReport{
		Removed: []string{"GET /v1/old"},
		DeprecationsAdded: []APIDeprecation{
			{
				Endpoint:           "DELETE /v1/templates/{id}/delete",
				AnnouncedVersion:   "v1",
				RemoveAfterVersion: "v3",
				Replacement:        "DELETE /v1/templates/{id}",
			},
		},
		DeprecationViolations:    []string{"GET /v1/old: removed without prior deprecation"},
		DeprecationLifecyclePass: false,
	}
	advice := GenerateUpgradeAdvice(report)
	if len(advice) < 2 {
		t.Fatalf("expected multiple upgrade advice entries, got %d", len(advice))
	}
	if advice[0].Severity != "error" {
		t.Fatalf("expected first advice severity=error, got %+v", advice[0])
	}
	foundWarn := false
	for _, item := range advice {
		if item.Severity == "warn" && item.Endpoint == "DELETE /v1/templates/{id}/delete" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("expected warning advice for deprecated endpoint")
	}
}
