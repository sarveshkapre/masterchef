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
}
