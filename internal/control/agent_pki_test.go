package control

import (
	"testing"
	"time"
)

func TestAgentPKIAutoApproveAndRotate(t *testing.T) {
	store := NewAgentPKIStore()
	store.SetPolicy(AgentCertificatePolicy{
		AutoApprove: true,
		RequiredAttributes: map[string]string{
			"env": "prod",
		},
	})
	csr, err := store.SubmitCSR(AgentCSRInput{
		AgentID: "agent-1",
		Attributes: map[string]string{
			"env": "prod",
		},
	})
	if err != nil {
		t.Fatalf("submit csr failed: %v", err)
	}
	if csr.Status != "issued" || csr.CertID == "" {
		t.Fatalf("expected csr auto-issued, got %+v", csr)
	}

	rotated, err := store.RotateAgentCertificate("agent-1")
	if err != nil {
		t.Fatalf("rotate certificate failed: %v", err)
	}
	if rotated.ID == "" || rotated.Status != "active" {
		t.Fatalf("expected rotated active cert, got %+v", rotated)
	}
}

func TestAgentPKIManualApproveRejectAndRevoke(t *testing.T) {
	store := NewAgentPKIStore()
	csr, err := store.SubmitCSR(AgentCSRInput{
		AgentID: "agent-2",
		Attributes: map[string]string{
			"env": "staging",
		},
	})
	if err != nil {
		t.Fatalf("submit csr failed: %v", err)
	}
	if csr.Status != "pending" {
		t.Fatalf("expected pending csr, got %+v", csr)
	}

	approved, err := store.DecideCSR(csr.ID, "approve", "")
	if err != nil {
		t.Fatalf("approve csr failed: %v", err)
	}
	if approved.Status != "issued" || approved.CertID == "" {
		t.Fatalf("expected approved issued csr, got %+v", approved)
	}

	revoked, err := store.RevokeCertificate(approved.CertID)
	if err != nil {
		t.Fatalf("revoke cert failed: %v", err)
	}
	if revoked.Status != "revoked" || revoked.RevokedAt == nil {
		t.Fatalf("expected revoked cert, got %+v", revoked)
	}

	csr2, err := store.SubmitCSR(AgentCSRInput{AgentID: "agent-3"})
	if err != nil {
		t.Fatalf("submit csr2 failed: %v", err)
	}
	rejected, err := store.DecideCSR(csr2.ID, "reject", "manual review failed")
	if err != nil {
		t.Fatalf("reject csr failed: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("expected rejected csr, got %+v", rejected)
	}
}

func TestAgentPKIExpiryReportAndRenew(t *testing.T) {
	store := NewAgentPKIStore()
	csr, err := store.SubmitCSR(AgentCSRInput{AgentID: "agent-exp"})
	if err != nil {
		t.Fatalf("submit csr failed: %v", err)
	}
	issued, err := store.DecideCSR(csr.ID, "approve", "")
	if err != nil {
		t.Fatalf("approve csr failed: %v", err)
	}
	store.mu.Lock()
	cert := store.certs[issued.CertID]
	cert.ExpiresAt = time.Now().UTC().Add(2 * time.Hour)
	store.mu.Unlock()

	report := store.ExpiryReport(4)
	if report.ExpiringCount != 1 {
		t.Fatalf("expected one expiring cert, got %+v", report)
	}
	renewed, err := store.RenewExpiring(4)
	if err != nil {
		t.Fatalf("renew expiring failed: %v", err)
	}
	if renewed.RenewedCount != 1 {
		t.Fatalf("expected one renewed cert, got %+v", renewed)
	}
}
