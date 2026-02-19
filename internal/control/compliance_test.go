package control

import (
	"strings"
	"testing"
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
		ProfileID:  profile.ID,
		TargetKind: "host",
		TargetName: "prod-1",
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
