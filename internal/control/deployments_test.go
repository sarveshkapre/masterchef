package control

import "testing"

func TestDeploymentStoreCreateAndList(t *testing.T) {
	store := NewDeploymentStore()
	item, err := store.Create(DeploymentTriggerInput{
		Environment: "staging",
		Branch:      "env/staging",
		ConfigPath:  "staging.yaml",
		Source:      "api",
		Priority:    "high",
		JobID:       "job-1",
	})
	if err != nil {
		t.Fatalf("create deployment failed: %v", err)
	}
	if item.ID == "" || item.Source != "api" || item.Status != DeploymentQueued {
		t.Fatalf("unexpected deployment trigger %+v", item)
	}
	if _, err := store.Create(DeploymentTriggerInput{
		Environment: "staging",
		Branch:      "env/staging",
		ConfigPath:  "staging.yaml",
		Source:      "bad",
	}); err == nil {
		t.Fatalf("expected source validation error")
	}
	out := store.List()
	if len(out) != 1 || out[0].ID != item.ID {
		t.Fatalf("unexpected list output %+v", out)
	}
}
