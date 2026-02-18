package control

import (
	"testing"
	"time"
)

func TestRuleEngine_EvaluateMatchAndCooldown(t *testing.T) {
	eng := NewRuleEngine()
	rule, err := eng.Create(Rule{
		Name:         "alert-remediation",
		SourcePrefix: "external.alert",
		MatchMode:    "all",
		Conditions: []RuleCondition{
			{Field: "fields.sev", Comparator: "eq", Value: "high"},
		},
		Actions: []RuleAction{{Type: "enqueue_apply", ConfigPath: "cfg.yaml", Priority: "high"}},
	})
	if err != nil {
		t.Fatalf("unexpected rule create error: %v", err)
	}
	if rule.ID == "" {
		t.Fatalf("expected rule id")
	}

	event := Event{Type: "external.alert", Fields: map[string]any{"sev": "high"}}
	matches, err := eng.Evaluate(event)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one matching rule, got %d", len(matches))
	}

	_, err = eng.SetEnabled(rule.ID, false)
	if err != nil {
		t.Fatalf("unexpected set enabled error: %v", err)
	}
	matches, err = eng.Evaluate(event)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no matches when rule disabled")
	}

	_, err = eng.SetEnabled(rule.ID, true)
	if err != nil {
		t.Fatalf("unexpected set enabled error: %v", err)
	}
	got, err := eng.Get(rule.ID)
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	got.CooldownSeconds = 60
	// Replace by recreating internal field update through direct map access is not exposed;
	// create another cooldown rule instead.
	cool, err := eng.Create(Rule{
		Name:            "cooldown",
		SourcePrefix:    "external.alert",
		CooldownSeconds: 5,
		Actions:         []RuleAction{{Type: "enqueue_apply", ConfigPath: "cfg2.yaml"}},
	})
	if err != nil {
		t.Fatalf("unexpected cooldown rule create error: %v", err)
	}
	_ = cool
	matches, err = eng.Evaluate(event)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected initial match before cooldown")
	}
	matches, err = eng.Evaluate(event)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(matches) > 1 {
		t.Fatalf("expected cooldown to suppress second cooldown-rule trigger, got %d matches", len(matches))
	}

	// Wait to avoid flaky timestamp equality on very fast systems.
	time.Sleep(10 * time.Millisecond)
}

func TestRuleEngine_CreateValidation(t *testing.T) {
	eng := NewRuleEngine()
	_, err := eng.Create(Rule{Name: "x", Actions: []RuleAction{{Type: "enqueue_apply", ConfigPath: "a.yaml"}}})
	if err == nil {
		t.Fatalf("expected source_prefix validation error")
	}
	_, err = eng.Create(Rule{Name: "x", SourcePrefix: "a", Actions: []RuleAction{{Type: "launch_template"}}})
	if err == nil {
		t.Fatalf("expected action validation error")
	}
}
