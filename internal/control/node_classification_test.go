package control

import "testing"

func TestNodeClassificationEvaluateByPriority(t *testing.T) {
	store := NewNodeClassificationStore()
	_, err := store.Upsert(NodeClassificationRuleInput{
		Name:        "prod-linux",
		MatchLabels: map[string]string{"env": "prod"},
		MatchFacts:  map[string]string{"os": "linux"},
		PolicyGroup: "production",
		RunList:     []string{"role[base]", "role[web]"},
		Variables:   map[string]any{"tier": "web"},
		Priority:    100,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("upsert first rule: %v", err)
	}
	_, err = store.Upsert(NodeClassificationRuleInput{
		Name:        "prod-default",
		MatchLabels: map[string]string{"env": "prod"},
		PolicyGroup: "prod-fallback",
		RunList:     []string{"role[base]"},
		Variables:   map[string]any{"owner": "platform"},
		Priority:    10,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("upsert second rule: %v", err)
	}
	res := store.Evaluate(NodeClassificationRequest{
		Node:   "web-1",
		Facts:  map[string]any{"os": "linux"},
		Labels: map[string]any{"env": "prod"},
	})
	if res.PolicyGroup != "production" {
		t.Fatalf("expected highest-priority policy group, got %+v", res)
	}
	if len(res.RunList) != 2 {
		t.Fatalf("expected deduped runlist of 2 items, got %+v", res)
	}
}
