package config

import (
	"strings"
	"testing"
)

func TestMarshalCanonicalSortsHostsResourcesAndDepends(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{Hosts: []Host{
			{Name: "b", Transport: "local"},
			{Name: "a", Transport: "local"},
		}},
		Resources: []Resource{
			{ID: "z", Type: "command", Host: "a", Command: "echo z", DependsOn: []string{"b", "a"}},
			{ID: "a", Type: "file", Host: "b", Path: "/tmp/a"},
		},
	}

	b, err := MarshalCanonical(cfg, "yaml")
	if err != nil {
		t.Fatalf("marshal canonical failed: %v", err)
	}
	s := string(b)
	if strings.Index(s, "name: a") > strings.Index(s, "name: b") {
		t.Fatalf("expected hosts sorted by name: %s", s)
	}
	if strings.Index(s, "id: a") > strings.Index(s, "id: z") {
		t.Fatalf("expected resources sorted by id: %s", s)
	}
	if strings.Index(s, "- a") > strings.Index(s, "- b") {
		t.Fatalf("expected depends_on sorted: %s", s)
	}
}
