package control

import "testing"

func TestReadinessScorecardStoreCreateAndList(t *testing.T) {
	store := NewReadinessScorecardStore()
	item, err := store.Create(ReadinessScorecardInput{
		Environment: "prod",
		Service:     "payments-api",
		Owner:       "sre-payments",
		Signals: ReadinessSignals{
			QualityScore:          0.97,
			ReliabilityScore:      0.96,
			PerformanceScore:      0.95,
			TestPassRate:          0.995,
			FlakeRate:             0.005,
			OpenCriticalIncidents: 0,
			P95ApplyLatencyMs:     1000,
		},
	})
	if err != nil {
		t.Fatalf("create readiness scorecard failed: %v", err)
	}
	if item.ID == "" || item.Environment != "prod" || item.Service != "payments-api" || item.Grade == "" {
		t.Fatalf("unexpected scorecard payload: %+v", item)
	}
	list := store.List("prod", "payments-api", 10)
	if len(list) == 0 || list[0].ID != item.ID {
		t.Fatalf("expected scorecard list to include created item: %+v", list)
	}
}

func TestReadinessScorecardStoreFailingGrade(t *testing.T) {
	store := NewReadinessScorecardStore()
	item, err := store.Create(ReadinessScorecardInput{
		Environment: "prod",
		Service:     "search-api",
		Signals: ReadinessSignals{
			QualityScore:          0.6,
			ReliabilityScore:      0.5,
			PerformanceScore:      0.6,
			TestPassRate:          0.7,
			FlakeRate:             0.2,
			OpenCriticalIncidents: 3,
			P95ApplyLatencyMs:     999999,
		},
	})
	if err != nil {
		t.Fatalf("create failing readiness scorecard failed: %v", err)
	}
	if item.Report.Pass {
		t.Fatalf("expected failing readiness scorecard report")
	}
	if item.Grade == "A" || item.Grade == "A+" {
		t.Fatalf("expected non-A grade for failing report, got %s", item.Grade)
	}
}
