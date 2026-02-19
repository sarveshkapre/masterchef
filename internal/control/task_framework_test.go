package control

import "testing"

func TestTaskFrameworkStore_RegisterAndMaskPlan(t *testing.T) {
	store := NewTaskFrameworkStore()
	task, err := store.RegisterTask(TaskDefinitionInput{
		Name:      "Deploy App",
		Module:    "packs/web",
		Action:    "deploy",
		Primitive: "module_action",
		Parameters: []TaskParameterSpec{
			{Name: "service", Type: "string", Required: true},
			{Name: "token", Type: "string", Required: true, Sensitive: true},
			{Name: "replicas", Type: "integer", Default: 2},
		},
	})
	if err != nil {
		t.Fatalf("register task failed: %v", err)
	}

	plan, err := store.RegisterPlan(TaskPlanInput{
		Name: "prod rollout",
		Steps: []TaskPlanStep{
			{
				Name:   "deploy-step",
				TaskID: task.ID,
				Parameters: map[string]any{
					"service": "api",
					"token":   "supersecret",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("register plan failed: %v", err)
	}
	if got := plan.Steps[0].Parameters["token"]; got != "***REDACTED***" {
		t.Fatalf("expected masked token in plan response, got %#v", got)
	}
	if got := plan.Steps[0].Parameters["replicas"]; got != int64(2) {
		t.Fatalf("expected integer default in plan response, got %#v", got)
	}

	stored, ok := store.GetPlan(plan.ID)
	if !ok {
		t.Fatalf("expected stored plan")
	}
	if got := stored.Steps[0].Parameters["token"]; got != "***REDACTED***" {
		t.Fatalf("expected masked token from get, got %#v", got)
	}

	preview, err := store.PreviewPlan(plan.ID, TaskPlanPreviewInput{
		Overrides: map[string]map[string]any{
			"deploy-step": {
				"token": "newsecret",
			},
		},
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if len(preview.Steps) != 1 {
		t.Fatalf("expected one preview step, got %#v", preview)
	}
	if got := preview.Steps[0].Parameters["token"]; got != "***REDACTED***" {
		t.Fatalf("expected masked token in preview, got %#v", got)
	}
	if len(preview.Steps[0].SensitiveFields) != 1 || preview.Steps[0].SensitiveFields[0] != "token" {
		t.Fatalf("expected sensitive field metadata, got %#v", preview.Steps[0].SensitiveFields)
	}
}

func TestTaskFrameworkStore_ParameterValidation(t *testing.T) {
	store := NewTaskFrameworkStore()
	task, err := store.RegisterTask(TaskDefinitionInput{
		Name:   "Scale",
		Module: "packs/scale",
		Action: "set",
		Parameters: []TaskParameterSpec{
			{Name: "replicas", Type: "integer", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("register task failed: %v", err)
	}

	_, err = store.RegisterPlan(TaskPlanInput{
		Name: "bad",
		Steps: []TaskPlanStep{{
			TaskID: task.ID,
			Parameters: map[string]any{
				"replicas": "three",
			},
		}},
	})
	if err == nil {
		t.Fatalf("expected integer validation error")
	}

	_, err = store.RegisterPlan(TaskPlanInput{
		Name: "bad-unknown",
		Steps: []TaskPlanStep{{
			TaskID: task.ID,
			Parameters: map[string]any{
				"replicas": 3,
				"extra":    true,
			},
		}},
	})
	if err == nil {
		t.Fatalf("expected unknown parameter validation error")
	}
}
