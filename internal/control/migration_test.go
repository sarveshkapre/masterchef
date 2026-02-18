package control

import (
	"testing"
	"time"
)

func TestMigrationStoreAssessScoresAndDiff(t *testing.T) {
	store := NewMigrationStore()
	eolSoon := time.Now().UTC().Add(7 * 24 * time.Hour).Format("2006-01-02")

	report, err := store.Assess(MigrationAssessmentRequest{
		SourcePlatform: "chef",
		Workload:       "payments-api",
		UsedFeatures: []string{
			"recipes",
			"data bags",
			"ruby block",
		},
		SemanticChecks: []MigrationSemanticCheck{
			{
				Name:       "idempotency",
				Expected:   "no-op on second run",
				Translated: "changes on second run",
			},
		},
		Deprecations: []MigrationDeprecationInput{
			{
				Name:        "legacy-chef-handler",
				Severity:    "high",
				EOLDate:     eolSoon,
				Replacement: "event hooks",
			},
		},
	})
	if err != nil {
		t.Fatalf("assess failed: %v", err)
	}
	if report.ParityScore != 67 {
		t.Fatalf("expected parity score 67, got %d", report.ParityScore)
	}
	if len(report.Unsupported) != 1 || report.Unsupported[0] != "ruby block" {
		t.Fatalf("unexpected unsupported features: %#v", report.Unsupported)
	}
	if report.RiskScore <= 0 {
		t.Fatalf("expected non-zero risk score")
	}
	if report.UrgencyScore < 60 {
		t.Fatalf("expected urgency score to reflect near-term deprecation, got %d", report.UrgencyScore)
	}
	if len(report.DiffReport) < 2 {
		t.Fatalf("expected diff entries for unsupported feature and semantic mismatch")
	}
}

func TestMigrationStoreListAndGet(t *testing.T) {
	store := NewMigrationStore()
	first, err := store.Assess(MigrationAssessmentRequest{SourcePlatform: "ansible", UsedFeatures: []string{"playbooks"}})
	if err != nil {
		t.Fatalf("first assess failed: %v", err)
	}
	second, err := store.Assess(MigrationAssessmentRequest{SourcePlatform: "ansible", UsedFeatures: []string{"playbooks", "roles"}})
	if err != nil {
		t.Fatalf("second assess failed: %v", err)
	}

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(list))
	}
	if list[0].ID != second.ID {
		t.Fatalf("expected newest report first, got %s", list[0].ID)
	}

	got, ok := store.Get(first.ID)
	if !ok {
		t.Fatalf("expected to get report by id")
	}
	if got.ID != first.ID {
		t.Fatalf("unexpected report id: %s", got.ID)
	}
}
