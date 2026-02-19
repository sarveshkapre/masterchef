package control

import "testing"

func TestImageBakeCreateAndPlan(t *testing.T) {
	store := NewImageBakeStore()
	pipeline, err := store.Create(ImageBakePipelineInput{
		Environment:      "prod",
		Name:             "linux-base",
		Builder:          "packer",
		BaseImage:        "ubuntu-24.04",
		TargetImage:      "masterchef-linux-base",
		ArtifactFormat:   "ami",
		PromoteAfterBake: true,
		Hooks: []ImageBakeHookInput{
			{Stage: "pre_bake", Action: "validate-packages", TimeoutSeconds: 120, Required: true},
			{Stage: "post_promote", Action: "publish-catalog", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("create image bake pipeline failed: %v", err)
	}

	plan, err := store.Plan(ImageBakePlanInput{
		PipelineID: pipeline.ID,
		Region:     "us-west-2",
		BuildRef:   "git:abc123",
	})
	if err != nil {
		t.Fatalf("plan image bake pipeline failed: %v", err)
	}
	if !plan.Allowed {
		t.Fatalf("expected allowed plan, got %+v", plan)
	}
	if len(plan.Steps) < 4 {
		t.Fatalf("expected hook+bake+promote steps, got %+v", plan.Steps)
	}
	if plan.Steps[0].Stage != "pre_bake" || plan.Steps[0].Action != "validate-packages" {
		t.Fatalf("expected pre_bake hook first, got %+v", plan.Steps[0])
	}

	hasBake := false
	hasPromote := false
	hasPostPromote := false
	for _, step := range plan.Steps {
		if step.Stage == "bake" && step.Action == "build-image" {
			hasBake = true
		}
		if step.Stage == "promote" && step.Action == "promote-image" {
			hasPromote = true
		}
		if step.Stage == "post_promote" && step.Action == "publish-catalog" {
			hasPostPromote = true
		}
	}
	if !hasBake || !hasPromote || !hasPostPromote {
		t.Fatalf("expected bake/promote/post_promote steps, got %+v", plan.Steps)
	}
}

func TestImageBakeCreateValidation(t *testing.T) {
	store := NewImageBakeStore()
	if _, err := store.Create(ImageBakePipelineInput{
		Environment: "prod",
		Name:        "linux-base",
		Builder:     "packer",
		BaseImage:   "ubuntu-24.04",
		TargetImage: "masterchef-linux-base",
		Hooks: []ImageBakeHookInput{
			{Stage: "before_bake", Action: "lint"},
		},
	}); err == nil {
		t.Fatalf("expected invalid hook stage validation error")
	}
	if _, err := store.Plan(ImageBakePlanInput{PipelineID: "missing"}); err == nil {
		t.Fatalf("expected missing pipeline plan error")
	}
}
