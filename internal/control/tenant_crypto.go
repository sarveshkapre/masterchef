package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type TenantCryptoKeyInput struct {
	Tenant    string `json:"tenant"`
	Algorithm string `json:"algorithm,omitempty"`
}

type TenantCryptoKey struct {
	ID          string    `json:"id"`
	Tenant      string    `json:"tenant"`
	Algorithm   string    `json:"algorithm"`
	Version     int       `json:"version"`
	Status      string    `json:"status"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

type TenantKeyRotateInput struct {
	Tenant string `json:"tenant"`
}

type TenantBoundaryCheckInput struct {
	RequestTenant string `json:"request_tenant"`
	KeyID         string `json:"key_id"`
	ContextTenant string `json:"context_tenant,omitempty"`
	Operation     string `json:"operation,omitempty"`
}

type TenantBoundaryDecision struct {
	Allowed bool   `json:"allowed"`
	Tenant  string `json:"tenant,omitempty"`
	KeyID   string `json:"key_id,omitempty"`
	Reason  string `json:"reason"`
}

type TenantCryptoStore struct {
	mu             sync.RWMutex
	nextID         int64
	keysByID       map[string]*TenantCryptoKey
	activeByTenant map[string]string
}

func NewTenantCryptoStore() *TenantCryptoStore {
	return &TenantCryptoStore{
		keysByID:       map[string]*TenantCryptoKey{},
		activeByTenant: map[string]string{},
	}
}

func (s *TenantCryptoStore) EnsureTenantKey(in TenantCryptoKeyInput) (TenantCryptoKey, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	if tenant == "" {
		return TenantCryptoKey{}, errors.New("tenant is required")
	}
	algorithm := strings.ToLower(strings.TrimSpace(in.Algorithm))
	if algorithm == "" {
		algorithm = "aes-256-gcm"
	}
	if algorithm != "aes-256-gcm" && algorithm != "chacha20-poly1305" {
		return TenantCryptoKey{}, errors.New("algorithm must be one of: aes-256-gcm, chacha20-poly1305")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if activeID, ok := s.activeByTenant[tenant]; ok {
		if item, exists := s.keysByID[activeID]; exists {
			return *item, nil
		}
	}

	s.nextID++
	id := "tenant-key-" + itoa(s.nextID)
	item := &TenantCryptoKey{
		ID:          id,
		Tenant:      tenant,
		Algorithm:   algorithm,
		Version:     1,
		Status:      "active",
		Fingerprint: tenant + ":" + algorithm + ":v1",
		CreatedAt:   time.Now().UTC(),
	}
	s.keysByID[id] = item
	s.activeByTenant[tenant] = id
	return *item, nil
}

func (s *TenantCryptoStore) Rotate(in TenantKeyRotateInput) (TenantCryptoKey, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	if tenant == "" {
		return TenantCryptoKey{}, errors.New("tenant is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	activeID, ok := s.activeByTenant[tenant]
	if !ok {
		return TenantCryptoKey{}, errors.New("tenant key not found")
	}
	active, ok := s.keysByID[activeID]
	if !ok {
		return TenantCryptoKey{}, errors.New("active tenant key missing")
	}
	active.Status = "retired"

	s.nextID++
	newID := "tenant-key-" + itoa(s.nextID)
	nextVersion := active.Version + 1
	newKey := &TenantCryptoKey{
		ID:          newID,
		Tenant:      tenant,
		Algorithm:   active.Algorithm,
		Version:     nextVersion,
		Status:      "active",
		Fingerprint: tenant + ":" + active.Algorithm + ":v" + itoa(int64(nextVersion)),
		CreatedAt:   time.Now().UTC(),
	}
	s.keysByID[newID] = newKey
	s.activeByTenant[tenant] = newID
	return *newKey, nil
}

func (s *TenantCryptoStore) List() []TenantCryptoKey {
	s.mu.RLock()
	out := make([]TenantCryptoKey, 0, len(s.keysByID))
	for _, item := range s.keysByID {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant == out[j].Tenant {
			return out[i].Version < out[j].Version
		}
		return out[i].Tenant < out[j].Tenant
	})
	return out
}

func (s *TenantCryptoStore) BoundaryCheck(in TenantBoundaryCheckInput) TenantBoundaryDecision {
	requestTenant := strings.ToLower(strings.TrimSpace(in.RequestTenant))
	contextTenant := strings.ToLower(strings.TrimSpace(in.ContextTenant))
	keyID := strings.TrimSpace(in.KeyID)
	if contextTenant == "" {
		contextTenant = requestTenant
	}
	if requestTenant == "" || keyID == "" {
		return TenantBoundaryDecision{Allowed: false, Reason: "request_tenant and key_id are required"}
	}

	s.mu.RLock()
	key, ok := s.keysByID[keyID]
	s.mu.RUnlock()
	if !ok {
		return TenantBoundaryDecision{Allowed: false, KeyID: keyID, Reason: "tenant key not found"}
	}
	if key.Tenant != requestTenant {
		return TenantBoundaryDecision{
			Allowed: false,
			Tenant:  requestTenant,
			KeyID:   keyID,
			Reason:  "key belongs to a different tenant",
		}
	}
	if contextTenant != requestTenant {
		return TenantBoundaryDecision{
			Allowed: false,
			Tenant:  requestTenant,
			KeyID:   keyID,
			Reason:  "cross-tenant crypto boundary violation",
		}
	}
	if key.Status != "active" {
		return TenantBoundaryDecision{
			Allowed: false,
			Tenant:  requestTenant,
			KeyID:   keyID,
			Reason:  "key is not active",
		}
	}
	return TenantBoundaryDecision{
		Allowed: true,
		Tenant:  requestTenant,
		KeyID:   keyID,
		Reason:  "tenant crypto boundary check passed",
	}
}
