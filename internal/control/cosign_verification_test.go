package control

import "testing"

func TestCosignVerificationSuccess(t *testing.T) {
	store := NewCosignVerificationStore()
	root, err := store.UpsertTrustRoot(CosignTrustRootInput{
		Name:               "prod-root",
		Issuer:             "https://token.actions.githubusercontent.com",
		Subject:            "repo:masterchef/masterchef",
		RekorPublicKeyRef:  "rekor://prod",
		TransparencyLogURL: "https://rekor.sigstore.dev",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("upsert trust root failed: %v", err)
	}
	store.SetPolicy(CosignPolicy{
		RequireTransparencyLog: true,
		AllowedIssuers:         []string{"https://token.actions.githubusercontent.com"},
		AllowedSubjects:        []string{"repo:masterchef/masterchef"},
		TrustedRootIDs:         []string{root.ID},
	})
	result := store.Verify(CosignVerifyInput{
		ArtifactRef:          "ghcr.io/masterchef/control-plane:v1.0.0",
		Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Signature:            "cosign:bundle-ref",
		TrustedRootID:        root.ID,
		Issuer:               "https://token.actions.githubusercontent.com",
		Subject:              "repo:masterchef/masterchef",
		TransparencyLogIndex: 123,
	})
	if !result.Verified {
		t.Fatalf("expected cosign verify success, got %+v", result)
	}
}

func TestCosignVerificationFailure(t *testing.T) {
	store := NewCosignVerificationStore()
	root, err := store.UpsertTrustRoot(CosignTrustRootInput{
		Name:    "staging-root",
		Issuer:  "https://issuer.example",
		Subject: "repo:acme/service",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("upsert trust root failed: %v", err)
	}
	result := store.Verify(CosignVerifyInput{
		ArtifactRef:   "ghcr.io/masterchef/agent:v1.0.0",
		Digest:        "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Signature:     "bad-signature",
		TrustedRootID: root.ID,
		Issuer:        "https://issuer.example",
		Subject:       "repo:acme/service",
	})
	if result.Verified || len(result.Violations) == 0 {
		t.Fatalf("expected cosign verify failure with violations, got %+v", result)
	}
}
