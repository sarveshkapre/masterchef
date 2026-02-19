package control

import "testing"

func TestAgentCatalogStoreCreateAndReplay(t *testing.T) {
	store := NewAgentCatalogStore()
	catalog, err := store.CreateCatalog(AgentCompiledCatalog{
		ConfigPath:  "c.yaml",
		PolicyGroup: "stable",
		AgentIDs:    []string{"agent-1", "agent-2"},
		ConfigSHA:   "abc123",
		Signed:      false,
	})
	if err != nil {
		t.Fatalf("create catalog failed: %v", err)
	}
	if catalog.ID == "" || len(catalog.AgentIDs) != 2 {
		t.Fatalf("unexpected catalog %+v", catalog)
	}
	if _, ok := store.GetCatalog(catalog.ID); !ok {
		t.Fatalf("expected to fetch catalog")
	}

	rec := store.RecordReplay(AgentCatalogReplayRecord{
		CatalogID:    catalog.ID,
		AgentID:      "agent-1",
		Allowed:      true,
		Verified:     true,
		Reason:       "signature verified",
		Disconnected: true,
	})
	if rec.ID == "" || !rec.Allowed {
		t.Fatalf("unexpected replay record %+v", rec)
	}
	if len(store.ListReplays(10)) != 1 {
		t.Fatalf("expected one replay record")
	}
}
