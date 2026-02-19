package control

import "testing"

func TestPolicyPullStoreSourcesAndResults(t *testing.T) {
	store := NewPolicyPullStore()
	cp, err := store.CreateSource(PolicyPullSourceInput{
		Name:    "control-plane",
		Type:    PolicyPullSourceTypeControlPlane,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create control-plane source failed: %v", err)
	}
	if cp.ID == "" || cp.Type != PolicyPullSourceTypeControlPlane {
		t.Fatalf("unexpected control-plane source: %+v", cp)
	}

	if _, err := store.CreateSource(PolicyPullSourceInput{
		Name:             "git",
		Type:             PolicyPullSourceTypeGitSigned,
		URL:              "https://example.com/repo.git",
		RequireSignature: true,
	}); err == nil {
		t.Fatalf("expected validation failure for missing public_key_path")
	}

	gitSrc, err := store.CreateSource(PolicyPullSourceInput{
		Name:             "signed-git",
		Type:             PolicyPullSourceTypeGitSigned,
		URL:              "https://example.com/repo.git",
		Branch:           "main",
		PublicKeyPath:    "keys/policy.pub",
		RequireSignature: true,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create signed git source failed: %v", err)
	}
	if gitSrc.ID == "" || gitSrc.Type != PolicyPullSourceTypeGitSigned {
		t.Fatalf("unexpected git source: %+v", gitSrc)
	}

	if len(store.ListSources()) != 2 {
		t.Fatalf("expected two sources")
	}
	if _, ok := store.GetSource(gitSrc.ID); !ok {
		t.Fatalf("expected to retrieve git source")
	}

	result := store.RecordResult(PolicyPullResultInput{
		SourceID:         gitSrc.ID,
		SourceType:       gitSrc.Type,
		Revision:         "abc123",
		ConfigPath:       "policy/main.yaml",
		ConfigSHA:        "sha256:1",
		Status:           "pulled",
		Verified:         true,
		RequireSignature: true,
	})
	if result.ID == "" || !result.Verified || result.Status != "pulled" {
		t.Fatalf("unexpected pull result: %+v", result)
	}
	if len(store.ListResults(10)) != 1 {
		t.Fatalf("expected one pull result")
	}
}
