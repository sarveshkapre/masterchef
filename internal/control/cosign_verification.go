package control

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type CosignTrustRootInput struct {
	Name               string `json:"name"`
	Issuer             string `json:"issuer"`
	Subject            string `json:"subject"`
	RekorPublicKeyRef  string `json:"rekor_public_key_ref,omitempty"`
	FulcioCertificate  string `json:"fulcio_certificate,omitempty"`
	TransparencyLogURL string `json:"transparency_log_url,omitempty"`
	Enabled            bool   `json:"enabled"`
}

type CosignTrustRoot struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Issuer             string    `json:"issuer"`
	Subject            string    `json:"subject"`
	RekorPublicKeyRef  string    `json:"rekor_public_key_ref,omitempty"`
	FulcioCertificate  string    `json:"fulcio_certificate,omitempty"`
	TransparencyLogURL string    `json:"transparency_log_url,omitempty"`
	Enabled            bool      `json:"enabled"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CosignPolicy struct {
	RequireTransparencyLog bool      `json:"require_transparency_log"`
	AllowedIssuers         []string  `json:"allowed_issuers,omitempty"`
	AllowedSubjects        []string  `json:"allowed_subjects,omitempty"`
	TrustedRootIDs         []string  `json:"trusted_root_ids,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type CosignVerifyInput struct {
	ArtifactRef          string `json:"artifact_ref"`
	Digest               string `json:"digest"`
	Signature            string `json:"signature"`
	TrustedRootID        string `json:"trusted_root_id"`
	Issuer               string `json:"issuer"`
	Subject              string `json:"subject"`
	TransparencyLogIndex int64  `json:"transparency_log_index,omitempty"`
}

type CosignVerifyResult struct {
	Verified            bool     `json:"verified"`
	ArtifactRef         string   `json:"artifact_ref,omitempty"`
	Digest              string   `json:"digest,omitempty"`
	TrustedRootID       string   `json:"trusted_root_id,omitempty"`
	TransparencyChecked bool     `json:"transparency_checked"`
	Reason              string   `json:"reason"`
	Violations          []string `json:"violations,omitempty"`
}

type CosignVerificationStore struct {
	mu       sync.RWMutex
	nextID   int64
	roots    map[string]*CosignTrustRoot
	policy   CosignPolicy
	digestRE *regexp.Regexp
}

func NewCosignVerificationStore() *CosignVerificationStore {
	return &CosignVerificationStore{
		roots: map[string]*CosignTrustRoot{},
		policy: CosignPolicy{
			RequireTransparencyLog: true,
			UpdatedAt:              time.Now().UTC(),
		},
		digestRE: regexp.MustCompile(`^sha256:[a-f0-9]{64}$`),
	}
}

