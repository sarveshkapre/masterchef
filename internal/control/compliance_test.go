package control

import (
	"strings"
	"testing"
	"time"
)

func TestComplianceProfileScanAndEvidence(t *testing.T) {
	store := NewComplianceStore()
	profile, err := store.CreateProfile(ComplianceProfileInput{
		Name:      "baseline-cis-linux",
		Framework: "cis",
		Version:   "1.0.0",
		Controls: []ComplianceControl{
			{ID: "CIS-1.1", Description: "filesystem permissions", Severity: "high"},
			{ID: "CIS-1.2", Description: "package integrity", Severity: "medium"},
		},
	})
	if err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	if profile.ID == "" {
		t.Fatalf("expected profile id")
	}

	scan, err := store.RunScan(ComplianceScanInput{
		ProfileID:   profile.ID,
		TargetKind:  "host",
		TargetName:  "prod-1",
		Team:        "payments",
		Environment: "prod",
		Service:     "checkout",
	})
	if err != nil {
		t.Fatalf("run scan failed: %v", err)
	}
	if scan.ID == "" || len(scan.Findings) != 2 {
		t.Fatalf("unexpected scan: %+v", scan)
	}

	jsonOut, ct, err := store.ExportEvidence(scan.ID, "json")
	if err != nil || ct != "application/json" || !strings.Contains(string(jsonOut), scan.ID) {
		t.Fatalf("unexpected json export result: err=%v ct=%s payload=%s", err, ct, string(jsonOut))
	}
	csvOut, ct, err := store.ExportEvidence(scan.ID, "csv")
	if err != nil || ct != "text/csv" || !strings.Contains(string(csvOut), "scan_id,profile_id") {
		t.Fatalf("unexpected csv export result: err=%v ct=%s payload=%s", err, ct, string(csvOut))
	}
	sarifOut, ct, err := store.ExportEvidence(scan.ID, "sarif")
	if err != nil || ct != "application/sarif+json" || !strings.Contains(string(sarifOut), `"version": "2.1.0"`) {
		t.Fatalf("unexpected sarif export result: err=%v ct=%s payload=%s", err, ct, string(sarifOut))
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	ex, err := store.CreateException(ComplianceExceptionInput{
		ProfileID:   profile.ID,
		ControlID:   "CIS-1.1",
		TargetKind:  "host",
		TargetName:  "prod-1",
		Reason:      "temporary waiver",
		RequestedBy: "platform-owner",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("create exception failed: %v", err)
	}
	if _, err := store.ApproveException(ex.ID, "security-lead", "approved"); err != nil {
		t.Fatalf("approve exception failed: %v", err)
	}
	scanWithWaiver, err := store.RunScan(ComplianceScanInput{
		ProfileID:   profile.ID,
		TargetKind:  "host",
		TargetName:  "prod-1",
		Team:        "payments",
		Environment: "prod",
		Service:     "checkout",
	})
	if err != nil {
		t.Fatalf("run scan with waiver failed: %v", err)
	}
	hasWaived := false
	for _, finding := range scanWithWaiver.Findings {
		if finding.ControlID == "CIS-1.1" && finding.Status == "waived" {
			hasWaived = true
			break
		}
	}
	if !hasWaived {
		t.Fatalf("expected waived finding after approved exception: %+v", scanWithWaiver.Findings)
	}

	scorecards, err := store.ScorecardsByDimension("team")
	if err != nil {
		t.Fatalf("scorecards by team failed: %v", err)
	}
	if len(scorecards) == 0 || scorecards[0].Dimension != "team" {
		t.Fatalf("expected non-empty team scorecards, got %+v", scorecards)
	}
}

func TestComplianceContinuousConfig(t *testing.T) {
	store := NewComplianceStore()
	profile, err := store.CreateProfile(ComplianceProfileInput{
		Name:      "baseline-stig-linux",
		Framework: "stig",
		Controls: []ComplianceControl{
			{ID: "STIG-1", Description: "secure boot", Severity: "high"},
		},
	})
	if err != nil {
		t.Fatalf("create profile failed: %v", err)
	}

	cfg, err := store.UpsertContinuousConfig(ComplianceContinuousInput{
		ProfileID:       profile.ID,
		TargetKind:      "cluster",
		TargetName:      "prod",
		IntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("upsert continuous config failed: %v", err)
	}
	if cfg.ID == "" || !cfg.Enabled {
		t.Fatalf("unexpected continuous config: %+v", cfg)
	}

	scan, updated, err := store.RunContinuousScan(cfg.ID)
	if err != nil {
		t.Fatalf("run continuous scan failed: %v", err)
	}
	if scan.ID == "" || updated.LastScanID == "" || updated.LastRunAt == nil {
		t.Fatalf("unexpected run continuous response: scan=%+v config=%+v", scan, updated)
	}
}
