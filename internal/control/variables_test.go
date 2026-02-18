package control

import "testing"

func TestResolveVariablesWithConflicts(t *testing.T) {
	result, err := ResolveVariables(VariableResolveRequest{
		Layers: []VariableLayer{
			{
				Name: "global",
				Data: map[string]any{
					"region": "us-east-1",
					"db": map[string]any{
						"host": "db-a",
						"port": 5432,
					},
				},
			},
			{
				Name: "environment/prod",
				Data: map[string]any{
					"db": map[string]any{
						"host": "db-prod",
					},
				},
			},
			{
				Name: "host/web-01",
				Data: map[string]any{
					"db": map[string]any{
						"host": "db-web-override",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success with hard_fail=false, got %v", err)
	}
	db, ok := result.Merged["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged db map, got %#v", result.Merged["db"])
	}
	if db["host"] != "db-web-override" {
		t.Fatalf("expected highest precedence override, got %#v", db["host"])
	}
	if len(result.Conflicts) < 2 {
		t.Fatalf("expected conflicts for db.host overrides, got %#v", result.Conflicts)
	}
	if len(result.SourceGraph) < 3 {
		t.Fatalf("expected source graph edges, got %#v", result.SourceGraph)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected ambiguous override warning")
	}
}

func TestResolveVariablesHardFail(t *testing.T) {
	_, err := ResolveVariables(VariableResolveRequest{
		HardFail: true,
		Layers: []VariableLayer{
			{Name: "a", Data: map[string]any{"x": "one"}},
			{Name: "b", Data: map[string]any{"x": "two"}},
		},
	})
	if err == nil {
		t.Fatalf("expected hard-fail conflict error")
	}
}
