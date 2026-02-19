package control

import "testing"

func TestBuiltInDeploymentProfiles(t *testing.T) {
	list := BuiltInDeploymentProfiles()
	if len(list) < 2 {
		t.Fatalf("expected built-in deployment profiles")
	}
}

func TestEvaluateDeploymentProfile(t *testing.T) {
	ok, err := EvaluateDeploymentProfile(DeploymentProfileEvaluationInput{
		Profile:            "minimal-footprint",
		ObjectStoreBackend: "filesystem",
		QueueMode:          "embedded",
	})
	if err != nil {
		t.Fatalf("evaluate minimal-footprint failed: %v", err)
	}
	if !ok.Pass {
		t.Fatalf("expected profile pass, got %+v", ok)
	}
	fail, err := EvaluateDeploymentProfile(DeploymentProfileEvaluationInput{
		Profile:            "minimal-footprint",
		ObjectStoreBackend: "s3",
		QueueMode:          "external",
	})
	if err != nil {
		t.Fatalf("evaluate minimal-footprint fail case errored: %v", err)
	}
	if fail.Pass {
		t.Fatalf("expected profile to fail, got %+v", fail)
	}
}
