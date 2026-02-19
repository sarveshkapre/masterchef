package control

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type SignatureVerificationKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Algorithm string    `json:"algorithm"`
	PublicKey string    `json:"public_key"`
	Scopes    []string  `json:"scopes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SignatureVerificationKeyInput struct {
	Name      string   `json:"name"`
	Algorithm string   `json:"algorithm,omitempty"`
	PublicKey string   `json:"public_key"`
	Scopes    []string `json:"scopes,omitempty"`
}

type SignatureAdmissionPolicy struct {
	RequireSignedScopes []string  `json:"require_signed_scopes,omitempty"`
	TrustedKeyIDs       []string  `json:"trusted_key_ids,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type SignatureAdmissionInput struct {
	Scope       string `json:"scope"`
	ArtifactRef string `json:"artifact_ref,omitempty"`
	Digest      string `json:"digest,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Payload     string `json:"payload,omitempty"`
}

type SignatureAdmissionResult struct {
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason,omitempty"`
	Scope    string `json:"scope"`
	KeyID    string `json:"key_id,omitempty"`
	Verified bool   `json:"verified"`
}

type signatureVerificationKeyRecord struct {
	item      SignatureVerificationKey
	publicKey ed25519.PublicKey
}

type SignatureAdmissionStore struct {
	mu     sync.RWMutex
	nextID int64
	keys   map[string]*signatureVerificationKeyRecord
	policy SignatureAdmissionPolicy
}

func NewSignatureAdmissionStore() *SignatureAdmissionStore {
	return &SignatureAdmissionStore{
		keys: map[string]*signatureVerificationKeyRecord{},
		policy: SignatureAdmissionPolicy{
			RequireSignedScopes: []string{"image", "collection"},
			UpdatedAt:           time.Now().UTC(),
		},
	}
}

func (s *SignatureAdmissionStore) AddKey(in SignatureVerificationKeyInput) (SignatureVerificationKey, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return SignatureVerificationKey{}, errors.New("name is required")
	}
	algorithm := strings.ToLower(strings.TrimSpace(in.Algorithm))
	if algorithm == "" {
		algorithm = "ed25519"
	}
	if algorithm != "ed25519" {
		return SignatureVerificationKey{}, errors.New("algorithm must be ed25519")
	}
	pubRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.PublicKey))
	if err != nil {
		return SignatureVerificationKey{}, errors.New("public_key must be base64 encoded ed25519 key")
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return SignatureVerificationKey{}, errors.New("public_key must be ed25519 public key bytes")
	}
	now := time.Now().UTC()
	item := SignatureVerificationKey{
		Name:      name,
		Algorithm: algorithm,
		PublicKey: strings.TrimSpace(in.PublicKey),
		Scopes:    normalizeSignatureScopes(in.Scopes),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item.ID = "sigkey-" + itoa(s.nextID)
	s.keys[item.ID] = &signatureVerificationKeyRecord{
		item:      item,
		publicKey: ed25519.PublicKey(append([]byte{}, pubRaw...)),
	}
	return cloneSignatureVerificationKey(item), nil
}

