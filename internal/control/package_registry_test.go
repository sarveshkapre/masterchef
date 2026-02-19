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
	if artifact.Visibility != "private" {
		t.Fatalf("expected default private visibility, got %q", artifact.Visibility)
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

func TestPackageRegistryCertificationAndPublicationGates(t *testing.T) {
	store := NewPackageRegistryStore()
	artifact, err := store.Publish(PackageArtifactInput{
		Kind:       "module",
		Name:       "core/security",
		Version:    "2.0.0",
		Digest:     "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		Visibility: "public",
		Signed:     true,
		KeyID:      "sigkey-2",
		Signature:  "sig",
		Provenance: PackageProvenance{
			SourceRepo:        "github.com/masterchef/security",
			SourceRef:         "refs/tags/v2.0.0",
			Builder:           "gha://build/222",
			SBOMDigest:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AttestationDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("publish artifact failed: %v", err)
	}
	if _, err := store.SetCertificationPolicy(PackageCertificationPolicy{
		RequireConformance: true,
		MinTestPassRate:    0.95,
		MaxHighVulns:       0,
		MaxCriticalVulns:   0,
		RequireSigned:      true,
		MinMaintainerScore: 75,
	}); err != nil {
		t.Fatalf("set certification policy failed: %v", err)
	}
	report, err := store.Certify(PackageCertificationInput{
		ArtifactID:              artifact.ID,
		ConformancePassed:       true,
		TestPassRate:            0.99,
		HighVulnerabilities:     0,
		CriticalVulnerabilities: 0,
		MaintainerScore:         90,
	})
	if err != nil {
		t.Fatalf("certify failed: %v", err)
	}
	if !report.Certified || report.Tier == "" {
		t.Fatalf("expected certified report, got %+v", report)
	}
	pubGate := store.PublicationGateCheck(PackagePublicationCheckInput{
		ArtifactID: artifact.ID,
		Target:     "public",
	})
	if !pubGate.Allowed {
		t.Fatalf("expected public publication to pass, got %+v", pubGate)
	}
}

func TestPackageRegistryMaintainerHealthMetrics(t *testing.T) {
	store := NewPackageRegistryStore()
	report, err := store.UpsertMaintainerHealth(MaintainerHealthInput{
		Maintainer:         "platform-team",
		TestPassRate:       0.99,
		IssueLatencyHours:  4,
		ReleaseCadenceDays: 7,
		OpenSecurityIssues: 0,
	})
	if err != nil {
		t.Fatalf("upsert maintainer health failed: %v", err)
	}
	if report.Score <= 0 || report.Tier != "gold" {
		t.Fatalf("unexpected maintainer report %+v", report)
	}
	list := store.ListMaintainerHealth()
	if len(list) != 1 {
		t.Fatalf("expected one maintainer health report")
	}
	got, ok := store.GetMaintainerHealth("platform-team")
	if !ok {
		t.Fatalf("expected maintainer lookup to succeed")
	}
	if got.Maintainer != "platform-team" {
		t.Fatalf("unexpected maintainer lookup %+v", got)
	}
}

func TestPackageRegistryProvenanceReport(t *testing.T) {
	store := NewPackageRegistryStore()
	artifact, err := store.Publish(PackageArtifactInput{
		Kind:      "module",
		Name:      "core/base",
		Version:   "1.0.0",
		Digest:    "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		Signed:    true,
		KeyID:     "sigkey-1",
		Signature: "sig",
		Provenance: PackageProvenance{
			SourceRepo:        "github.com/masterchef/core",
			SourceRef:         "refs/tags/v1.0.0",
			Builder:           "gha://build/44",
			SBOMDigest:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AttestationDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if _, err := store.Certify(PackageCertificationInput{
		ArtifactID:              artifact.ID,
		ConformancePassed:       true,
		TestPassRate:            0.99,
		HighVulnerabilities:     1,
		CriticalVulnerabilities: 0,
		MaintainerScore:         88,
	}); err != nil {
		t.Fatalf("certify failed: %v", err)
	}
	report := store.ProvenanceReport()
	if len(report) != 1 {
		t.Fatalf("expected one provenance report row")
	}
	if report[0].ArtifactID != artifact.ID || report[0].SourceRepo == "" {
		t.Fatalf("unexpected provenance report item %+v", report[0])
	}
}

func TestPackageRegistryVisibilityFilter(t *testing.T) {
	store := NewPackageRegistryStore()
	if _, err := store.Publish(PackageArtifactInput{
		Kind:       "module",
		Name:       "public/pkg",
		Version:    "1.0.0",
		Digest:     "sha256:5555555555555555555555555555555555555555555555555555555555555555",
		Visibility: "public",
		Signed:     true,
		KeyID:      "k1",
		Signature:  "s1",
	}); err != nil {
		t.Fatalf("publish public artifact failed: %v", err)
	}
	if _, err := store.Publish(PackageArtifactInput{
		Kind:       "module",
		Name:       "private/pkg",
		Version:    "1.0.0",
		Digest:     "sha256:6666666666666666666666666666666666666666666666666666666666666666",
		Visibility: "private",
		Signed:     true,
		KeyID:      "k2",
		Signature:  "s2",
	}); err != nil {
		t.Fatalf("publish private artifact failed: %v", err)
	}
	publicOnly := store.ListArtifactsByVisibility("public")
	if len(publicOnly) != 1 || publicOnly[0].Visibility != "public" {
		t.Fatalf("expected single public artifact, got %+v", publicOnly)
	}
}

func TestPackageRegistryQualityScoringAndBadges(t *testing.T) {
	store := NewPackageRegistryStore()
	artifact, err := store.Publish(PackageArtifactInput{
		Kind:       "module",
		Name:       "quality/module",
		Version:    "1.0.0",
		Digest:     "sha256:7777777777777777777777777777777777777777777777777777777777777777",
		Visibility: "public",
		Signed:     true,
		KeyID:      "kq",
		Signature:  "sq",
		Metadata: map[string]string{
			"maintainer": "platform-team",
		},
		Provenance: PackageProvenance{
			SBOMDigest:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AttestationDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if _, err := store.UpsertMaintainerHealth(MaintainerHealthInput{
		Maintainer:         "platform-team",
		TestPassRate:       0.99,
		IssueLatencyHours:  4,
		ReleaseCadenceDays: 7,
		OpenSecurityIssues: 0,
	}); err != nil {
		t.Fatalf("maintainer health upsert failed: %v", err)
	}
	if _, err := store.Certify(PackageCertificationInput{
		ArtifactID:              artifact.ID,
		ConformancePassed:       true,
		TestPassRate:            0.99,
		HighVulnerabilities:     0,
		CriticalVulnerabilities: 0,
		MaintainerScore:         95,
	}); err != nil {
		t.Fatalf("certify failed: %v", err)
	}
	report, err := store.EvaluateQuality(PackageQualityInput{ArtifactID: artifact.ID})
	if err != nil {
		t.Fatalf("evaluate quality failed: %v", err)
	}
	if report.Score <= 0 || report.CraftsmanshipTier == "none" {
		t.Fatalf("expected positive craftsmanship quality report: %+v", report)
	}
	if report.TrustBadge != "trusted" && report.TrustBadge != "verified" {
		t.Fatalf("expected trusted or verified badge, got %+v", report)
	}

	list := store.ListQuality("module")
	if len(list) == 0 {
		t.Fatalf("expected quality report list for modules")
	}
}
