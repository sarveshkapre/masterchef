package control

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type OIDCWorkloadProvider struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IssuerURL string    `json:"issuer_url"`
	Audience  string    `json:"audience"`
	JWKSURL   string    `json:"jwks_url"`
	AllowedSA []string  `json:"allowed_service_accounts,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OIDCWorkloadProviderInput struct {
	Name      string   `json:"name"`
	IssuerURL string   `json:"issuer_url"`
	Audience  string   `json:"audience"`
	JWKSURL   string   `json:"jwks_url"`
	AllowedSA []string `json:"allowed_service_accounts,omitempty"`
}

type OIDCWorkloadExchangeInput struct {
	ProviderID     string `json:"provider_id"`
	SubjectToken   string `json:"subject_token"`
	ServiceAccount string `json:"service_account"`
	Workload       string `json:"workload"`
}

type OIDCWorkloadCredential struct {
	ID             string    `json:"id"`
	ProviderID     string    `json:"provider_id"`
	ServiceAccount string    `json:"service_account"`
	Workload       string    `json:"workload"`
	Token          string    `json:"token"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type OIDCWorkloadStore struct {
	mu           sync.RWMutex
	nextProvider int64
	nextCred     int64
	providers    map[string]*OIDCWorkloadProvider
	creds        map[string]*OIDCWorkloadCredential
}

func NewOIDCWorkloadStore() *OIDCWorkloadStore {
	return &OIDCWorkloadStore{
		providers: map[string]*OIDCWorkloadProvider{},
		creds:     map[string]*OIDCWorkloadCredential{},
	}
}

func (s *OIDCWorkloadStore) CreateProvider(in OIDCWorkloadProviderInput) (OIDCWorkloadProvider, error) {
	name := strings.TrimSpace(in.Name)
	issuer := strings.TrimSpace(in.IssuerURL)
	aud := strings.TrimSpace(in.Audience)
	jwks := strings.TrimSpace(in.JWKSURL)
	if name == "" || issuer == "" || aud == "" || jwks == "" {
		return OIDCWorkloadProvider{}, errors.New("name, issuer_url, audience, and jwks_url are required")
	}
	now := time.Now().UTC()
	item := OIDCWorkloadProvider{
		Name:      name,
		IssuerURL: issuer,
		Audience:  aud,
		JWKSURL:   jwks,
		AllowedSA: normalizeStringSlice(in.AllowedSA),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextProvider++
	item.ID = "oidc-provider-" + itoa(s.nextProvider)
	s.providers[item.ID] = &item
	return cloneOIDCProvider(item), nil
}

func (s *OIDCWorkloadStore) ListProviders() []OIDCWorkloadProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OIDCWorkloadProvider, 0, len(s.providers))
	for _, item := range s.providers {
		out = append(out, cloneOIDCProvider(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *OIDCWorkloadStore) GetProvider(id string) (OIDCWorkloadProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.providers[strings.TrimSpace(id)]
	if !ok {
		return OIDCWorkloadProvider{}, false
	}
	return cloneOIDCProvider(*item), true
}

func (s *OIDCWorkloadStore) Exchange(in OIDCWorkloadExchangeInput) (OIDCWorkloadCredential, error) {
	providerID := strings.TrimSpace(in.ProviderID)
	subjectToken := strings.TrimSpace(in.SubjectToken)
	serviceAccount := strings.TrimSpace(in.ServiceAccount)
	workload := strings.TrimSpace(in.Workload)
	if providerID == "" || subjectToken == "" || serviceAccount == "" || workload == "" {
		return OIDCWorkloadCredential{}, errors.New("provider_id, subject_token, service_account, and workload are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireCredsLocked(time.Now().UTC())
	provider, ok := s.providers[providerID]
	if !ok {
		return OIDCWorkloadCredential{}, errors.New("oidc workload provider not found")
	}
	if len(provider.AllowedSA) > 0 {
		allowed := false
		for _, sa := range provider.AllowedSA {
			if sa == serviceAccount {
				allowed = true
				break
			}
		}
		if !allowed {
			return OIDCWorkloadCredential{}, errors.New("service_account not allowed by provider policy")
		}
	}
	token, err := randomOIDCToken()
	if err != nil {
		return OIDCWorkloadCredential{}, err
	}
	now := time.Now().UTC()
	s.nextCred++
	item := OIDCWorkloadCredential{
		ID:             "oidc-cred-" + itoa(s.nextCred),
		ProviderID:     providerID,
		ServiceAccount: serviceAccount,
		Workload:       workload,
		Token:          "mcoidc_" + token,
		IssuedAt:       now,
		ExpiresAt:      now.Add(15 * time.Minute),
	}
	s.creds[item.ID] = &item
	return cloneOIDCCredential(item), nil
}

func (s *OIDCWorkloadStore) ListCredentials() []OIDCWorkloadCredential {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireCredsLocked(now)
	out := make([]OIDCWorkloadCredential, 0, len(s.creds))
	for _, item := range s.creds {
		out = append(out, cloneOIDCCredential(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	return out
}

func (s *OIDCWorkloadStore) GetCredential(id string) (OIDCWorkloadCredential, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireCredsLocked(now)
	item, ok := s.creds[strings.TrimSpace(id)]
	if !ok {
		return OIDCWorkloadCredential{}, false
	}
	return cloneOIDCCredential(*item), true
}

func (s *OIDCWorkloadStore) expireCredsLocked(now time.Time) {
	for id, item := range s.creds {
		if !now.Before(item.ExpiresAt) {
			delete(s.creds, id)
		}
	}
}

func cloneOIDCProvider(in OIDCWorkloadProvider) OIDCWorkloadProvider {
	out := in
	out.AllowedSA = append([]string{}, in.AllowedSA...)
	return out
}

func cloneOIDCCredential(in OIDCWorkloadCredential) OIDCWorkloadCredential {
	return in
}

func randomOIDCToken() (string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return hex.EncodeToString(entropy), nil
}