func (s *SignatureAdmissionStore) ListKeys() []SignatureVerificationKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SignatureVerificationKey, 0, len(s.keys))
	for _, item := range s.keys {
		out = append(out, cloneSignatureVerificationKey(item.item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *SignatureAdmissionStore) GetKey(id string) (SignatureVerificationKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.keys[strings.TrimSpace(id)]
	if !ok {
		return SignatureVerificationKey{}, false
	}
	return cloneSignatureVerificationKey(item.item), true
}

func (s *SignatureAdmissionStore) Policy() SignatureAdmissionPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSignatureAdmissionPolicy(s.policy)
}

func (s *SignatureAdmissionStore) SetPolicy(policy SignatureAdmissionPolicy) (SignatureAdmissionPolicy, error) {
	policy.RequireSignedScopes = normalizeSignatureScopes(policy.RequireSignedScopes)
	policy.TrustedKeyIDs = normalizeStringSlice(policy.TrustedKeyIDs)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, keyID := range policy.TrustedKeyIDs {
		if _, ok := s.keys[keyID]; !ok {
			return SignatureAdmissionPolicy{}, errors.New("trusted key not found: " + keyID)
		}
	}
	policy.UpdatedAt = time.Now().UTC()
	s.policy = policy
	return cloneSignatureAdmissionPolicy(policy), nil
}

var signatureDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func (s *SignatureAdmissionStore) Admit(in SignatureAdmissionInput) SignatureAdmissionResult {
	scope := normalizeSignatureScope(in.Scope)
	if scope == "" {
		return SignatureAdmissionResult{Allowed: false, Reason: "scope is required", Scope: scope}
	}
	if !isKnownSignatureScope(scope) {
		return SignatureAdmissionResult{Allowed: false, Reason: "scope must be one of image, collection, module, provider", Scope: scope}
	}
	policy := s.Policy()
	signatureRequired := scopeRequiredByPolicy(policy, scope)
	keyID := strings.TrimSpace(in.KeyID)
	sig := strings.TrimSpace(in.Signature)
	digest := strings.ToLower(strings.TrimSpace(in.Digest))

	if signatureRequired && (keyID == "" || sig == "") {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "signed artifact required by policy for scope " + scope,
			Scope:   scope,
		}
	}
	if digest != "" && !signatureDigestPattern.MatchString(digest) {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "digest must be immutable sha256:<64-hex>",
			Scope:   scope,
			KeyID:   keyID,
		}
	}
	if keyID == "" && sig == "" {
		return SignatureAdmissionResult{Allowed: true, Scope: scope}
	}
	if keyID == "" || sig == "" {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "key_id and signature must both be provided",
			Scope:   scope,
			KeyID:   keyID,
		}
	}

	s.mu.RLock()
	record, ok := s.keys[keyID]
	s.mu.RUnlock()
	if !ok {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "verification key not found",
			Scope:   scope,
			KeyID:   keyID,
		}
	}
	if len(record.item.Scopes) > 0 && !sliceContains(record.item.Scopes, scope) {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "verification key scope does not include " + scope,
			Scope:   scope,
			KeyID:   keyID,
		}
	}
	if len(policy.TrustedKeyIDs) > 0 && !sliceContains(policy.TrustedKeyIDs, keyID) {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "key not trusted by policy",
			Scope:   scope,
			KeyID:   keyID,
		}
	}

	sigRaw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "signature must be base64 encoded",
			Scope:   scope,
			KeyID:   keyID,
		}
	}
	payload := strings.TrimSpace(in.Payload)
	if payload == "" {
		payload = canonicalSignaturePayload(scope, strings.TrimSpace(in.ArtifactRef), digest)
	}
	if !ed25519.Verify(record.publicKey, []byte(payload), sigRaw) {
		return SignatureAdmissionResult{
			Allowed: false,
			Reason:  "signature verification failed",
			Scope:   scope,
			KeyID:   keyID,
		}
	}
	return SignatureAdmissionResult{
		Allowed:  true,
		Scope:    scope,
		KeyID:    keyID,
		Verified: true,
	}
}

func cloneSignatureVerificationKey(in SignatureVerificationKey) SignatureVerificationKey {
	out := in
	out.Scopes = append([]string{}, in.Scopes...)
	return out
}

func cloneSignatureAdmissionPolicy(in SignatureAdmissionPolicy) SignatureAdmissionPolicy {
	out := in
	out.RequireSignedScopes = append([]string{}, in.RequireSignedScopes...)
	out.TrustedKeyIDs = append([]string{}, in.TrustedKeyIDs...)
	return out
}

func normalizeSignatureScopes(in []string) []string {
	in = normalizeStringSlice(in)
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		scope := normalizeSignatureScope(item)
		if scope == "" || !isKnownSignatureScope(scope) {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func normalizeSignatureScope(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}

func isKnownSignatureScope(scope string) bool {
	switch scope {
	case "image", "collection", "module", "provider":
		return true
	default:
		return false
	}
}

func scopeRequiredByPolicy(policy SignatureAdmissionPolicy, scope string) bool {
	return sliceContains(policy.RequireSignedScopes, scope)
}

func canonicalSignaturePayload(scope, artifactRef, digest string) string {
	parts := []string{scope}
	if artifactRef != "" {
		parts = append(parts, artifactRef)
	}
	if digest != "" {
		parts = append(parts, digest)
	}
	return strings.Join(parts, "|")
}

func sliceContains(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}
