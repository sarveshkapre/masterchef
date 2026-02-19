package control

import "testing"

func TestArtifactDeploymentStoreCreateAndPlan(t *testing.T) {
	store := NewArtifactDeploymentStore()
	item, plan, err := store.Create(ArtifactDeploymentInput{
		Environment: "prod",
		ArtifactRef: "registry/masterchef/api:v1.2.3",
		Checksum:    "sha256:abc123",
		Targets:     []string{"api-1", "api-2", "api-3"},
		StageSize:   2,
	})
	if err != nil {
		t.Fatalf("create artifact deployment failed: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected artifact deployment id")
	}
	if !plan.Allowed || len(plan.Stages) != 2 {
		t.Fatalf("expected 2 rollout stages, got %+v", plan)
	}
}

func TestArtifactDeploymentStoreBlocksMissingChecksum(t *testing.T) {
	store := NewArtifactDeploymentStore()
	_, plan, err := store.Create(ArtifactDeploymentInput{
		Environment: "prod",
		ArtifactRef: "registry/masterchef/api:v1.2.3",
		Targets:     []string{"api-1"},
	})
	if err != nil {
		t.Fatalf("create deployment should succeed before plan check: %v", err)
	}
	if plan.Allowed {
		t.Fatalf("expected missing checksum block, got %+v", plan)
	}
}
