package control

import "testing"

func TestModulePolicyHarnessStoreCaseAndRun(t *testing.T) {
	store := NewModulePolicyHarnessStore()
	item, err := store.UpsertCase(ModulePolicyHarnessCaseInput{
		Name: "Policy smoke",
		Kind: "policy",
		Assertions: []ModulePolicyHarnessAssertionInput{
			{Field: "exit_code", Expected: "0"},
			{Field: "resource_count", Expected: "12"},
		},
	})
	if err != nil {
		t.Fatalf("upsert harness case failed: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected case id")
	}

	runPass, err := store.Run(ModulePolicyHarnessRunInput{
		CaseID: item.ID,
		Observed: map[string]string{
			"exit_code":      "0",
			"resource_count": "12",
		},
	})
	if err != nil {
		t.Fatalf("run harness failed: %v", err)
	}
	if runPass.Status != "passed" || runPass.Failed != 0 {
		t.Fatalf("expected passed run, got %+v", runPass)
	}

	runFail, err := store.Run(ModulePolicyHarnessRunInput{
		CaseID: item.ID,
		Observed: map[string]string{
			"exit_code":      "1",
			"resource_count": "12",
		},
	})
	if err != nil {
		t.Fatalf("run harness with failure failed unexpectedly: %v", err)
	}
	if runFail.Status != "failed" || runFail.Passed != 1 || runFail.Failed != 1 {
		t.Fatalf("expected mixed failed run, got %+v", runFail)
	}
}

func TestModulePolicyHarnessStoreValidation(t *testing.T) {
	store := NewModulePolicyHarnessStore()
	if _, err := store.UpsertCase(ModulePolicyHarnessCaseInput{
		Name: "invalid",
		Kind: "unknown",
		Assertions: []ModulePolicyHarnessAssertionInput{
			{Field: "exit_code", Expected: "0"},
		},
	}); err == nil {
		t.Fatalf("expected invalid kind error")
	}
	if _, err := store.Run(ModulePolicyHarnessRunInput{CaseID: "missing"}); err == nil {
		t.Fatalf("expected missing case run error")
	}
}
