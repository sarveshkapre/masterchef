package control

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type PackageProvenance struct {
	SourceRepo        string    `json:"source_repo,omitempty"`
	SourceRef         string    `json:"source_ref,omitempty"`
	Builder           string    `json:"builder,omitempty"`
	BuildTimestamp    time.Time `json:"build_timestamp,omitempty"`
	SBOMDigest        string    `json:"sbom_digest,omitempty"`
	AttestationDigest string    `json:"attestation_digest,omitempty"`
}

type PackageArtifact struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"` // module|provider
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Digest     string            `json:"digest"`
	Signed     bool              `json:"signed"`
	KeyID      string            `json:"key_id,omitempty"`
	Signature  string            `json:"signature,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Provenance PackageProvenance `json:"provenance"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type PackageArtifactInput struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Digest     string            `json:"digest"`
	Signed     bool              `json:"signed"`
	KeyID      string            `json:"key_id,omitempty"`
	Signature  string            `json:"signature,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Provenance PackageProvenance `json:"provenance"`
}

type PackageSigningPolicy struct {
	RequireSigned bool      `json:"require_signed"`
	TrustedKeyIDs []string  `json:"trusted_key_ids,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PackageVerificationInput struct {
	ArtifactID string `json:"artifact_id"`
}

type PackageVerificationResult struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
}

type PackageRegistryStore struct {
	mu        sync.RWMutex
	nextID    int64
	artifacts map[string]*PackageArtifact
	policy    PackageSigningPolicy
}

func NewPackageRegistryStore() *PackageRegistryStore {
	return &PackageRegistryStore{
		artifacts: map[string]*PackageArtifact{},
		policy: PackageSigningPolicy{
			RequireSigned: true,
			UpdatedAt:     time.Now().UTC(),
		},
	}
}

var packageDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func (s *PackageRegistryStore) Publish(in PackageArtifactInput) (PackageArtifact, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	name := strings.TrimSpace(in.Name)
	version := strings.TrimSpace(in.Version)
	digest := strings.ToLower(strings.TrimSpace(in.Digest))
	if kind == "" || name == "" || version == "" || digest == "" {
		return PackageArtifact{}, errors.New("kind, name, version, and digest are required")
	}
	if kind != "module" && kind != "provider" {
		return PackageArtifact{}, errors.New("kind must be module or provider")
	}
	if !packageDigestPattern.MatchString(digest) {
		return PackageArtifact{}, errors.New("digest must be immutable sha256:<64-hex>")
	}
	if in.Signed {
		if strings.TrimSpace(in.KeyID) == "" || strings.TrimSpace(in.Signature) == "" {
			return PackageArtifact{}, errors.New("key_id and signature are required for signed artifact")
		}
	}
	prov := in.Provenance
	prov.SourceRepo = strings.TrimSpace(prov.SourceRepo)
	prov.SourceRef = strings.TrimSpace(prov.SourceRef)
	prov.Builder = strings.TrimSpace(prov.Builder)
	prov.SBOMDigest = strings.TrimSpace(prov.SBOMDigest)
	prov.AttestationDigest = strings.TrimSpace(prov.AttestationDigest)

	meta := map[string]string{}
	for k, v := range in.Metadata {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		meta[key] = strings.TrimSpace(v)
	}
	now := time.Now().UTC()
	item := PackageArtifact{
		Kind:       kind,
		Name:       name,
		Version:    version,
		Digest:     digest,
		Signed:     in.Signed,
		KeyID:      strings.TrimSpace(in.KeyID),
		Signature:  strings.TrimSpace(in.Signature),
		Metadata:   meta,
		Provenance: prov,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item.ID = "pkg-artifact-" + itoa(s.nextID)
	s.artifacts[item.ID] = &item
	return clonePackageArtifact(item), nil
}

func (s *PackageRegistryStore) ListArtifacts() []PackageArtifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PackageArtifact, 0, len(s.artifacts))
	for _, item := range s.artifacts {
		out = append(out, clonePackageArtifact(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *PackageRegistryStore) GetArtifact(id string) (PackageArtifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.artifacts[strings.TrimSpace(id)]
	if !ok {
		return PackageArtifact{}, false
	}
	return clonePackageArtifact(*item), true
}

func (s *PackageRegistryStore) Policy() PackageSigningPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePackageSigningPolicy(s.policy)
}

func (s *PackageRegistryStore) SetPolicy(policy PackageSigningPolicy) PackageSigningPolicy {
	policy.TrustedKeyIDs = normalizeStringSlice(policy.TrustedKeyIDs)
	policy.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return clonePackageSigningPolicy(policy)
}

func (s *PackageRegistryStore) Verify(in PackageVerificationInput) PackageVerificationResult {
	artifactID := strings.TrimSpace(in.ArtifactID)
	if artifactID == "" {
		return PackageVerificationResult{Allowed: false, Reason: "artifact_id is required"}
	}
	s.mu.RLock()
	artifact, ok := s.artifacts[artifactID]
	policy := clonePackageSigningPolicy(s.policy)
	s.mu.RUnlock()
	if !ok {
		return PackageVerificationResult{Allowed: false, Reason: "artifact not found", ArtifactID: artifactID}
	}
	if policy.RequireSigned && !artifact.Signed {
		return PackageVerificationResult{Allowed: false, Reason: "signed artifact required by policy", ArtifactID: artifact.ID}
	}
	if artifact.Signed {
		if artifact.KeyID == "" || artifact.Signature == "" {
			return PackageVerificationResult{Allowed: false, Reason: "signed artifact missing key/signature", ArtifactID: artifact.ID}
		}
		if len(policy.TrustedKeyIDs) > 0 {
			matched := false
			for _, keyID := range policy.TrustedKeyIDs {
				if keyID == artifact.KeyID {
					matched = true
					break
				}
			}
			if !matched {
				return PackageVerificationResult{Allowed: false, Reason: "artifact signing key not trusted", ArtifactID: artifact.ID}
			}
		}
	}
	return PackageVerificationResult{Allowed: true, ArtifactID: artifact.ID}
}

func clonePackageArtifact(in PackageArtifact) PackageArtifact {
	out := in
	out.Metadata = map[string]string{}
	for k, v := range in.Metadata {
		out.Metadata[k] = v
	}
	return out
}

func clonePackageSigningPolicy(in PackageSigningPolicy) PackageSigningPolicy {
	out := in
	out.TrustedKeyIDs = append([]string{}, in.TrustedKeyIDs...)
	return out
}
