package control

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExecutionCredential struct {
	ID         string     `json:"id"`
	Subject    string     `json:"subject"`
	Scopes     []string   `json:"scopes,omitempty"`
	TTLSeconds int        `json:"ttl_seconds"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type ExecutionCredentialIssueInput struct {
	Subject    string   `json:"subject"`
	Scopes     []string `json:"scopes,omitempty"`
	TTLSeconds int      `json:"ttl_seconds,omitempty"`
}

type IssuedExecutionCredential struct {
	Credential ExecutionCredential `json:"credential"`
	Token      string              `json:"token"`
}

type ExecutionCredentialValidationInput struct {
	Token          string   `json:"token"`
	RequiredScopes []string `json:"required_scopes,omitempty"`
}

type ExecutionCredentialValidationResult struct {
	Allowed      bool      `json:"allowed"`
	Reason       string    `json:"reason,omitempty"`
	CredentialID string    `json:"credential_id,omitempty"`
	Subject      string    `json:"subject,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

type executionCredentialRecord struct {
	credential ExecutionCredential
	tokenHash  string
}

type ExecutionCredentialStore struct {
	mu         sync.RWMutex
	nextID     int64
	records    map[string]*executionCredentialRecord
	tokenIndex map[string]string
}

func NewExecutionCredentialStore() *ExecutionCredentialStore {
	return &ExecutionCredentialStore{
		records:    map[string]*executionCredentialRecord{},
		tokenIndex: map[string]string{},
	}
}

func (s *ExecutionCredentialStore) Issue(in ExecutionCredentialIssueInput) (IssuedExecutionCredential, error) {
	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return IssuedExecutionCredential{}, errors.New("subject is required")
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 900
	}
	if ttl < 30 {
		return IssuedExecutionCredential{}, errors.New("ttl_seconds must be >= 30")
	}
	if ttl > 3600 {
		return IssuedExecutionCredential{}, errors.New("ttl_seconds must be <= 3600")
	}
	token, err := generateExecutionCredentialToken()
	if err != nil {
		return IssuedExecutionCredential{}, err
	}
	now := time.Now().UTC()
	cred := ExecutionCredential{
		Subject:    subject,
		Scopes:     normalizeStringSlice(in.Scopes),
		TTLSeconds: ttl,
		IssuedAt:   now,
		ExpiresAt:  now.Add(time.Duration(ttl) * time.Second),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	cred.ID = "execcred-" + itoa(s.nextID)
	tokenHash := hashExecutionCredentialToken(token)
	s.records[cred.ID] = &executionCredentialRecord{
		credential: cred,
		tokenHash:  tokenHash,
	}
	s.tokenIndex[tokenHash] = cred.ID
	return IssuedExecutionCredential{
		Credential: cloneExecutionCredential(cred),
		Token:      token,
	}, nil
}

func (s *ExecutionCredentialStore) List() []ExecutionCredential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ExecutionCredential, 0, len(s.records))
	for _, item := range s.records {
		out = append(out, cloneExecutionCredential(item.credential))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	return out
}

func (s *ExecutionCredentialStore) Get(id string) (ExecutionCredential, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		return ExecutionCredential{}, false
	}
	return cloneExecutionCredential(item.credential), true
}

func (s *ExecutionCredentialStore) Revoke(id string) (ExecutionCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		return ExecutionCredential{}, errors.New("execution credential not found")
	}
	if item.credential.RevokedAt == nil {
		now := time.Now().UTC()
		item.credential.RevokedAt = &now
	}
	return cloneExecutionCredential(item.credential), nil
}

func (s *ExecutionCredentialStore) Validate(in ExecutionCredentialValidationInput) ExecutionCredentialValidationResult {
	return s.validateAt(in, time.Now().UTC())
}

func (s *ExecutionCredentialStore) validateAt(in ExecutionCredentialValidationInput, now time.Time) ExecutionCredentialValidationResult {
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return ExecutionCredentialValidationResult{Allowed: false, Reason: "token is required"}
	}
	tokenHash := hashExecutionCredentialToken(token)
	s.mu.RLock()
	credentialID, ok := s.tokenIndex[tokenHash]
	if !ok {
		s.mu.RUnlock()
		return ExecutionCredentialValidationResult{Allowed: false, Reason: "credential token not recognized"}
	}
	item, ok := s.records[credentialID]
	if !ok {
		s.mu.RUnlock()
		return ExecutionCredentialValidationResult{Allowed: false, Reason: "credential token not recognized"}
	}
	cred := cloneExecutionCredential(item.credential)
	s.mu.RUnlock()
	if cred.RevokedAt != nil {
		return ExecutionCredentialValidationResult{
			Allowed:      false,
			Reason:       "execution credential revoked",
			CredentialID: cred.ID,
			Subject:      cred.Subject,
			ExpiresAt:    cred.ExpiresAt,
		}
	}
	if !now.Before(cred.ExpiresAt) {
		return ExecutionCredentialValidationResult{
			Allowed:      false,
			Reason:       "execution credential expired",
			CredentialID: cred.ID,
			Subject:      cred.Subject,
			ExpiresAt:    cred.ExpiresAt,
		}
	}
	if missing := missingExecutionCredentialScopes(cred.Scopes, in.RequiredScopes); len(missing) > 0 {
		return ExecutionCredentialValidationResult{
			Allowed:      false,
			Reason:       "missing required scopes: " + strings.Join(missing, ","),
			CredentialID: cred.ID,
			Subject:      cred.Subject,
			ExpiresAt:    cred.ExpiresAt,
		}
	}
	return ExecutionCredentialValidationResult{
		Allowed:      true,
		CredentialID: cred.ID,
		Subject:      cred.Subject,
		ExpiresAt:    cred.ExpiresAt,
	}
}

func missingExecutionCredentialScopes(actual, required []string) []string {
	required = normalizeStringSlice(required)
	if len(required) == 0 {
		return nil
	}
	have := map[string]struct{}{}
	for _, scope := range normalizeStringSlice(actual) {
		have[scope] = struct{}{}
	}
	missing := make([]string, 0)
	for _, scope := range required {
		if _, ok := have[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	return missing
}

func cloneExecutionCredential(in ExecutionCredential) ExecutionCredential {
	out := in
	out.Scopes = append([]string{}, in.Scopes...)
	if in.RevokedAt != nil {
		revokedAt := *in.RevokedAt
		out.RevokedAt = &revokedAt
	}
	return out
}

func generateExecutionCredentialToken() (string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return "mcex_" + hex.EncodeToString(entropy), nil
}

func hashExecutionCredentialToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
