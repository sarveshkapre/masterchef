package control

import "testing"

func TestEphemeralEnvironmentLifecycle(t *testing.T) {
	store := NewEphemeralEnvironmentStore()
	env, err := store.CreateEnvironment(EphemeralTestEnvironmentInput{
		Name:       "integration-pr-42",
		Profile:    "kubernetes",
		NodeCount:  12,
		TTLMinutes: 90,
		CreatedBy:  "ci",
	})
	if err != nil {
		t.Fatalf("create environment failed: %v", err)
	}
	if env.ID == "" || env.Status != "active" {
		t.Fatalf("unexpected environment: %+v", env)
	}

	check, err := store.RunIntegrationCheck(IntegrationCheckInput{
		EnvironmentID: env.ID,
		Suite:         "smoke",
		Seed:          42,
		TriggeredBy:   "ci",
	})
	if err != nil {
		t.Fatalf("run integration check failed: %v", err)
	}
	if check.ID == "" || check.Status == "" {
		t.Fatalf("unexpected integration check result: %+v", check)
	}

	list := store.ListChecks(env.ID, 10)
	if len(list) == 0 || list[0].ID != check.ID {
		t.Fatalf("expected check list to include latest result: %+v", list)
	}

	destroyed, err := store.DestroyEnvironment(env.ID)
	if err != nil {
		t.Fatalf("destroy environment failed: %v", err)
	}
	if destroyed.Status != "destroyed" {
		t.Fatalf("expected destroyed status, got %+v", destroyed)
	}
}

func TestIntegrationCheckRequiresActiveEnvironment(t *testing.T) {
	store := NewEphemeralEnvironmentStore()
	env, err := store.CreateEnvironment(EphemeralTestEnvironmentInput{
		Name:      "integration-pr-99",
		Profile:   "vm",
		NodeCount: 3,
	})
	if err != nil {
		t.Fatalf("create environment failed: %v", err)
	}
	if _, err := store.DestroyEnvironment(env.ID); err != nil {
		t.Fatalf("destroy environment failed: %v", err)
	}
	if _, err := store.RunIntegrationCheck(IntegrationCheckInput{
		EnvironmentID: env.ID,
		Suite:         "full",
	}); err == nil {
		t.Fatalf("expected integration check on destroyed env to fail")
	}
}
