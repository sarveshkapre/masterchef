package control

import "testing"

func TestPatchManagementStorePolicyAndPlan(t *testing.T) {
	store := NewPatchManagementStore()
	policy, err := store.UpsertPolicy(PatchPolicyInput{
		Environment:            "prod",
		WindowStartHourUTC:     1,
		WindowDurationHours:    4,
		MaxParallelHosts:       2,
		AllowedClassifications: []string{"security", "critical"},
		RequireRebootApproval:  true,
	})
	if err != nil {
		t.Fatalf("upsert patch policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected patch policy id")
	}

	blocked := store.Plan(PatchPlanInput{
		Environment: "prod",
		HourUTC:     2,
		Hosts: []PatchHost{
			{ID: "node-1", Classification: "security", NeedsReboot: true},
		},
		RebootApproved: false,
	})
	if blocked.Allowed {
		t.Fatalf("expected reboot approval block, got %+v", blocked)
	}

	plan := store.Plan(PatchPlanInput{
		Environment: "prod",
		HourUTC:     2,
		Hosts: []PatchHost{
			{ID: "node-1", Classification: "security", NeedsReboot: true},
			{ID: "node-2", Classification: "critical", NeedsReboot: false},
			{ID: "node-3", Classification: "feature", NeedsReboot: false},
		},
		RebootApproved: true,
	})
	if !plan.Allowed || len(plan.Waves) != 1 {
		t.Fatalf("unexpected patch plan %+v", plan)
	}
}

func TestPatchManagementStoreWindowValidation(t *testing.T) {
	store := NewPatchManagementStore()
	if _, err := store.UpsertPolicy(PatchPolicyInput{
		Environment:         "prod",
		WindowStartHourUTC:  24,
		WindowDurationHours: 2,
	}); err == nil {
		t.Fatalf("expected invalid window start validation error")
	}
}
