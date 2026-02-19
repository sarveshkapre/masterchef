package control

import "testing"

func TestInventoryDriftStoreAnalyzeAndReconcile(t *testing.T) {
	store := NewInventoryDriftStore()
	report := store.Analyze(InventoryDriftAnalyzeInput{
		Desired: []InventoryDriftHost{
			{Name: "node-a", Labels: map[string]string{"role": "web"}},
			{Name: "node-b", Labels: map[string]string{"role": "db"}},
		},
		Observed: []InventoryDriftHost{
			{Name: "node-a", Labels: map[string]string{"role": "api"}},
			{Name: "node-c", Labels: map[string]string{"role": "cache"}},
		},
	}, true)
	if report.ID == "" {
		t.Fatalf("expected report id")
	}
	if report.Summary["missing"] != 1 || report.Summary["unexpected"] != 1 || report.Summary["label_drift"] != 1 {
		t.Fatalf("unexpected drift summary %+v", report.Summary)
	}
	if len(report.Actions) != 3 {
		t.Fatalf("expected reconcile actions, got %+v", report.Actions)
	}
}
