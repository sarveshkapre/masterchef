package control

import (
	"testing"
	"time"
)

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

func TestTaskFrameworkStore_AsyncExecution(t *testing.T) {
	store := NewTaskFrameworkStore()
	task, err := store.RegisterTask(TaskDefinitionInput{
		Name:   "Deploy",
		Module: "packs/web",
		Action: "deploy",
		Parameters: []TaskParameterSpec{
			{Name: "service", Type: "string", Required: true},
			{Name: "token", Type: "string", Sensitive: true, Required: true},
		},
	})
	if err != nil {
		t.Fatalf("register task failed: %v", err)
	}
	plan, err := store.RegisterPlan(TaskPlanInput{
		Name: "prod",
		Steps: []TaskPlanStep{
			{
				Name:   "deploy-step",
				TaskID: task.ID,
				Parameters: map[string]any{
					"service": "api",
					"token":   "secret",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("register plan failed: %v", err)
	}
	exec, err := store.StartExecution(TaskExecutionInput{PlanID: plan.ID, TimeoutSeconds: 5, PollIntervalMS: 100})
	if err != nil {
		t.Fatalf("start execution failed: %v", err)
	}
	if exec.Status != TaskExecutionPending {
		t.Fatalf("expected pending execution on start, got %+v", exec)
	}

	var final TaskExecution
	for i := 0; i < 50; i++ {
		current, ok := store.GetExecution(exec.ID)
		if !ok {
			t.Fatalf("execution disappeared")
		}
		if current.Status == TaskExecutionSucceeded {
			final = current
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.Status != TaskExecutionSucceeded {
		t.Fatalf("expected execution to succeed, got %+v", final)
	}
	if len(final.Steps) != 1 {
		t.Fatalf("expected one executed step, got %+v", final)
	}
	if got := final.Steps[0].Parameters["token"]; got != "***REDACTED***" {
		t.Fatalf("expected masked token in execution step, got %#v", got)
	}
}

func TestTaskFrameworkStore_AsyncExecutionTimeout(t *testing.T) {
	store := NewTaskFrameworkStore()
	task, err := store.RegisterTask(TaskDefinitionInput{
		Name:   "Deploy",
		Module: "packs/web",
		Action: "deploy",
		Parameters: []TaskParameterSpec{
			{Name: "service", Type: "string", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("register task failed: %v", err)
	}
	steps := make([]TaskPlanStep, 0, 200)
	for i := 0; i < 200; i++ {
		steps = append(steps, TaskPlanStep{
			Name:   "step-" + itoa(int64(i+1)),
			TaskID: task.ID,
			Parameters: map[string]any{
				"service": "api",
			},
		})
	}
	plan, err := store.RegisterPlan(TaskPlanInput{Name: "big-plan", Steps: steps})
	if err != nil {
		t.Fatalf("register plan failed: %v", err)
	}
	exec, err := store.StartExecution(TaskExecutionInput{
		PlanID:         plan.ID,
		TimeoutSeconds: 1,
		PollIntervalMS: 100,
	})
	if err != nil {
		t.Fatalf("start execution failed: %v", err)
	}

	var final TaskExecution
	for i := 0; i < 200; i++ {
		current, ok := store.GetExecution(exec.ID)
		if !ok {
			t.Fatalf("execution disappeared")
		}
		if current.Status == TaskExecutionTimedOut || current.Status == TaskExecutionSucceeded {
			final = current
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.Status != TaskExecutionTimedOut {
		t.Fatalf("expected timed out execution, got %+v", final)
	}
}

func TestTaskFrameworkStore_ExecutionTagFilters(t *testing.T) {
	store := NewTaskFrameworkStore()
	task, err := store.RegisterTask(TaskDefinitionInput{
		Name:   "Deploy",
		Module: "packs/web",
		Action: "deploy",
		Parameters: []TaskParameterSpec{
			{Name: "service", Type: "string", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("register task failed: %v", err)
	}
	plan, err := store.RegisterPlan(TaskPlanInput{
		Name: "tagged-rollout",
		Steps: []TaskPlanStep{
			{
				Name:   "deploy-api",
				TaskID: task.ID,
				Tags:   []string{"web", "prod"},
				Parameters: map[string]any{
					"service": "api",
				},
			},
			{
				Name:   "deploy-worker",
				TaskID: task.ID,
				Tags:   []string{"web", "canary"},
				Parameters: map[string]any{
					"service": "worker",
				},
			},
			{
				Name:   "migrate-db",
				TaskID: task.ID,
				Tags:   []string{"db", "prod"},
				Parameters: map[string]any{
					"service": "db",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("register plan failed: %v", err)
	}
	exec, err := store.StartExecution(TaskExecutionInput{
		PlanID:      plan.ID,
		IncludeTags: []string{"WEB"},
		ExcludeTags: []string{"canary"},
	})
	if err != nil {
		t.Fatalf("start execution failed: %v", err)
	}

	var final TaskExecution
	for i := 0; i < 50; i++ {
		current, ok := store.GetExecution(exec.ID)
		if !ok {
			t.Fatalf("execution disappeared")
		}
		if current.Status == TaskExecutionSucceeded {
			final = current
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.Status != TaskExecutionSucceeded {
		t.Fatalf("expected execution to succeed, got %+v", final)
	}
	if len(final.IncludeTags) != 1 || final.IncludeTags[0] != "web" {
		t.Fatalf("expected normalized include tags, got %#v", final.IncludeTags)
	}
	if len(final.ExcludeTags) != 1 || final.ExcludeTags[0] != "canary" {
		t.Fatalf("expected normalized exclude tags, got %#v", final.ExcludeTags)
	}
	if final.StepCount != 1 || final.CompletedSteps != 1 || len(final.Steps) != 1 {
		t.Fatalf("expected one filtered step execution, got %+v", final)
	}
	if final.Steps[0].Name != "deploy-api" {
		t.Fatalf("expected deploy-api to run, got %+v", final.Steps[0])
	}
	if len(final.Steps[0].Tags) != 2 || final.Steps[0].Tags[0] != "prod" || final.Steps[0].Tags[1] != "web" {
		t.Fatalf("expected normalized step tags, got %#v", final.Steps[0].Tags)
	}
}
