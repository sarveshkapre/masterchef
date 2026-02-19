package control

import "testing"

func TestEvaluateDeploymentPreflightReady(t *testing.T) {
	result, err := EvaluateDeploymentPreflight(DeploymentPreflightValidateInput{
		Profile: "ha",
		Checks: []DeploymentPreflightCheckInput{
			{Dependency: "network", Healthy: true, LatencyMS: 80},
			{Dependency: "dns", Healthy: true, LatencyMS: 20},
			{Dependency: "storage", Healthy: true, LatencyMS: 110},
			{Dependency: "database", Healthy: true, LatencyMS: 90},
			{Dependency: "queue", Healthy: true, LatencyMS: 30},
		},
	})
	if err != nil {
		t.Fatalf("evaluate deployment preflight failed: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected preflight ready, got %+v", result)
	}
}

func TestEvaluateDeploymentPreflightMissingDependency(t *testing.T) {
	result, err := EvaluateDeploymentPreflight(DeploymentPreflightValidateInput{
		Checks: []DeploymentPreflightCheckInput{
			{Dependency: "network", Healthy: true},
			{Dependency: "dns", Healthy: true},
		},
	})
	if err != nil {
		t.Fatalf("evaluate deployment preflight failed: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected preflight not ready due to missing dependencies, got %+v", result)
	}
	if len(result.Missing) == 0 || len(result.BlockingIssues) == 0 {
		t.Fatalf("expected missing and blocking issues, got %+v", result)
	}
}
