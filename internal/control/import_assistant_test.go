package control

import "testing"

func TestBuildImportAssistant(t *testing.T) {
	res, err := BuildImportAssistant(ImportAssistantRequest{
		Type:         "secrets",
		SourceSystem: "vault",
		SampleFields: []string{"name", "secret_value"},
	})
	if err != nil {
		t.Fatalf("build import assistant failed: %v", err)
	}
	if len(res.RequiredFields) == 0 || len(res.NextSteps) == 0 {
		t.Fatalf("expected assistant output to include requirements and next steps")
	}
	if res.SuggestedMapping["name"] != "path" {
		t.Fatalf("expected sample mapping for name->path, got %+v", res.SuggestedMapping)
	}
}

func TestBuildImportAssistantValidation(t *testing.T) {
	if _, err := BuildImportAssistant(ImportAssistantRequest{}); err == nil {
		t.Fatalf("expected missing type error")
	}
	if _, err := BuildImportAssistant(ImportAssistantRequest{Type: "unknown"}); err == nil {
		t.Fatalf("expected unsupported type error")
	}
}
