package control

import "testing"

func TestGitOpsPromotionImmutableArtifactPin(t *testing.T) {
	store := NewGitOpsPromotionStore()
	p, err := store.Create(GitOpsPromotionInput{
		Name:           "service-a",
		Stages:         []string{"staging", "production"},
		ArtifactDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Actor:          "sre",
	})
	if err != nil {
		t.Fatalf("create promotion failed: %v", err)
	}
	if p.CurrentStage != "staging" || p.Status != PromotionStatusInProgress {
		t.Fatalf("unexpected promotion initial state: %+v", p)
	}

	if _, err := store.Advance(p.ID, "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "sre", "bad digest"); err == nil {
		t.Fatalf("expected immutable digest mismatch failure")
	}

	p, err = store.Advance(p.ID, p.ArtifactDigest, "sre", "approved for prod")
	if err != nil {
		t.Fatalf("advance promotion failed: %v", err)
	}
	if p.CurrentStage != "production" || p.Status != PromotionStatusCompleted {
		t.Fatalf("expected completed production stage, got %+v", p)
	}
}
