package control

import "testing"

func TestOfflineStoreModeAndBundles(t *testing.T) {
	store := NewOfflineStore()
	mode := store.SetMode(OfflineModeConfig{Enabled: true, AirGapped: true, MirrorPath: "/mnt/mirror"})
	if !mode.Enabled || !mode.AirGapped {
		t.Fatalf("unexpected mode %+v", mode)
	}
	bundle, err := store.CreateBundle(OfflineBundleInput{
		ManifestSHA: "sha256:abc",
		Items:       []string{"policy/main.yaml", "modules/pkg.tgz"},
		Artifacts:   []string{"registry/pkg@sha256:abc"},
		Signed:      true,
		Signature:   "sig",
	})
	if err != nil {
		t.Fatalf("create bundle failed: %v", err)
	}
	if bundle.ID == "" || !bundle.Signed {
		t.Fatalf("unexpected bundle %+v", bundle)
	}
	if len(store.ListBundles(10)) != 1 {
		t.Fatalf("expected one bundle")
	}
}

func TestOfflineStoreMirrorsAndSync(t *testing.T) {
	store := NewOfflineStore()
	if _, err := store.CreateBundle(OfflineBundleInput{
		ManifestSHA: "sha256:def",
		Items:       []string{"policy/edge.yaml"},
		Artifacts:   []string{"registry/pkg@sha256:def", "registry/no-digest"},
		Signed:      false,
	}); err != nil {
		t.Fatalf("seed bundle failed: %v", err)
	}
	mirror, err := store.UpsertMirror(OfflineMirrorInput{
		Name:            "corp-registry",
		Upstream:        "registry.example.com",
		MirrorPath:      "/srv/mirror",
		IncludePatterns: []string{"masterchef/*"},
	})
	if err != nil {
		t.Fatalf("upsert mirror failed: %v", err)
	}
	if mirror.ID == "" {
		t.Fatalf("expected mirror id")
	}
	result, err := store.SyncMirror(OfflineMirrorSyncInput{MirrorID: mirror.ID})
	if err != nil {
		t.Fatalf("sync mirror failed: %v", err)
	}
	if result.ArtifactCount != 2 || result.SyncedArtifacts != 1 || result.Status != "partial" {
		t.Fatalf("unexpected sync result %+v", result)
	}
	if len(store.ListMirrors()) != 1 {
		t.Fatalf("expected one mirror")
	}
}
