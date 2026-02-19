package control

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type SSOProvider struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Protocol       string    `json:"protocol"` // oidc|saml
	IssuerURL      string    `json:"issuer_url"`
	ClientID       string    `json:"client_id"`
	RedirectURL    string    `json:"redirect_url"`
	AllowedDomains []string  `json:"allowed_domains,omitempty"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SSOProviderInput struct {
	Name           string   `json:"name"`
	Protocol       string   `json:"protocol"`
	IssuerURL      string   `json:"issuer_url"`
	ClientID       string   `json:"client_id"`
	RedirectURL    string   `json:"redirect_url"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
}

type SSOLoginStartInput struct {
	ProviderID string `json:"provider_id"`
	Email      string `json:"email"`
	RelayState string `json:"relay_state,omitempty"`
}

type SSOLoginStart struct {
	ProviderID string    `json:"provider_id"`
	State      string    `json:"state"`
	AuthURL    string    `json:"auth_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SSOLoginCompleteInput struct {
	State   string   `json:"state"`
	Code    string   `json:"code"`
	Subject string   `json:"subject"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups,omitempty"`
}

type SSOSession struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	Subject    string    `json:"subject"`
	Email      string    `json:"email"`
	Groups     []string  `json:"groups,omitempty"`
	Token      string    `json:"token"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type pendingSSOLogin struct {
	providerID string
	email      string
	relayState string
	expiresAt  time.Time
}

type IdentityStore struct {
	mu           sync.RWMutex
	nextProvider int64
	nextSession  int64
	providers    map[string]*SSOProvider
	pending      map[string]pendingSSOLogin
	sessions     map[string]*SSOSession
}

func NewIdentityStore() *IdentityStore {
	return &IdentityStore{
		providers: map[string]*SSOProvider{},
		pending:   map[string]pendingSSOLogin{},
		sessions:  map[string]*SSOSession{},
	}
}

func (s *IdentityStore) CreateProvider(in SSOProviderInput) (SSOProvider, error) {
	name := strings.TrimSpace(in.Name)
	protocol := strings.ToLower(strings.TrimSpace(in.Protocol))
	issuerURL := strings.TrimSpace(in.IssuerURL)
	clientID := strings.TrimSpace(in.ClientID)
	redirectURL := strings.TrimSpace(in.RedirectURL)
	if name == "" || protocol == "" || issuerURL == "" || clientID == "" || redirectURL == "" {
		return SSOProvider{}, errors.New("name, protocol, issuer_url, client_id, and redirect_url are required")
	}
	if protocol != "oidc" && protocol != "saml" {
		return SSOProvider{}, errors.New("protocol must be oidc or saml")
	}
	if _, err := url.ParseRequestURI(issuerURL); err != nil {
		return SSOProvider{}, errors.New("issuer_url must be valid")
	}
	if _, err := url.ParseRequestURI(redirectURL); err != nil {
		return SSOProvider{}, errors.New("redirect_url must be valid")
	}
	now := time.Now().UTC()
	item := SSOProvider{
		Name:           name,
		Protocol:       protocol,
		IssuerURL:      issuerURL,
		ClientID:       clientID,
		RedirectURL:    redirectURL,
		AllowedDomains: normalizeStringSlice(in.AllowedDomains),
		Enabled:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextProvider++
	item.ID = "sso-provider-" + itoa(s.nextProvider)
	s.providers[item.ID] = &item
	return cloneSSOProvider(item), nil
}

func (s *IdentityStore) ListProviders() []SSOProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SSOProvider, 0, len(s.providers))
	for _, item := range s.providers {
		out = append(out, cloneSSOProvider(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *IdentityStore) GetProvider(id string) (SSOProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.providers[strings.TrimSpace(id)]
	if !ok {
		return SSOProvider{}, false
	}
	return cloneSSOProvider(*item), true
}

func (s *IdentityStore) SetProviderEnabled(id string, enabled bool) (SSOProvider, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.providers[id]
	if !ok {
		return SSOProvider{}, errors.New("sso provider not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	return cloneSSOProvider(*item), nil
}

func (s *IdentityStore) StartLogin(in SSOLoginStartInput) (SSOLoginStart, error) {
	providerID := strings.TrimSpace(in.ProviderID)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if providerID == "" || email == "" {
		return SSOLoginStart{}, errors.New("provider_id and email are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.providers[providerID]
	if !ok {
		return SSOLoginStart{}, errors.New("sso provider not found")
	}
	if !provider.Enabled {
		return SSOLoginStart{}, errors.New("sso provider is disabled")
	}
	if len(provider.AllowedDomains) > 0 && !emailDomainAllowed(email, provider.AllowedDomains) {
		return SSOLoginStart{}, errors.New("email domain not allowed for provider")
	}
	state, err := randomToken(16)
	if err != nil {
		return SSOLoginStart{}, err
	}
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	s.pending[state] = pendingSSOLogin{
		providerID: providerID,
		email:      email,
		relayState: strings.TrimSpace(in.RelayState),
		expiresAt:  expiresAt,
	}
	authURL := provider.IssuerURL + "/authorize?client_id=" + url.QueryEscape(provider.ClientID) + "&redirect_uri=" + url.QueryEscape(provider.RedirectURL) + "&state=" + url.QueryEscape(state)
	return SSOLoginStart{
		ProviderID: providerID,
		State:      state,
		AuthURL:    authURL,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *IdentityStore) CompleteLogin(in SSOLoginCompleteInput) (SSOSession, error) {
	state := strings.TrimSpace(in.State)
	code := strings.TrimSpace(in.Code)
	subject := strings.TrimSpace(in.Subject)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if state == "" || code == "" || subject == "" || email == "" {
		return SSOSession{}, errors.New("state, code, subject, and email are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expirePendingLocked(now)
	pending, ok := s.pending[state]
	if !ok {
		return SSOSession{}, errors.New("login state is invalid or expired")
	}
	delete(s.pending, state)
	provider, ok := s.providers[pending.providerID]
	if !ok {
		return SSOSession{}, errors.New("sso provider not found")
	}
	if len(provider.AllowedDomains) > 0 && !emailDomainAllowed(email, provider.AllowedDomains) {
		return SSOSession{}, errors.New("email domain not allowed for provider")
	}
	token, err := randomToken(24)
	if err != nil {
		return SSOSession{}, err
	}
	s.nextSession++
	item := SSOSession{
		ID:         "sso-session-" + itoa(s.nextSession),
		ProviderID: pending.providerID,
		Subject:    subject,
		Email:      email,
		Groups:     normalizeStringSlice(in.Groups),
		Token:      "mcsso_" + token,
		IssuedAt:   now,
		ExpiresAt:  now.Add(8 * time.Hour),
	}
	s.sessions[item.ID] = &item
	return cloneSSOSession(item), nil
}

func (s *IdentityStore) ListSessions() []SSOSession {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireSessionsLocked(now)
	out := make([]SSOSession, 0, len(s.sessions))
	for _, item := range s.sessions {
		out = append(out, cloneSSOSession(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	return out
}

func (s *IdentityStore) GetSession(id string) (SSOSession, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireSessionsLocked(now)
	item, ok := s.sessions[strings.TrimSpace(id)]
	if !ok {
		return SSOSession{}, false
	}
	return cloneSSOSession(*item), true
}

func (s *IdentityStore) expirePendingLocked(now time.Time) {
	for state, item := range s.pending {
		if !now.Before(item.expiresAt) {
			delete(s.pending, state)
		}
	}
}

func (s *IdentityStore) expireSessionsLocked(now time.Time) {
	for id, item := range s.sessions {
		if !now.Before(item.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

func emailDomainAllowed(email string, allowedDomains []string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	for _, allowed := range allowedDomains {
		if domain == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func randomToken(bytesCount int) (string, error) {
	entropy := make([]byte, bytesCount)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return hex.EncodeToString(entropy), nil
}

func cloneSSOProvider(in SSOProvider) SSOProvider {
	out := in
	out.AllowedDomains = append([]string{}, in.AllowedDomains...)
	return out
}

func cloneSSOSession(in SSOSession) SSOSession {
	out := in
	out.Groups = append([]string{}, in.Groups...)
	return out
}
