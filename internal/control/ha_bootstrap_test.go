package control

import "testing"

func TestBuildHABootstrapPlan(t *testing.T) {
	plan, err := BuildHABootstrapPlan(HABootstrapRequest{
		ClusterName:  "prod-control",
		Region:       "us-east-1",
		Replicas:     3,
		ObjectStore:  "s3",
		QueueBackend: "postgres",
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.ClusterName != "prod-control" || plan.Replicas != 3 {
		t.Fatalf("unexpected plan %+v", plan)
	}
	if plan.Command == "" || len(plan.Steps) == 0 {
		t.Fatalf("expected bootstrap command and steps")
	}
}
