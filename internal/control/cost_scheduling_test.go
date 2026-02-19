package control

import "testing"

func TestCostSchedulingStorePolicyAndAdmit(t *testing.T) {
	store := NewCostSchedulingStore()
	policy, err := store.Upsert(CostSchedulingPolicyInput{
		Environment:           "prod",
		MaxCostPerRun:         100,
		MaxHourlyBudget:       500,
		OffPeakCostMultiplier: 0.5,
		ThrottleAbovePercent:  80,
	})
	if err != nil {
		t.Fatalf("upsert cost scheduling policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	allowed, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "prod",
		EstimatedCost: 40,
		HourlySpend:   120,
		QueueDepth:    10,
		Priority:      "high",
	})
	if err != nil {
		t.Fatalf("admit returned error: %v", err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected allowed decision, got %+v", allowed)
	}

	throttled, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "prod",
		EstimatedCost: 10,
		HourlySpend:   410,
		QueueDepth:    20,
	})
	if err != nil {
		t.Fatalf("admit returned error for throttled decision: %v", err)
	}
	if throttled.Allowed || throttled.ThrottleSeconds == 0 {
		t.Fatalf("expected throttled decision, got %+v", throttled)
	}

	rejectedRun, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "prod",
		EstimatedCost: 180,
		HourlySpend:   50,
		QueueDepth:    1,
	})
	if err != nil {
		t.Fatalf("admit returned error for max run cost rejection: %v", err)
	}
	if rejectedRun.Allowed {
		t.Fatalf("expected max cost per run rejection, got %+v", rejectedRun)
	}

	rejectedBudget, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "prod",
		EstimatedCost: 80,
		HourlySpend:   450,
		QueueDepth:    1,
	})
	if err != nil {
		t.Fatalf("admit returned error for budget rejection: %v", err)
	}
	if rejectedBudget.Allowed {
		t.Fatalf("expected budget rejection, got %+v", rejectedBudget)
	}

	offPeak, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "prod",
		EstimatedCost: 80,
		HourlySpend:   100,
		QueueDepth:    1,
		OffPeak:       true,
	})
	if err != nil {
		t.Fatalf("admit returned error for off-peak decision: %v", err)
	}
	if offPeak.EffectiveCost != 40 {
		t.Fatalf("expected discounted off-peak effective cost, got %+v", offPeak)
	}

	noPolicy, err := store.Admit(CostSchedulingAdmissionInput{
		Environment:   "dev",
		EstimatedCost: 25,
		HourlySpend:   10,
	})
	if err != nil {
		t.Fatalf("admit returned error for unconfigured environment: %v", err)
	}
	if !noPolicy.Allowed || noPolicy.PolicyID != "" {
		t.Fatalf("expected default allow for unconfigured environment, got %+v", noPolicy)
	}
}

func TestCostSchedulingStoreValidation(t *testing.T) {
	store := NewCostSchedulingStore()
	if _, err := store.Upsert(CostSchedulingPolicyInput{Environment: "prod"}); err == nil {
		t.Fatalf("expected upsert validation error for missing cost bounds")
	}
	if _, err := store.Admit(CostSchedulingAdmissionInput{Environment: "", EstimatedCost: 1}); err == nil {
		t.Fatalf("expected admit validation error for missing environment")
	}
	if _, err := store.Admit(CostSchedulingAdmissionInput{Environment: "prod", EstimatedCost: -1}); err == nil {
		t.Fatalf("expected admit validation error for negative estimated_cost")
	}
}
