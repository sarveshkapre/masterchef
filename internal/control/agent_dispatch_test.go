package control

import "testing"

func TestAgentDispatchStoreModeAndRecords(t *testing.T) {
	store := NewAgentDispatchStore()
	if store.Mode() != AgentDispatchModeLocal {
		t.Fatalf("expected default local mode")
	}
	if _, err := store.SetMode("event_bus"); err != nil {
		t.Fatalf("set mode failed: %v", err)
	}
	if store.Mode() != AgentDispatchModeEventBus {
		t.Fatalf("expected event_bus mode")
	}
	rec := store.Record(store.Mode(), "hybrid", AgentDispatchRequest{
		ConfigPath:  "c.yaml",
		Environment: "prod",
		Priority:    "high",
		Force:       true,
	}, "dispatched", "")
	if rec.ID == "" || rec.Mode != AgentDispatchModeEventBus || rec.Status != "dispatched" {
		t.Fatalf("unexpected dispatch record %+v", rec)
	}
	if rec.Strategy != AgentDispatchStrategyHybrid {
		t.Fatalf("expected hybrid strategy in record, got %+v", rec)
	}
	if len(store.List(10)) != 1 {
		t.Fatalf("expected one dispatch record")
	}
}

func TestAgentDispatchEnvironmentStrategies(t *testing.T) {
	store := NewAgentDispatchStore()
	set, err := store.SetEnvironmentStrategy("prod", "pull")
	if err != nil {
		t.Fatalf("set environment strategy failed: %v", err)
	}
	if set.Strategy != AgentDispatchStrategyPull {
		t.Fatalf("expected pull strategy, got %+v", set)
	}
	got, ok := store.GetEnvironmentStrategy("prod")
	if !ok {
		t.Fatalf("expected strategy for prod")
	}
	if got.Strategy != AgentDispatchStrategyPull {
		t.Fatalf("unexpected strategy %+v", got)
	}
	effective := store.EffectiveStrategy("staging")
	if effective.Strategy != AgentDispatchStrategyHybrid {
		t.Fatalf("expected default hybrid strategy, got %+v", effective)
	}
}
