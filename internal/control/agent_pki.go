package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AgentCertificatePolicy struct {
	AutoApprove        bool              `json:"auto_approve"`
	RequiredAttributes map[string]string `json:"required_attributes,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type AgentCSR struct {
	ID         string            `json:"id"`
	AgentID    string            `json:"agent_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Status     string            `json:"status"` // pending|approved|rejected|issued
	Reason     string            `json:"reason,omitempty"`
	CertID     string            `json:"cert_id,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type AgentCSRInput struct {
	AgentID    string            `json:"agent_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type AgentCertificate struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	Serial    string     `json:"serial"`
	Status    string     `json:"status"` // active|revoked|rotated
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RotatedBy string     `json:"rotated_by,omitempty"`
}

type AgentCertificateExpiryReport struct {
	GeneratedAt   time.Time          `json:"generated_at"`
	WithinHours   int                `json:"within_hours"`
	ExpiringCount int                `json:"expiring_count"`
	Items         []AgentCertificate `json:"items"`
}

type AgentCertificateRenewalResult struct {
	RequestedWithinHours int                `json:"requested_within_hours"`
	RenewedCount         int                `json:"renewed_count"`
	Renewed              []AgentCertificate `json:"renewed"`
}

type AgentPKIStore struct {
	mu       sync.RWMutex
	nextCSR  int64
	nextCert int64
	policy   AgentCertificatePolicy
	csrs     map[string]*AgentCSR
	certs    map[string]*AgentCertificate
}

func NewAgentPKIStore() *AgentPKIStore {
	return &AgentPKIStore{
		policy: AgentCertificatePolicy{
			AutoApprove: false,
			UpdatedAt:   time.Now().UTC(),
		},
		csrs:  map[string]*AgentCSR{},
		certs: map[string]*AgentCertificate{},
	}
}

func (s *AgentPKIStore) Policy() AgentCertificatePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgentCertPolicy(s.policy)
}

func (s *AgentPKIStore) SetPolicy(policy AgentCertificatePolicy) AgentCertificatePolicy {
	req := map[string]string{}
	for k, v := range policy.RequiredAttributes {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		req[key] = strings.TrimSpace(v)
	}
	policy.RequiredAttributes = req
	policy.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return cloneAgentCertPolicy(policy)
}

func (s *AgentPKIStore) SubmitCSR(in AgentCSRInput) (AgentCSR, error) {
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		return AgentCSR{}, errors.New("agent_id is required")
	}
	attrs := map[string]string{}
	for k, v := range in.Attributes {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		attrs[key] = strings.TrimSpace(v)
	}
	now := time.Now().UTC()
	item := AgentCSR{
		AgentID:    agentID,
		Attributes: attrs,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextCSR++
	item.ID = "csr-" + itoa(s.nextCSR)
	s.csrs[item.ID] = &item
	if s.policy.AutoApprove && csrMatchesPolicy(item, s.policy) {
		cert := s.issueCertificateLocked(agentID)
		item.Status = "issued"
		item.CertID = cert.ID
		item.UpdatedAt = time.Now().UTC()
		s.csrs[item.ID] = &item
	}
	return cloneAgentCSR(item), nil
}

