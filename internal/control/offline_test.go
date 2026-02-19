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
