package control

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestSignatureAdmission(t *testing.T) {
	store := NewSignatureAdmissionStore()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair failed: %v", err)
	}
	key, err := store.AddKey(SignatureVerificationKeyInput{
		Name:      "release-key",
		Algorithm: "ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(pub),
		Scopes:    []string{"image", "collection"},
	})
	if err != nil {
		t.Fatalf("add key failed: %v", err)
	}
	if key.ID == "" {
		t.Fatalf("expected key id")
	}

	policy, err := store.SetPolicy(SignatureAdmissionPolicy{
		RequireSignedScopes: []string{"image"},
		TrustedKeyIDs:       []string{key.ID},
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if len(policy.RequireSignedScopes) != 1 || policy.RequireSignedScopes[0] != "image" {
		t.Fatalf("unexpected policy %+v", policy)
	}

	digest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	payload := canonicalSignaturePayload("image", "ghcr.io/masterchef/runtime", digest)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(payload)))
	allowed := store.Admit(SignatureAdmissionInput{
		Scope:       "image",
		ArtifactRef: "ghcr.io/masterchef/runtime",
		Digest:      digest,
		KeyID:       key.ID,
		Signature:   signature,
	})
	if !allowed.Allowed || !allowed.Verified {
		t.Fatalf("expected signed image to pass admission: %+v", allowed)
	}

	invalidSig := store.Admit(SignatureAdmissionInput{
		Scope:       "image",
		ArtifactRef: "ghcr.io/masterchef/runtime",
		Digest:      digest,
		KeyID:       key.ID,
		Signature:   base64.StdEncoding.EncodeToString([]byte("invalid")),
	})
	if invalidSig.Allowed {
		t.Fatalf("expected invalid signature to fail admission: %+v", invalidSig)
	}

	unsignedCollection := store.Admit(SignatureAdmissionInput{
		Scope:       "collection",
		ArtifactRef: "masterchef/core-pack",
		Digest:      digest,
	})
	if !unsignedCollection.Allowed {
		t.Fatalf("expected unsigned collection to pass when not required: %+v", unsignedCollection)
	}
}

func TestSignatureAdmissionPolicyValidation(t *testing.T) {
	store := NewSignatureAdmissionStore()
	if _, err := store.SetPolicy(SignatureAdmissionPolicy{
		RequireSignedScopes: []string{"image"},
		TrustedKeyIDs:       []string{"sigkey-unknown"},
	}); err == nil {
		t.Fatalf("expected unknown trusted key to fail")
	}
}
