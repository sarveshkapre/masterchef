package control

import (
	"testing"
	"time"
)

func TestGitOpsPreviewStoreLifecycle(t *testing.T) {
	store := NewGitOpsPreviewStore()
	p, err := store.Create(GitOpsPreviewInput{
		Branch:         "feature/my-change",
		Environment:    "preview-pr-12",
		ConfigPath:     "masterchef.yaml",
		ArtifactDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TTLSeconds:     1,
	})
	if err != nil {
		t.Fatalf("create preview failed: %v", err)
	}
	if p.ID == "" || p.Status != PreviewStatusActive {
		t.Fatalf("unexpected preview: %+v", p)
	}

	if _, err := store.SetStatus(p.ID, PreviewStatusPromoted); err != nil {
		t.Fatalf("set status promoted failed: %v", err)
	}
	got, ok := store.Get(p.ID)
	if !ok || got.Status != PreviewStatusPromoted {
		t.Fatalf("expected promoted preview, got %+v ok=%t", got, ok)
	}

	p2, err := store.Create(GitOpsPreviewInput{
		Branch:     "bugfix/x",
		TTLSeconds: 1,
	})
	if err != nil {
		t.Fatalf("create second preview failed: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	list := store.List(true)
	foundExpired := false
	for _, item := range list {
		if item.ID == p2.ID && item.Status == PreviewStatusExpired {
			foundExpired = true
			break
		}
	}
	if !foundExpired {
		t.Fatalf("expected second preview to expire, got %+v", list)
	}
}

func TestGitOpsPreviewStoreDigestValidation(t *testing.T) {
	store := NewGitOpsPreviewStore()
	if _, err := store.Create(GitOpsPreviewInput{
		Branch:         "x",
		ArtifactDigest: "sha256:not-valid",
	}); err == nil {
		t.Fatalf("expected invalid digest error")
	}
}
