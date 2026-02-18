package config

import "testing"

func TestAnalyzeReturnsDiagnostics(t *testing.T) {
	cfg := &Config{
		Version:   "v1",
		Inventory: Inventory{Hosts: []Host{{Name: "h1", Transport: "ssh"}}},
		Resources: []Resource{
			{ID: "c1", Type: "command", Host: "h1", Command: "echo hi"},
			{ID: "f1", Type: "file", Host: "h1", Path: "/tmp/x"},
		},
	}
	diags := Analyze(cfg)
	if len(diags) < 3 {
		t.Fatalf("expected multiple diagnostics, got %d", len(diags))
	}
}
