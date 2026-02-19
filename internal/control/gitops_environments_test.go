package control

import "testing"

func TestGitOpsEnvironmentStoreUpsertList(t *testing.T) {
	store := NewGitOpsEnvironmentStore()
	item, created, err := store.Upsert(GitOpsEnvironmentUpsert{
		Name:             "Staging",
		Branch:           "env/staging",
		SourceConfigPath: "configs/base.yaml",
		OutputPath:       ".masterchef/materialized/staging.yaml",
		ContentSHA256:    "abc123",
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if !created || item.Name != "staging" {
		t.Fatalf("unexpected created item: created=%t item=%+v", created, item)
	}
	item, created, err = store.Upsert(GitOpsEnvironmentUpsert{
		Name:             "staging",
		Branch:           "env/staging-v2",
		SourceConfigPath: "configs/base.yaml",
		OutputPath:       ".masterchef/materialized/staging.yaml",
		ContentSHA256:    "def456",
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if created || item.Branch != "env/staging-v2" {
		t.Fatalf("expected update, got created=%t item=%+v", created, item)
	}
	list := store.List()
	if len(list) != 1 || list[0].ContentSHA256 != "def456" {
		t.Fatalf("unexpected list output: %+v", list)
	}
}
