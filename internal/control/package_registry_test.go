package control

import (
	"testing"
	"time"
)

func TestPackageRegistryPublishAndVerify(t *testing.T) {
	store := NewPackageRegistryStore()
	artifact, err := store.Publish(PackageArtifactInput{
		Kind:      "module",
		Name:      "core/network",
		Version:   "1.2.3",
		Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		Signed:    true,
		KeyID:     "sigkey-1",
		Signature: "signature",
		Provenance: PackageProvenance{
			SourceRepo:     "github.com/masterchef/modules",
			SourceRef:      "refs/tags/v1.2.3",
			Builder:        "gha://build/123",
			BuildTimestamp: time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("publish artifact failed: %v", err)
	}
	if artifact.ID == "" {
		t.Fatalf("expected artifact id")
	}

	store.SetPolicy(PackageSigningPolicy{
		RequireSigned: true,
		TrustedKeyIDs: []string{"sigkey-1"},
	})
	verified := store.Verify(PackageVerificationInput{ArtifactID: artifact.ID})
	if !verified.Allowed {
		t.Fatalf("expected artifact verification pass: %+v", verified)
	}
}

func TestPackageRegistryVerificationFailure(t *testing.T) {
	store := NewPackageRegistryStore()
	artifact, err := store.Publish(PackageArtifactInput{
		Kind:     "provider",
		Name:     "aws",
		Version:  "0.9.0",
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Signed:   false,
		Metadata: map[string]string{"channel": "edge"},
	})
	if err != nil {
		t.Fatalf("publish artifact failed: %v", err)
	}
	store.SetPolicy(PackageSigningPolicy{RequireSigned: true})
	verified := store.Verify(PackageVerificationInput{ArtifactID: artifact.ID})
	if verified.Allowed {
		t.Fatalf("expected unsigned artifact rejection")
	}
}
