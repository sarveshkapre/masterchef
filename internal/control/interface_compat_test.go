package control

import "testing"

func TestDetectInterfaceCompatibilityBreaking(t *testing.T) {
	report, err := DetectInterfaceCompatibility(
		InterfaceContract{
			Kind:    "provider",
			Name:    "pkg-provider",
			Version: "v1",
			Inputs:  map[string]string{"name": "string", "version": "string"},
			Outputs: map[string]string{"changed": "bool"},
		},
		InterfaceContract{
			Kind:    "provider",
			Name:    "pkg-provider",
			Version: "v2",
			Inputs:  map[string]string{"name": "string"},
			Outputs: map[string]string{"changed": "string"},
		},
	)
	if err != nil {
		t.Fatalf("detect interface compatibility failed: %v", err)
	}
	if !report.Breaking || len(report.BreakingChanges) == 0 {
		t.Fatalf("expected breaking changes, got %+v", report)
	}
}

func TestDetectInterfaceCompatibilityNonBreaking(t *testing.T) {
	report, err := DetectInterfaceCompatibility(
		InterfaceContract{
			Kind:    "module",
			Name:    "web-module",
			Version: "v1",
			Inputs:  map[string]string{"name": "string"},
			Outputs: map[string]string{"status": "string"},
		},
		InterfaceContract{
			Kind:    "module",
			Name:    "web-module",
			Version: "v2",
			Inputs:  map[string]string{"name": "string", "region": "string"},
			Outputs: map[string]string{"status": "string", "endpoint": "string"},
		},
	)
	if err != nil {
		t.Fatalf("detect interface compatibility failed: %v", err)
	}
	if report.Breaking {
		t.Fatalf("expected non-breaking changes, got %+v", report)
	}
	if len(report.NonBreakingHints) == 0 {
		t.Fatalf("expected non-breaking hints, got %+v", report)
	}
}
