package control

import "testing"

func TestWorkflowWizardCatalogListAndGet(t *testing.T) {
	catalog := NewWorkflowWizardCatalog()
	items := catalog.List()
	if len(items) < 4 {
		t.Fatalf("expected built-in wizards, got %d", len(items))
	}
	item, err := catalog.Get("rollout")
	if err != nil {
		t.Fatalf("get rollout wizard failed: %v", err)
	}
	if item.ID != "rollout" || len(item.Steps) == 0 {
		t.Fatalf("unexpected rollout wizard: %+v", item)
	}
}

func TestWorkflowWizardCatalogLaunchValidation(t *testing.T) {
	catalog := NewWorkflowWizardCatalog()
	result, err := catalog.Launch("rollback", WorkflowWizardLaunchInput{
		Inputs: map[string]string{
			"run_id": "run-123",
		},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("launch validation failed: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected launch to require rollback_config_path")
	}
	if len(result.MissingInputs) != 1 || result.MissingInputs[0] != "rollback_config_path" {
		t.Fatalf("unexpected missing inputs: %+v", result.MissingInputs)
	}
	if result.NextStepID != "execute-rollback" {
		t.Fatalf("expected execute-rollback step, got %s", result.NextStepID)
	}
	if !result.DryRun {
		t.Fatalf("expected dry-run flag to propagate")
	}

	ready, err := catalog.Launch("rollout", WorkflowWizardLaunchInput{
		Inputs: map[string]string{
			"config_path":        "c.yaml",
			"strategy":           "canary",
			"target_environment": "prod",
		},
	})
	if err != nil {
		t.Fatalf("launch ready-path failed: %v", err)
	}
	if !ready.Ready || len(ready.MissingInputs) != 0 {
		t.Fatalf("expected rollout wizard to be ready: %+v", ready)
	}
}
