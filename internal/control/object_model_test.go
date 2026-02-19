package control

import "testing"

func TestObjectModelRegistryListAndResolve(t *testing.T) {
	registry := NewObjectModelRegistry()
	items := registry.List()
	if len(items) < 5 {
		t.Fatalf("expected object model entries, got %d", len(items))
	}

	entry, err := registry.Resolve("playbook")
	if err != nil {
		t.Fatalf("expected alias resolution to succeed: %v", err)
	}
	if entry.Canonical != "runbook" {
		t.Fatalf("expected runbook canonical mapping, got %+v", entry)
	}

	entry, err = registry.Resolve("Policy Bundle")
	if err != nil {
		t.Fatalf("expected case-insensitive term resolution: %v", err)
	}
	if entry.Canonical != "policy_bundle" {
		t.Fatalf("expected policy_bundle canonical mapping, got %+v", entry)
	}

	if _, err := registry.Resolve("unknown-term"); err == nil {
		t.Fatalf("expected unknown term resolution error")
	}
}
