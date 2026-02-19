package control

import "testing"

func TestRegionalFailoverDrillStoreRunAndScorecards(t *testing.T) {
	store := NewRegionalFailoverDrillStore()
	if _, err := store.Run(RegionalFailoverDrillInput{Region: "us-east-1", TargetRTOSeconds: 300, SimulatedRecoveryMs: 200000}); err != nil {
		t.Fatalf("run drill failed: %v", err)
	}
	if _, err := store.Run(RegionalFailoverDrillInput{Region: "us-east-1", TargetRTOSeconds: 300, SimulatedRecoveryMs: 400000}); err != nil {
		t.Fatalf("run drill failed: %v", err)
	}
	if _, err := store.Run(RegionalFailoverDrillInput{Region: "eu-west-1", TargetRTOSeconds: 300, SimulatedRecoveryMs: 100000}); err != nil {
		t.Fatalf("run drill failed: %v", err)
	}
	if len(store.List(10)) != 3 {
		t.Fatalf("expected three drill runs")
	}
	cards := store.Scorecards(24 * 365)
	if len(cards) != 2 {
		t.Fatalf("expected two regional scorecards, got %+v", cards)
	}
}
