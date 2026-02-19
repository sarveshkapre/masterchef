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
	Visibility string            `json:"visibility"` // public|private
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
	Visibility string            `json:"visibility,omitempty"`
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

type PackageCertificationPolicy struct {
	RequireConformance bool      `json:"require_conformance"`
	MinTestPassRate    float64   `json:"min_test_pass_rate"`
	MaxHighVulns       int       `json:"max_high_vulns"`
	MaxCriticalVulns   int       `json:"max_critical_vulns"`
	RequireSigned      bool      `json:"require_signed"`
	MinMaintainerScore int       `json:"min_maintainer_score"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PackageCertificationInput struct {
	ArtifactID              string  `json:"artifact_id"`
	ConformancePassed       bool    `json:"conformance_passed"`
	TestPassRate            float64 `json:"test_pass_rate"`
	HighVulnerabilities     int     `json:"high_vulnerabilities"`
	CriticalVulnerabilities int     `json:"critical_vulnerabilities"`
	MaintainerScore         int     `json:"maintainer_score"`
}

type PackageCertificationReport struct {
	ID                      string    `json:"id"`
	ArtifactID              string    `json:"artifact_id"`
	Certified               bool      `json:"certified"`
	Tier                    string    `json:"tier,omitempty"`
	Reasons                 []string  `json:"reasons,omitempty"`
	Score                   int       `json:"score"`
	ConformancePassed       bool      `json:"conformance_passed"`
	TestPassRate            float64   `json:"test_pass_rate"`
	HighVulnerabilities     int       `json:"high_vulnerabilities"`
	CriticalVulnerabilities int       `json:"critical_vulnerabilities"`
	MaintainerScore         int       `json:"maintainer_score"`
	CreatedAt               time.Time `json:"created_at"`
}

type PackagePublicationCheckInput struct {
	ArtifactID string `json:"artifact_id"`
	Target     string `json:"target,omitempty"` // public|private
}

type PackagePublicationCheckResult struct {
	Allowed           bool      `json:"allowed"`
	ArtifactID        string    `json:"artifact_id,omitempty"`
	Target            string    `json:"target"`
	Reasons           []string  `json:"reasons,omitempty"`
	CertificationTier string    `json:"certification_tier,omitempty"`
	CheckedAt         time.Time `json:"checked_at"`
}

type MaintainerHealthInput struct {
	Maintainer         string  `json:"maintainer"`
	TestPassRate       float64 `json:"test_pass_rate"`
	IssueLatencyHours  float64 `json:"issue_latency_hours"`
	ReleaseCadenceDays float64 `json:"release_cadence_days"`
	OpenSecurityIssues int     `json:"open_security_issues"`
}

type MaintainerHealthReport struct {
	Maintainer         string    `json:"maintainer"`
	TestPassRate       float64   `json:"test_pass_rate"`
	IssueLatencyHours  float64   `json:"issue_latency_hours"`
	ReleaseCadenceDays float64   `json:"release_cadence_days"`
	OpenSecurityIssues int       `json:"open_security_issues"`
	Score              int       `json:"score"`
	Tier               string    `json:"tier"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PackageProvenanceReportItem struct {
	ArtifactID              string    `json:"artifact_id"`
	Kind                    string    `json:"kind"`
	Name                    string    `json:"name"`
	Version                 string    `json:"version"`
	Digest                  string    `json:"digest"`
	SourceRepo              string    `json:"source_repo,omitempty"`
	SourceRef               string    `json:"source_ref,omitempty"`
	Builder                 string    `json:"builder,omitempty"`
	SBOMDigest              string    `json:"sbom_digest,omitempty"`
	AttestationDigest       string    `json:"attestation_digest,omitempty"`
	Certified               bool      `json:"certified"`
	CertificationTier       string    `json:"certification_tier,omitempty"`
	HighVulnerabilities     int       `json:"high_vulnerabilities"`
	CriticalVulnerabilities int       `json:"critical_vulnerabilities"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type PackageRegistryStore struct {
	mu             sync.RWMutex
	nextID         int64
	nextCertID     int64
	artifacts      map[string]*PackageArtifact
	policy         PackageSigningPolicy
	certPolicy     PackageCertificationPolicy
	certifications map[string]*PackageCertificationReport
	maintainers    map[string]*MaintainerHealthReport
}

func NewPackageRegistryStore() *PackageRegistryStore {
	return &PackageRegistryStore{
		artifacts: map[string]*PackageArtifact{},
		policy: PackageSigningPolicy{
			RequireSigned: true,
			UpdatedAt:     time.Now().UTC(),
		},
		certPolicy: PackageCertificationPolicy{
			RequireConformance: true,
			MinTestPassRate:    0.98,
			MaxHighVulns:       0,
			MaxCriticalVulns:   0,
			RequireSigned:      true,
			MinMaintainerScore: 80,
			UpdatedAt:          time.Now().UTC(),
		},
		certifications: map[string]*PackageCertificationReport{},
		maintainers:    map[string]*MaintainerHealthReport{},
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
	visibility := strings.ToLower(strings.TrimSpace(in.Visibility))
	if visibility == "" {
		visibility = "private"
	}
	if visibility != "public" && visibility != "private" {
		return PackageArtifact{}, errors.New("visibility must be public or private")
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
		Visibility: visibility,
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
	return s.ListArtifactsByVisibility("")
}

func (s *PackageRegistryStore) ListArtifactsByVisibility(visibility string) []PackageArtifact {
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PackageArtifact, 0, len(s.artifacts))
	for _, item := range s.artifacts {
		if visibility != "" && item.Visibility != visibility {
			continue
		}
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

func (s *PackageRegistryStore) CertificationPolicy() PackageCertificationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.certPolicy
}

func (s *PackageRegistryStore) SetCertificationPolicy(policy PackageCertificationPolicy) (PackageCertificationPolicy, error) {
	if policy.MinTestPassRate == 0 {
		policy.MinTestPassRate = 0.98
	}
	if policy.MinTestPassRate < 0 || policy.MinTestPassRate > 1 {
		return PackageCertificationPolicy{}, errors.New("min_test_pass_rate must be between 0 and 1")
	}
	if policy.MaxHighVulns < 0 || policy.MaxCriticalVulns < 0 {
		return PackageCertificationPolicy{}, errors.New("vulnerability thresholds cannot be negative")
	}
	if policy.MinMaintainerScore == 0 {
		policy.MinMaintainerScore = 80
	}
	if policy.MinMaintainerScore < 0 || policy.MinMaintainerScore > 100 {
		return PackageCertificationPolicy{}, errors.New("min_maintainer_score must be between 0 and 100")
	}
	policy.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.certPolicy = policy
	s.mu.Unlock()
	return policy, nil
}

func (s *PackageRegistryStore) Certify(in PackageCertificationInput) (PackageCertificationReport, error) {
	artifactID := strings.TrimSpace(in.ArtifactID)
	if artifactID == "" {
		return PackageCertificationReport{}, errors.New("artifact_id is required")
	}
	if in.TestPassRate < 0 || in.TestPassRate > 1 {
		return PackageCertificationReport{}, errors.New("test_pass_rate must be between 0 and 1")
	}
	if in.HighVulnerabilities < 0 || in.CriticalVulnerabilities < 0 {
		return PackageCertificationReport{}, errors.New("vulnerability counts cannot be negative")
	}
	if in.MaintainerScore < 0 || in.MaintainerScore > 100 {
		return PackageCertificationReport{}, errors.New("maintainer_score must be between 0 and 100")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, ok := s.artifacts[artifactID]
	if !ok {
		return PackageCertificationReport{}, errors.New("artifact not found")
	}
	policy := s.certPolicy
	reasons := make([]string, 0, 6)
	if policy.RequireSigned && !artifact.Signed {
		reasons = append(reasons, "artifact must be signed")
	}
	if policy.RequireConformance && !in.ConformancePassed {
		reasons = append(reasons, "conformance suite must pass")
	}
	if in.TestPassRate < policy.MinTestPassRate {
		reasons = append(reasons, "test pass rate below policy threshold")
	}
	if in.HighVulnerabilities > policy.MaxHighVulns {
		reasons = append(reasons, "high vulnerabilities exceed policy threshold")
	}
	if in.CriticalVulnerabilities > policy.MaxCriticalVulns {
		reasons = append(reasons, "critical vulnerabilities exceed policy threshold")
	}
	if in.MaintainerScore < policy.MinMaintainerScore {
		reasons = append(reasons, "maintainer score below policy threshold")
	}

	score := 100
	score -= maxInt(0, int((policy.MinTestPassRate-in.TestPassRate)*100))
	score -= in.HighVulnerabilities * 5
	score -= in.CriticalVulnerabilities * 20
	if in.MaintainerScore < policy.MinMaintainerScore {
		score -= (policy.MinMaintainerScore - in.MaintainerScore)
	}
	if score < 0 {
		score = 0
	}
	tier := ""
	certified := len(reasons) == 0
	if certified {
		switch {
		case score >= 95:
			tier = "gold"
		case score >= 85:
			tier = "silver"
		default:
			tier = "bronze"
		}
	}

	s.nextCertID++
	report := PackageCertificationReport{
		ID:                      "pkg-cert-" + itoa(s.nextCertID),
		ArtifactID:              artifact.ID,
		Certified:               certified,
		Tier:                    tier,
		Reasons:                 reasons,
		Score:                   score,
		ConformancePassed:       in.ConformancePassed,
		TestPassRate:            in.TestPassRate,
		HighVulnerabilities:     in.HighVulnerabilities,
		CriticalVulnerabilities: in.CriticalVulnerabilities,
		MaintainerScore:         in.MaintainerScore,
		CreatedAt:               time.Now().UTC(),
	}
	s.certifications[artifact.ID] = &report
	return clonePackageCertificationReport(report), nil
}

func (s *PackageRegistryStore) ListCertifications() []PackageCertificationReport {
	s.mu.RLock()
	out := make([]PackageCertificationReport, 0, len(s.certifications))
	for _, item := range s.certifications {
		out = append(out, clonePackageCertificationReport(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *PackageRegistryStore) PublicationGateCheck(in PackagePublicationCheckInput) PackagePublicationCheckResult {
	artifactID := strings.TrimSpace(in.ArtifactID)
	target := strings.ToLower(strings.TrimSpace(in.Target))
	if target == "" {
		target = "public"
	}
	result := PackagePublicationCheckResult{
		ArtifactID: artifactID,
		Target:     target,
		CheckedAt:  time.Now().UTC(),
	}
	if artifactID == "" {
		result.Reasons = append(result.Reasons, "artifact_id is required")
		return result
	}

	s.mu.RLock()
	artifact, ok := s.artifacts[artifactID]
	policy := clonePackageSigningPolicy(s.policy)
	certPolicy := s.certPolicy
	cert, certOK := s.certifications[artifactID]
	s.mu.RUnlock()
	if !ok {
		result.Reasons = append(result.Reasons, "artifact not found")
		return result
	}

	verify := s.Verify(PackageVerificationInput{ArtifactID: artifactID})
	if !verify.Allowed {
		result.Reasons = append(result.Reasons, verify.Reason)
	}
	if target == "public" {
		if artifact.Visibility != "public" {
			result.Reasons = append(result.Reasons, "artifact visibility must be public for public publication")
		}
		if certPolicy.RequireSigned && !artifact.Signed {
			result.Reasons = append(result.Reasons, "public publication requires signed artifact")
		}
		if !certOK || !cert.Certified {
			result.Reasons = append(result.Reasons, "artifact is not certified")
		}
		if artifact.Provenance.SBOMDigest == "" {
			result.Reasons = append(result.Reasons, "sbom digest is required for public publication")
		}
		if artifact.Provenance.AttestationDigest == "" {
			result.Reasons = append(result.Reasons, "attestation digest is required for public publication")
		}
	}
	if target == "private" && policy.RequireSigned && !artifact.Signed {
		result.Reasons = append(result.Reasons, "private publication still requires signed artifact by policy")
	}

	if certOK {
		result.CertificationTier = cert.Tier
	}
	result.Allowed = len(result.Reasons) == 0
	return result
}

func (s *PackageRegistryStore) UpsertMaintainerHealth(in MaintainerHealthInput) (MaintainerHealthReport, error) {
	maintainer := strings.ToLower(strings.TrimSpace(in.Maintainer))
	if maintainer == "" {
		return MaintainerHealthReport{}, errors.New("maintainer is required")
	}
	if in.TestPassRate < 0 || in.TestPassRate > 1 {
		return MaintainerHealthReport{}, errors.New("test_pass_rate must be between 0 and 1")
	}
	if in.IssueLatencyHours < 0 || in.ReleaseCadenceDays < 0 {
		return MaintainerHealthReport{}, errors.New("issue_latency_hours and release_cadence_days must be non-negative")
	}
	if in.OpenSecurityIssues < 0 {
		return MaintainerHealthReport{}, errors.New("open_security_issues cannot be negative")
	}

	score := 100
	score -= maxInt(0, int((1.0-in.TestPassRate)*100))
	score -= maxInt(0, int(in.IssueLatencyHours/12))
	score -= maxInt(0, int(in.ReleaseCadenceDays/14))
	score -= in.OpenSecurityIssues * 10
	if score < 0 {
		score = 0
	}
	tier := "bronze"
	switch {
	case score >= 95:
		tier = "gold"
	case score >= 85:
		tier = "silver"
	}

	report := MaintainerHealthReport{
		Maintainer:         maintainer,
		TestPassRate:       in.TestPassRate,
		IssueLatencyHours:  in.IssueLatencyHours,
		ReleaseCadenceDays: in.ReleaseCadenceDays,
		OpenSecurityIssues: in.OpenSecurityIssues,
		Score:              score,
		Tier:               tier,
		UpdatedAt:          time.Now().UTC(),
	}
	s.mu.Lock()
	s.maintainers[maintainer] = &report
	s.mu.Unlock()
	return report, nil
}

func (s *PackageRegistryStore) ListMaintainerHealth() []MaintainerHealthReport {
	s.mu.RLock()
	out := make([]MaintainerHealthReport, 0, len(s.maintainers))
	for _, item := range s.maintainers {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func (s *PackageRegistryStore) GetMaintainerHealth(maintainer string) (MaintainerHealthReport, bool) {
	s.mu.RLock()
	item, ok := s.maintainers[strings.ToLower(strings.TrimSpace(maintainer))]
	s.mu.RUnlock()
	if !ok {
		return MaintainerHealthReport{}, false
	}
	return *item, true
}

func (s *PackageRegistryStore) ProvenanceReport() []PackageProvenanceReportItem {
	s.mu.RLock()
	out := make([]PackageProvenanceReportItem, 0, len(s.artifacts))
	for _, artifact := range s.artifacts {
		item := PackageProvenanceReportItem{
			ArtifactID:        artifact.ID,
			Kind:              artifact.Kind,
			Name:              artifact.Name,
			Version:           artifact.Version,
			Digest:            artifact.Digest,
			SourceRepo:        artifact.Provenance.SourceRepo,
			SourceRef:         artifact.Provenance.SourceRef,
			Builder:           artifact.Provenance.Builder,
			SBOMDigest:        artifact.Provenance.SBOMDigest,
			AttestationDigest: artifact.Provenance.AttestationDigest,
			UpdatedAt:         artifact.UpdatedAt,
		}
		if cert, ok := s.certifications[artifact.ID]; ok {
			item.Certified = cert.Certified
			item.CertificationTier = cert.Tier
			item.HighVulnerabilities = cert.HighVulnerabilities
			item.CriticalVulnerabilities = cert.CriticalVulnerabilities
			if cert.CreatedAt.After(item.UpdatedAt) {
				item.UpdatedAt = cert.CreatedAt
			}
		}
		out = append(out, item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
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

func clonePackageCertificationReport(in PackageCertificationReport) PackageCertificationReport {
	out := in
	out.Reasons = append([]string{}, in.Reasons...)
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
