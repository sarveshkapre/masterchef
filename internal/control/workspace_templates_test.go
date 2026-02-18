package control

import "testing"

func TestWorkspaceTemplateCatalog(t *testing.T) {
	catalog := NewWorkspaceTemplateCatalog()
	list := catalog.List()
	if len(list) < 4 {
		t.Fatalf("expected predefined workspace templates, got %d", len(list))
	}

	item, err := catalog.Get("stateless-kubernetes-service")
	if err != nil {
		t.Fatalf("get workspace template failed: %v", err)
	}
	if item.Pattern != "stateless-services" {
		t.Fatalf("expected stateless-services pattern, got %q", item.Pattern)
	}
	if len(item.ScaffoldFiles) == 0 {
		t.Fatalf("expected scaffold files to be present")
	}
	if _, ok := item.ScaffoldFiles["policy/main.yaml"]; !ok {
		t.Fatalf("expected policy scaffold file")
	}
}