func (s *AgentPKIStore) ListCSRs() []AgentCSR {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentCSR, 0, len(s.csrs))
	for _, item := range s.csrs {
		out = append(out, cloneAgentCSR(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *AgentPKIStore) GetCSR(id string) (AgentCSR, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.csrs[strings.TrimSpace(id)]
	if !ok {
		return AgentCSR{}, false
	}
	return cloneAgentCSR(*item), true
}

func (s *AgentPKIStore) DecideCSR(id, decision, reason string) (AgentCSR, error) {
	id = strings.TrimSpace(id)
	decision = strings.ToLower(strings.TrimSpace(decision))
	if id == "" {
		return AgentCSR{}, errors.New("csr id is required")
	}
	if decision != "approve" && decision != "reject" {
		return AgentCSR{}, errors.New("decision must be approve or reject")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.csrs[id]
	if !ok {
		return AgentCSR{}, errors.New("csr not found")
	}
	if item.Status != "pending" {
		return AgentCSR{}, errors.New("csr is not pending")
	}
	if decision == "approve" {
		cert := s.issueCertificateLocked(item.AgentID)
		item.Status = "issued"
		item.CertID = cert.ID
	} else {
		item.Status = "rejected"
		item.Reason = strings.TrimSpace(reason)
	}
	item.UpdatedAt = time.Now().UTC()
	return cloneAgentCSR(*item), nil
}

func (s *AgentPKIStore) ListCertificates() []AgentCertificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentCertificate, 0, len(s.certs))
	for _, item := range s.certs {
		out = append(out, cloneAgentCert(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	return out
}

func (s *AgentPKIStore) RevokeCertificate(id string) (AgentCertificate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentCertificate{}, errors.New("certificate id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.certs[id]
	if !ok {
		return AgentCertificate{}, errors.New("certificate not found")
	}
	if item.Status != "revoked" {
		now := time.Now().UTC()
		item.Status = "revoked"
		item.RevokedAt = &now
	}
	return cloneAgentCert(*item), nil
}

func (s *AgentPKIStore) RotateAgentCertificate(agentID string) (AgentCertificate, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentCertificate{}, errors.New("agent_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *AgentCertificate
	for _, cert := range s.certs {
		if cert.AgentID != agentID || cert.Status != "active" {
			continue
		}
		if latest == nil || cert.IssuedAt.After(latest.IssuedAt) {
			latest = cert
		}
	}
	if latest != nil {
		latest.Status = "rotated"
	}
	newCert := s.issueCertificateLocked(agentID)
	if latest != nil {
		latest.RotatedBy = newCert.ID
	}
	return cloneAgentCert(newCert), nil
}

func (s *AgentPKIStore) ExpiryReport(withinHours int) AgentCertificateExpiryReport {
	if withinHours <= 0 {
		withinHours = 72
	}
	now := time.Now().UTC()
	threshold := now.Add(time.Duration(withinHours) * time.Hour)
	s.mu.RLock()
	items := make([]AgentCertificate, 0)
	for _, cert := range s.certs {
		if cert.Status != "active" {
			continue
		}
		if cert.ExpiresAt.Before(threshold) || cert.ExpiresAt.Equal(threshold) {
			items = append(items, cloneAgentCert(*cert))
		}
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].ExpiresAt.Before(items[j].ExpiresAt) })
	return AgentCertificateExpiryReport{
		GeneratedAt:   now,
		WithinHours:   withinHours,
		ExpiringCount: len(items),
		Items:         items,
	}
}

func (s *AgentPKIStore) RenewExpiring(withinHours int) (AgentCertificateRenewalResult, error) {
	if withinHours <= 0 {
		withinHours = 72
	}
	now := time.Now().UTC()
	threshold := now.Add(time.Duration(withinHours) * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	renewed := make([]AgentCertificate, 0)
	for _, cert := range s.certs {
		if cert.Status != "active" {
			continue
		}
		if cert.ExpiresAt.After(threshold) {
			continue
		}
		cert.Status = "rotated"
		newCert := s.issueCertificateLocked(cert.AgentID)
		cert.RotatedBy = newCert.ID
		renewed = append(renewed, cloneAgentCert(newCert))
	}
	return AgentCertificateRenewalResult{
		RequestedWithinHours: withinHours,
		RenewedCount:         len(renewed),
		Renewed:              renewed,
	}, nil
}

func (s *AgentPKIStore) issueCertificateLocked(agentID string) AgentCertificate {
	s.nextCert++
	now := time.Now().UTC()
	item := AgentCertificate{
		ID:        "cert-" + itoa(s.nextCert),
		AgentID:   agentID,
		Serial:    "SERIAL-" + itoa(s.nextCert),
		Status:    "active",
		IssuedAt:  now,
		ExpiresAt: now.Add(90 * 24 * time.Hour),
	}
	s.certs[item.ID] = &item
	return item
}

func csrMatchesPolicy(csr AgentCSR, policy AgentCertificatePolicy) bool {
	for key, expected := range policy.RequiredAttributes {
		if strings.TrimSpace(csr.Attributes[key]) != expected {
			return false
		}
	}
	return true
}

func cloneAgentCertPolicy(in AgentCertificatePolicy) AgentCertificatePolicy {
	out := in
	out.RequiredAttributes = map[string]string{}
	for k, v := range in.RequiredAttributes {
		out.RequiredAttributes[k] = v
	}
	return out
}

func cloneAgentCSR(in AgentCSR) AgentCSR {
	out := in
	out.Attributes = map[string]string{}
	for k, v := range in.Attributes {
		out.Attributes[k] = v
	}
	return out
}

func cloneAgentCert(in AgentCertificate) AgentCertificate {
	out := in
	if in.RevokedAt != nil {
		revokedAt := *in.RevokedAt
		out.RevokedAt = &revokedAt
	}
	return out
}
