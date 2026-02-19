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
	rec := store.Record(store.Mode(), AgentDispatchRequest{
		ConfigPath: "c.yaml",
		Priority:   "high",
		Force:      true,
	}, "dispatched", "")
	if rec.ID == "" || rec.Mode != AgentDispatchModeEventBus || rec.Status != "dispatched" {
		t.Fatalf("unexpected dispatch record %+v", rec)
	}
	if len(store.List(10)) != 1 {
		t.Fatalf("expected one dispatch record")
	}
}
