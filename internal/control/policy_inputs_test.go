package control

import (
	"context"
	"testing"
)

func TestResolvePolicyInputsMergeStrategies(t *testing.T) {
	reg := NewVariableSourceRegistry(t.TempDir())
	req := PolicyInputResolveRequest{
		Sources: []VariableSourceSpec{
			{
				Name: "base",
				Type: "inline",
				Config: map[string]any{
					"data": map[string]any{
						"service": map[string]any{
							"replicas": 2,
							"tier":     "base",
						},
						"region": "us-east-1",
					},
				},
			},
			{
				Name: "override",
				Type: "inline",
				Config: map[string]any{
					"data": map[string]any{
						"service": map[string]any{
							"replicas": 5,
						},
						"region": "us-west-2",
					},
				},
			},
		},
	}

	res, err := ResolvePolicyInputs(context.Background(), reg, req)
	if err != nil {
		t.Fatalf("resolve policy inputs default strategy failed: %v", err)
	}
	service, _ := res.Merged["service"].(map[string]any)
	if service["replicas"] != float64(5) || res.Merged["region"] != "us-west-2" {
		t.Fatalf("expected merge-last behavior, got %+v", res.Merged)
	}

	req.Strategy = "merge-first"
	res, err = ResolvePolicyInputs(context.Background(), reg, req)
	if err != nil {
		t.Fatalf("resolve policy inputs merge-first failed: %v", err)
	}
	service, _ = res.Merged["service"].(map[string]any)
	if service["replicas"] != float64(2) || res.Merged["region"] != "us-east-1" {
		t.Fatalf("expected merge-first behavior, got %+v", res.Merged)
	}
}

func TestResolvePolicyInputsHardFailAndLookup(t *testing.T) {
	reg := NewVariableSourceRegistry(t.TempDir())
	req := PolicyInputResolveRequest{
		HardFail: true,
		Lookup:   "service.replicas",
		Sources: []VariableSourceSpec{
			{
				Name: "base",
				Type: "inline",
				Config: map[string]any{
					"data": map[string]any{"service": map[string]any{"replicas": 2}},
				},
			},
			{
				Name: "override",
				Type: "inline",
				Config: map[string]any{
					"data": map[string]any{"service": map[string]any{"replicas": 3}},
				},
			},
		},
	}
	res, err := ResolvePolicyInputs(context.Background(), reg, req)
	if err == nil {
		t.Fatalf("expected hard-fail conflict error")
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("expected conflicts in hard-fail result")
	}

	req.HardFail = false
	res, err = ResolvePolicyInputs(context.Background(), reg, req)
	if err != nil {
		t.Fatalf("resolve policy inputs without hard fail failed: %v", err)
	}
	if !res.Found || res.Value != float64(3) {
		t.Fatalf("expected lookup value 3, got found=%t value=%#v", res.Found, res.Value)
	}
}
