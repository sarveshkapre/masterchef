package control

import "testing"

func TestMigrationToolingTranslatePlatforms(t *testing.T) {
	store := NewMigrationToolingStore()
	cases := []struct {
		name     string
		platform string
		source   string
	}{
		{
			name:     "chef",
			platform: "chef",
			source:   `package "nginx" do action :install end; service "nginx" do action :start end`,
		},
		{
			name:     "ansible",
			platform: "ansible",
			source:   `- hosts: all; tasks: - name: Install nginx apt: name=nginx state=present; become: true`,
		},
		{
			name:     "puppet",
			platform: "puppet",
			source:   `class web { package { 'nginx': ensure => present } service { 'nginx': ensure => running } }`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, err := store.Translate(MigrationTranslateInput{
				SourcePlatform: tc.platform,
				SourceContent:  tc.source,
			})
			if err != nil {
				t.Fatalf("translate failed: %v", err)
			}
			if item.ID == "" || item.GeneratedConfig == "" {
				t.Fatalf("expected translation output, got %+v", item)
			}
			if len(item.DetectedFeatures) == 0 {
				t.Fatalf("expected detected features for %s translation", tc.platform)
			}
		})
	}
}

func TestMigrationToolingEquivalenceAndDiff(t *testing.T) {
	store := NewMigrationToolingStore()
	item, err := store.Translate(MigrationTranslateInput{
		SourcePlatform: "chef",
		SourceContent:  `recipe "web"; package "nginx"; handler "notify"`,
	})
	if err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	eq, err := store.Equivalence(MigrationEquivalenceInput{
		TranslationID: item.ID,
		SemanticChecks: []MigrationSemanticCheck{
			{Name: "package-install", Expected: "resources", Translated: "resources"},
			{Name: "handler-preserved", Expected: "handlers", Translated: "handlers"},
		},
	})
	if err != nil {
		t.Fatalf("equivalence failed: %v", err)
	}
	if !eq.Pass {
		t.Fatalf("expected semantic equivalence pass, got %+v", eq)
	}
	diff, err := store.DiffReport(item.ID)
	if err != nil {
		t.Fatalf("diff report failed: %v", err)
	}
	if diff.ParityScore <= 0 || len(diff.DiffReport) == 0 {
		t.Fatalf("expected non-empty diff report, got %+v", diff)
	}
}

func TestMigrationToolingDeprecationScan(t *testing.T) {
	store := NewMigrationToolingStore()
	result, err := store.DeprecationScan(MigrationDeprecationScanInput{
		SourcePlatform: "chef",
		Modules: []MigrationDeprecationScanModule{
			{Name: "legacy-cookbook", Severity: "high", EOLDate: "2026-05-01", Replacement: "masterchef-module"},
			{Name: "old-handler", Severity: "medium"},
		},
	})
	if err != nil {
		t.Fatalf("deprecation scan failed: %v", err)
	}
	if result.UrgencyScore <= 0 || len(result.Items) != 2 {
		t.Fatalf("expected scored deprecation scan items, got %+v", result)
	}
}