func (s *CosignVerificationStore) UpsertTrustRoot(in CosignTrustRootInput) (CosignTrustRoot, error) {
	name := strings.TrimSpace(in.Name)
	issuer := strings.TrimSpace(in.Issuer)
	subject := strings.TrimSpace(in.Subject)
	if name == "" || issuer == "" || subject == "" {
		return CosignTrustRoot{}, errors.New("name, issuer, and subject are required")
	}
	item := CosignTrustRoot{
		Name:               name,
		Issuer:             issuer,
		Subject:            subject,
		RekorPublicKeyRef:  strings.TrimSpace(in.RekorPublicKeyRef),
		FulcioCertificate:  strings.TrimSpace(in.FulcioCertificate),
		TransparencyLogURL: strings.TrimSpace(in.TransparencyLogURL),
		Enabled:            in.Enabled,
		UpdatedAt:          time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.roots {
		if strings.EqualFold(existing.Name, item.Name) {
			item.ID = existing.ID
			s.roots[item.ID] = &item
			return item, nil
		}
	}
	s.nextID++
	item.ID = "cosign-root-" + itoa(s.nextID)
	s.roots[item.ID] = &item
	return item, nil
}

func (s *CosignVerificationStore) ListTrustRoots() []CosignTrustRoot {
	s.mu.RLock()
	out := make([]CosignTrustRoot, 0, len(s.roots))
	for _, root := range s.roots {
		out = append(out, *root)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *CosignVerificationStore) GetTrustRoot(id string) (CosignTrustRoot, bool) {
	s.mu.RLock()
	item, ok := s.roots[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return CosignTrustRoot{}, false
	}
	return *item, true
}

func (s *CosignVerificationStore) Policy() CosignPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return CosignPolicy{
		RequireTransparencyLog: s.policy.RequireTransparencyLog,
		AllowedIssuers:         append([]string{}, s.policy.AllowedIssuers...),
		AllowedSubjects:        append([]string{}, s.policy.AllowedSubjects...),
		TrustedRootIDs:         append([]string{}, s.policy.TrustedRootIDs...),
		UpdatedAt:              s.policy.UpdatedAt,
	}
}

func (s *CosignVerificationStore) SetPolicy(in CosignPolicy) CosignPolicy {
	policy := CosignPolicy{
		RequireTransparencyLog: in.RequireTransparencyLog,
		AllowedIssuers:         normalizeStringList(in.AllowedIssuers),
		AllowedSubjects:        normalizeStringList(in.AllowedSubjects),
		TrustedRootIDs:         normalizeStringList(in.TrustedRootIDs),
		UpdatedAt:              time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return policy
}

func (s *CosignVerificationStore) Verify(in CosignVerifyInput) CosignVerifyResult {
	artifactRef := strings.TrimSpace(in.ArtifactRef)
	digest := strings.ToLower(strings.TrimSpace(in.Digest))
	signature := strings.TrimSpace(in.Signature)
	rootID := strings.TrimSpace(in.TrustedRootID)
	issuer := strings.TrimSpace(in.Issuer)
	subject := strings.TrimSpace(in.Subject)
	if artifactRef == "" || digest == "" || signature == "" || rootID == "" || issuer == "" || subject == "" {
		return CosignVerifyResult{
			Verified:    false,
			ArtifactRef: artifactRef,
			Digest:      digest,
			Reason:      "artifact_ref, digest, signature, trusted_root_id, issuer, and subject are required",
		}
	}
	if !s.digestRE.MatchString(digest) {
		return CosignVerifyResult{
			Verified:    false,
			ArtifactRef: artifactRef,
			Digest:      digest,
			Reason:      "digest must be immutable sha256 format",
		}
	}

	s.mu.RLock()
	root, ok := s.roots[rootID]
	policy := s.policy
	s.mu.RUnlock()
	if !ok {
		return CosignVerifyResult{
			Verified:      false,
			ArtifactRef:   artifactRef,
			Digest:        digest,
			TrustedRootID: rootID,
			Reason:        "trusted root not found",
		}
	}
	violations := make([]string, 0)
	if !root.Enabled {
		violations = append(violations, "trusted root is disabled")
	}
	if !strings.EqualFold(root.Issuer, issuer) {
		violations = append(violations, "issuer does not match trusted root")
	}
	if !strings.EqualFold(root.Subject, subject) {
		violations = append(violations, "subject does not match trusted root")
	}
	if len(policy.TrustedRootIDs) > 0 && !containsNormalized(policy.TrustedRootIDs, rootID) {
		violations = append(violations, "trusted root is not in policy allowlist")
	}
	if len(policy.AllowedIssuers) > 0 && !containsFold(policy.AllowedIssuers, issuer) {
		violations = append(violations, "issuer not allowed by policy")
	}
	if len(policy.AllowedSubjects) > 0 && !containsFold(policy.AllowedSubjects, subject) {
		violations = append(violations, "subject not allowed by policy")
	}
	transparencyChecked := in.TransparencyLogIndex > 0
	if policy.RequireTransparencyLog && !transparencyChecked {
		violations = append(violations, "transparency log entry is required")
	}
	if !strings.HasPrefix(signature, "cosign:") {
		violations = append(violations, "signature must be cosign bundle format (cosign:...)")
	}
	if len(violations) > 0 {
		return CosignVerifyResult{
			Verified:            false,
			ArtifactRef:         artifactRef,
			Digest:              digest,
			TrustedRootID:       rootID,
			TransparencyChecked: transparencyChecked,
			Reason:              "cosign verification failed",
			Violations:          violations,
		}
	}
	return CosignVerifyResult{
		Verified:            true,
		ArtifactRef:         artifactRef,
		Digest:              digest,
		TrustedRootID:       rootID,
		TransparencyChecked: transparencyChecked,
		Reason:              "cosign signature and trust policy verified",
	}
}

func containsNormalized(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}
