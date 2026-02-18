package control

import "testing"

func TestUseCaseTemplateCatalog(t *testing.T) {
	catalog := NewUseCaseTemplateCatalog()
	list := catalog.List()
	if len(list) < 11 {
		t.Fatalf("expected predefined use-case templates, got %d", len(list))
	}

	item, err := catalog.Get("blue-green-release")
	if err != nil {
		t.Fatalf("get use-case template failed: %v", err)
	}
	if item.Scenario != "release-rollout" {
		t.Fatalf("unexpected scenario: %s", item.Scenario)
	}
	if len(item.WorkflowStepFiles) != 3 {
		t.Fatalf("expected three workflow step files, got %d", len(item.WorkflowStepFiles))
	}
	if _, ok := item.ScaffoldFiles["configs/apply.yaml"]; !ok {
		t.Fatalf("expected apply scaffold file")
	}
}
