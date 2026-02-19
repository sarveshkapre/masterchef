package control

import "testing"

func TestArtifactDistributionStorePolicyAndPlan(t *testing.T) {
	store := NewArtifactDistributionStore()
	policy, err := store.Upsert(ArtifactDistributionPolicyInput{
		Environment:             "prod",
		MaxTransferMbps:         200,
		CacheWarmThresholdMB:    150,
		PreferRegionalCache:     true,
		RelayOnConstrainedLinks: true,
		Compression:             "zstd",
	})
	if err != nil {
		t.Fatalf("upsert artifact distribution policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	large, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "prod",
		ArtifactID:             "bundle-v1",
		ArtifactSizeMB:         500,
		AvailableBandwidthMbps: 100,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if !large.Allowed || large.Strategy != "regional-cache" || !large.PreWarmCache {
		t.Fatalf("expected large artifact to route through cache, got %+v", large)
	}

	constrained, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "prod",
		ArtifactID:             "bundle-v1",
		ArtifactSizeMB:         50,
		AvailableBandwidthMbps: 10,
	})
	if err != nil {
		t.Fatalf("plan on constrained link failed: %v", err)
	}
	if constrained.Strategy != "relay-cache" {
		t.Fatalf("expected relay-cache on constrained link, got %+v", constrained)
	}

	urgent, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "prod",
		ArtifactID:             "hotfix-v3",
		ArtifactSizeMB:         40,
		AvailableBandwidthMbps: 30,
		Urgent:                 true,
	})
	if err != nil {
		t.Fatalf("urgent plan failed: %v", err)
	}
	if urgent.ThrottleMbps != 30 || urgent.Strategy != "direct" {
		t.Fatalf("expected urgent bypass behavior, got %+v", urgent)
	}

	noPolicy, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "dev",
		ArtifactID:             "bundle-v2",
		ArtifactSizeMB:         20,
		AvailableBandwidthMbps: 100,
	})
	if err != nil {
		t.Fatalf("plan without policy failed: %v", err)
	}
	if !noPolicy.Allowed || noPolicy.PolicyID != "" {
		t.Fatalf("expected default planning when policy is absent, got %+v", noPolicy)
	}

	deferred, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "prod",
		ArtifactID:             "bundle-v4",
		ArtifactSizeMB:         20,
		AvailableBandwidthMbps: 0,
	})
	if err != nil {
		t.Fatalf("deferred decision should not return error: %v", err)
	}
	if deferred.Allowed || deferred.Strategy != "defer" {
		t.Fatalf("expected deferred decision on zero bandwidth, got %+v", deferred)
	}
}

func TestArtifactDistributionStoreValidation(t *testing.T) {
	store := NewArtifactDistributionStore()
	if _, err := store.Upsert(ArtifactDistributionPolicyInput{
		Environment:     "prod",
		MaxTransferMbps: 100,
		Compression:     "brotli",
	}); err == nil {
		t.Fatalf("expected invalid compression error")
	}
	if _, err := store.Plan(ArtifactDistributionPlanInput{
		Environment:            "prod",
		ArtifactID:             "",
		ArtifactSizeMB:         20,
		AvailableBandwidthMbps: 20,
	}); err == nil {
		t.Fatalf("expected missing artifact_id validation error")
	}
}
