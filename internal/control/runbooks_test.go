package control

import "testing"

func TestRunbookStoreLifecycle(t *testing.T) {
	store := NewRunbookStore()

	rb, err := store.Create(Runbook{
		Name:       "DB emergency rollback",
		TargetType: RunbookTargetConfig,
		ConfigPath: "rollback.yaml",
		RiskLevel:  "high",
		Tags:       []string{"db", "rollback", "db"},
	})
	if err != nil {
		t.Fatalf("create runbook failed: %v", err)
	}
	if rb.Status != RunbookDraft {
		t.Fatalf("expected draft status, got %s", rb.Status)
	}
	if len(rb.Tags) != 2 {
		t.Fatalf("expected deduplicated tags")
	}

	rb, err = store.Approve(rb.ID)
	if err != nil {
		t.Fatalf("approve runbook failed: %v", err)
	}
	if rb.Status != RunbookApproved {
		t.Fatalf("expected approved status, got %s", rb.Status)
	}

	rb, err = store.Deprecate(rb.ID)
	if err != nil {
		t.Fatalf("deprecate runbook failed: %v", err)
	}
	if rb.Status != RunbookDeprecated {
		t.Fatalf("expected deprecated status, got %s", rb.Status)
	}
}

func TestRunbookStoreCatalog(t *testing.T) {
	store := NewRunbookStore()

	dbRunbook, err := store.Create(Runbook{
		Name:       "DB maintenance",
		TargetType: RunbookTargetConfig,
		ConfigPath: "db.yaml",
		RiskLevel:  "high",
		Owner:      "db-team",
		Tags:       []string{"database", "maintenance"},
	})
	if err != nil {
		t.Fatalf("create db runbook failed: %v", err)
	}
	if _, err := store.Approve(dbRunbook.ID); err != nil {
		t.Fatalf("approve db runbook failed: %v", err)
	}

	webRunbook, err := store.Create(Runbook{
		Name:       "Web restart",
		TargetType: RunbookTargetConfig,
		ConfigPath: "web.yaml",
		RiskLevel:  "low",
		Owner:      "web-team",
		Tags:       []string{"web", "restart"},
	})
	if err != nil {
		t.Fatalf("create web runbook failed: %v", err)
	}
	if _, err := store.Approve(webRunbook.ID); err != nil {
		t.Fatalf("approve web runbook failed: %v", err)
	}

	_, err = store.Create(Runbook{
		Name:       "Draft-only",
		TargetType: RunbookTargetConfig,
		ConfigPath: "draft.yaml",
		RiskLevel:  "medium",
		Owner:      "web-team",
	})
	if err != nil {
		t.Fatalf("create draft runbook failed: %v", err)
	}

	catalog := store.Catalog(RunbookCatalogQuery{
		Owner:        "web-team",
		MaxRiskLevel: "medium",
	})
	if len(catalog) != 1 || catalog[0].Name != "Web restart" {
		t.Fatalf("unexpected owner-filtered catalog: %+v", catalog)
	}

	catalog = store.Catalog(RunbookCatalogQuery{
		Tag:          "database",
		MaxRiskLevel: "high",
	})
	if len(catalog) != 1 || catalog[0].Name != "DB maintenance" {
		t.Fatalf("unexpected tag-filtered catalog: %+v", catalog)
	}
}
