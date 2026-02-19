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

type DelegationToken struct {
	ID         string     `json:"id"`
	Grantor    string     `json:"grantor"`
	Delegatee  string     `json:"delegatee"`
	PipelineID string     `json:"pipeline_id,omitempty"`
	Scopes     []string   `json:"scopes,omitempty"`
	TTLSeconds int        `json:"ttl_seconds"`
	MaxUses    int        `json:"max_uses"`
	UsedCount  int        `json:"used_count"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type DelegationTokenIssueInput struct {
	Grantor    string   `json:"grantor"`
	Delegatee  string   `json:"delegatee"`
	PipelineID string   `json:"pipeline_id,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	TTLSeconds int      `json:"ttl_seconds,omitempty"`
	MaxUses    int      `json:"max_uses,omitempty"`
}

type IssuedDelegationToken struct {
	Token      string          `json:"token"`
	Delegation DelegationToken `json:"delegation"`
}

type DelegationTokenValidationInput struct {
	Token         string `json:"token"`
	RequiredScope string `json:"required_scope,omitempty"`
}

type DelegationTokenValidationResult struct {
	Allowed       bool      `json:"allowed"`
	Reason        string    `json:"reason,omitempty"`
	DelegationID  string    `json:"delegation_id,omitempty"`
	Grantor       string    `json:"grantor,omitempty"`
	Delegatee     string    `json:"delegatee,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	UsesRemaining int       `json:"uses_remaining,omitempty"`
}

type delegationTokenRecord struct {
	item      DelegationToken
	tokenHash string
}

type DelegationTokenStore struct {
	mu         sync.RWMutex
	nextID     int64
	tokens     map[string]*delegationTokenRecord
	tokenIndex map[string]string
}

func NewDelegationTokenStore() *DelegationTokenStore {
	return &DelegationTokenStore{
		tokens:     map[string]*delegationTokenRecord{},
		tokenIndex: map[string]string{},
	}
}

func (s *DelegationTokenStore) Issue(in DelegationTokenIssueInput) (IssuedDelegationToken, error) {
	grantor := strings.TrimSpace(in.Grantor)
	delegatee := strings.TrimSpace(in.Delegatee)
	if grantor == "" || delegatee == "" {
		return IssuedDelegationToken{}, errors.New("grantor and delegatee are required")
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 900
	}
	if ttl < 60 {
		return IssuedDelegationToken{}, errors.New("ttl_seconds must be >= 60")
	}
	if ttl > 86400 {
		return IssuedDelegationToken{}, errors.New("ttl_seconds must be <= 86400")
	}
	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}
	if maxUses > 100 {
		return IssuedDelegationToken{}, errors.New("max_uses must be <= 100")
	}
	scopes := normalizeStringSlice(in.Scopes)
	if len(scopes) == 0 {
		return IssuedDelegationToken{}, errors.New("at least one scope is required")
	}
	token, err := generateDelegationToken()
	if err != nil {
		return IssuedDelegationToken{}, err
	}
	now := time.Now().UTC()
	item := DelegationToken{
		Grantor:    grantor,
		Delegatee:  delegatee,
		PipelineID: strings.TrimSpace(in.PipelineID),
		Scopes:     scopes,
		TTLSeconds: ttl,
		MaxUses:    maxUses,
		UsedCount:  0,
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Duration(ttl) * time.Second),
	}
	tokenHash := hashDelegationToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item.ID = "delegation-" + itoa(s.nextID)
	s.tokens[item.ID] = &delegationTokenRecord{
		item:      item,
		tokenHash: tokenHash,
	}
	s.tokenIndex[tokenHash] = item.ID
	return IssuedDelegationToken{
		Token:      token,
		Delegation: cloneDelegationToken(item),
	}, nil
}

func (s *DelegationTokenStore) List() []DelegationToken {
	now := time.Now().UTC()
	s.mu.Lock()
	s.cleanupExpiredLocked(now)
	out := make([]DelegationToken, 0, len(s.tokens))
	for _, item := range s.tokens {
		out = append(out, cloneDelegationToken(item.item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *DelegationTokenStore) Get(id string) (DelegationToken, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	item, ok := s.tokens[strings.TrimSpace(id)]
	if !ok {
		return DelegationToken{}, false
	}
	return cloneDelegationToken(item.item), true
}

func (s *DelegationTokenStore) Revoke(id string) (DelegationToken, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DelegationToken{}, errors.New("delegation token id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.tokens[id]
	if !ok {
		return DelegationToken{}, errors.New("delegation token not found")
	}
	if item.item.RevokedAt == nil {
		now := time.Now().UTC()
		item.item.RevokedAt = &now
	}
	return cloneDelegationToken(item.item), nil
}

func (s *DelegationTokenStore) Validate(in DelegationTokenValidationInput) DelegationTokenValidationResult {
	return s.validateAt(in, time.Now().UTC())
}

func (s *DelegationTokenStore) validateAt(in DelegationTokenValidationInput, now time.Time) DelegationTokenValidationResult {
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return DelegationTokenValidationResult{Allowed: false, Reason: "token is required"}
	}
	requiredScope := strings.TrimSpace(in.RequiredScope)
	tokenHash := hashDelegationToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	tokenID, ok := s.tokenIndex[tokenHash]
	if !ok {
		return DelegationTokenValidationResult{Allowed: false, Reason: "delegation token not recognized"}
	}
	record, ok := s.tokens[tokenID]
	if !ok {
		return DelegationTokenValidationResult{Allowed: false, Reason: "delegation token not recognized"}
	}
	if record.item.RevokedAt != nil {
		return validationFromDelegation(record.item, false, "delegation token revoked")
	}
	if !now.Before(record.item.ExpiresAt) {
		return validationFromDelegation(record.item, false, "delegation token expired")
	}
	if requiredScope != "" && !sliceContains(record.item.Scopes, requiredScope) {
		return validationFromDelegation(record.item, false, "required scope not granted")
	}
	if record.item.UsedCount >= record.item.MaxUses {
		return validationFromDelegation(record.item, false, "delegation token exhausted")
	}
	record.item.UsedCount++
	return validationFromDelegation(record.item, true, "")
}

func (s *DelegationTokenStore) cleanupExpiredLocked(now time.Time) {
	for _, record := range s.tokens {
		if record.item.RevokedAt != nil {
			continue
		}
		if !now.Before(record.item.ExpiresAt) {
			expiredAt := record.item.ExpiresAt
			record.item.RevokedAt = &expiredAt
		}
	}
}

func validationFromDelegation(item DelegationToken, allowed bool, reason string) DelegationTokenValidationResult {
	remaining := item.MaxUses - item.UsedCount
	if remaining < 0 {
		remaining = 0
	}
	return DelegationTokenValidationResult{
		Allowed:       allowed,
		Reason:        reason,
		DelegationID:  item.ID,
		Grantor:       item.Grantor,
		Delegatee:     item.Delegatee,
		ExpiresAt:     item.ExpiresAt,
		UsesRemaining: remaining,
	}
}

func cloneDelegationToken(in DelegationToken) DelegationToken {
	out := in
	out.Scopes = append([]string{}, in.Scopes...)
	if in.RevokedAt != nil {
		revokedAt := *in.RevokedAt
		out.RevokedAt = &revokedAt
	}
	return out
}

func generateDelegationToken() (string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return "mcdel_" + hex.EncodeToString(entropy), nil
}

func hashDelegationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
