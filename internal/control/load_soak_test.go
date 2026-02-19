package control

import "testing"

func TestLoadSoakSuiteRun(t *testing.T) {
	store := NewLoadSoakStore()
	run, err := store.Run(LoadSoakRunInput{
		SuiteID:     "load-control-plane",
		Seed:        42,
		TriggeredBy: "ci",
	})
	if err != nil {
		t.Fatalf("run load/soak suite failed: %v", err)
	}
	if run.ID == "" || run.SuiteID != "load-control-plane" || run.Status == "" {
		t.Fatalf("unexpected run payload: %+v", run)
	}
	list := store.ListRuns("load-control-plane", 10)
	if len(list) == 0 || list[0].ID != run.ID {
		t.Fatalf("expected run list to include latest run: %+v", list)
	}
}

func TestLoadSoakSuiteUpsert(t *testing.T) {
	store := NewLoadSoakStore()
	suite, err := store.UpsertSuite(LoadSoakSuite{
		ID:                   "soak-control-plane-nightly",
		Name:                 "Control Plane Nightly Soak",
		TargetComponent:      "control-plane",
		Mode:                 "soak",
		DurationMinutes:      360,
		Concurrency:          120,
		TargetThroughputRPS:  180,
		ExpectedP95LatencyMS: 1800,
	})
	if err != nil {
		t.Fatalf("upsert suite failed: %v", err)
	}
	if suite.ID != "soak-control-plane-nightly" || suite.Mode != "soak" {
		t.Fatalf("unexpected suite payload: %+v", suite)
	}
}
