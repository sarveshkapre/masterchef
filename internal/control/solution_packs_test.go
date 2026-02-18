package control

import "testing"

func TestSolutionPackCatalog(t *testing.T) {
	c := NewSolutionPackCatalog()
	list := c.List()
	if len(list) < 3 {
		t.Fatalf("expected predefined solution packs, got %d", len(list))
	}
	p, err := c.Get("stateless-vm-service")
	if err != nil {
		t.Fatalf("get solution pack failed: %v", err)
	}
	if p.StarterConfigYAML == "" {
		t.Fatalf("expected starter config content")
	}
}
