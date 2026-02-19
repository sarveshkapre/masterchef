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

type JITAccessGrant struct {
	ID                  string     `json:"id"`
	Subject             string     `json:"subject"`
	Resource            string     `json:"resource"`
	Action              string     `json:"action"`
	IssuedBy            string     `json:"issued_by"`
	Reason              string     `json:"reason"`
	BreakGlassRequestID string     `json:"break_glass_request_id,omitempty"`
	TTLSeconds          int        `json:"ttl_seconds"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
}

type JITAccessGrantIssueInput struct {
	Subject             string `json:"subject"`
	Resource            string `json:"resource"`
	Action              string `json:"action"`
	IssuedBy            string `json:"issued_by"`
	Reason              string `json:"reason"`
	BreakGlassRequestID string `json:"break_glass_request_id,omitempty"`
	TTLSeconds          int    `json:"ttl_seconds,omitempty"`
}

type IssuedJITAccessGrant struct {
	Grant JITAccessGrant `json:"grant"`
	Token string         `json:"token"`
}

type JITAccessGrantValidationInput struct {
	Token    string `json:"token"`
	Resource string `json:"resource,omitempty"`
	Action   string `json:"action,omitempty"`
}

type JITAccessGrantValidationResult struct {
	Allowed   bool      `json:"allowed"`
	Reason    string    `json:"reason,omitempty"`
	GrantID   string    `json:"grant_id,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	Resource  string    `json:"resource,omitempty"`
	Action    string    `json:"action,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type jitGrantRecord struct {
	grant     JITAccessGrant
	tokenHash string
}

type JITAccessGrantStore struct {
	mu         sync.RWMutex
	nextID     int64
	grants     map[string]*jitGrantRecord
	tokenIndex map[string]string
}

func NewJITAccessGrantStore() *JITAccessGrantStore {
	return &JITAccessGrantStore{
		grants:     map[string]*jitGrantRecord{},
		tokenIndex: map[string]string{},
	}
}

func (s *JITAccessGrantStore) Issue(in JITAccessGrantIssueInput) (IssuedJITAccessGrant, error) {
	subject := strings.TrimSpace(in.Subject)
	resource := strings.TrimSpace(in.Resource)
	action := strings.TrimSpace(in.Action)
	issuedBy := strings.TrimSpace(in.IssuedBy)
	reason := strings.TrimSpace(in.Reason)
	if subject == "" || resource == "" || action == "" || issuedBy == "" || reason == "" {
		return IssuedJITAccessGrant{}, errors.New("subject, resource, action, issued_by, and reason are required")
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 900
	}
	if ttl < 60 {
		return IssuedJITAccessGrant{}, errors.New("ttl_seconds must be >= 60")
	}
	if ttl > 3600 {
		return IssuedJITAccessGrant{}, errors.New("ttl_seconds must be <= 3600")
	}
	token, err := generateJITAccessGrantToken()
	if err != nil {
		return IssuedJITAccessGrant{}, err
	}
	now := time.Now().UTC()
	grant := JITAccessGrant{
		Subject:             subject,
		Resource:            resource,
		Action:              action,
		IssuedBy:            issuedBy,
		Reason:              reason,
		BreakGlassRequestID: strings.TrimSpace(in.BreakGlassRequestID),
		TTLSeconds:          ttl,
		CreatedAt:           now,
		ExpiresAt:           now.Add(time.Duration(ttl) * time.Second),
	}

	tokenHash := hashJITAccessGrantToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	grant.ID = "jit-grant-" + itoa(s.nextID)
	s.grants[grant.ID] = &jitGrantRecord{grant: grant, tokenHash: tokenHash}
	s.tokenIndex[tokenHash] = grant.ID
	return IssuedJITAccessGrant{
		Grant: cloneJITAccessGrant(grant),
		Token: token,
	}, nil
}

func (s *JITAccessGrantStore) List() []JITAccessGrant {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireLocked(now)
	out := make([]JITAccessGrant, 0, len(s.grants))
	for _, item := range s.grants {
		out = append(out, cloneJITAccessGrant(item.grant))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *JITAccessGrantStore) Get(id string) (JITAccessGrant, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	item, ok := s.grants[strings.TrimSpace(id)]
	if !ok {
		return JITAccessGrant{}, false
	}
	return cloneJITAccessGrant(item.grant), true
}

func (s *JITAccessGrantStore) Revoke(id string) (JITAccessGrant, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return JITAccessGrant{}, errors.New("grant id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.grants[id]
	if !ok {
		return JITAccessGrant{}, errors.New("jit access grant not found")
	}
	if item.grant.RevokedAt == nil {
		now := time.Now().UTC()
		item.grant.RevokedAt = &now
	}
	return cloneJITAccessGrant(item.grant), nil
}

func (s *JITAccessGrantStore) Validate(in JITAccessGrantValidationInput) JITAccessGrantValidationResult {
	return s.validateAt(in, time.Now().UTC())
}

func (s *JITAccessGrantStore) validateAt(in JITAccessGrantValidationInput, now time.Time) JITAccessGrantValidationResult {
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return JITAccessGrantValidationResult{Allowed: false, Reason: "token is required"}
	}
	tokenHash := hashJITAccessGrantToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	grantID, ok := s.tokenIndex[tokenHash]
	if !ok {
		return JITAccessGrantValidationResult{Allowed: false, Reason: "jit access grant token not recognized"}
	}
	record, ok := s.grants[grantID]
	if !ok {
		return JITAccessGrantValidationResult{Allowed: false, Reason: "jit access grant token not recognized"}
	}
	grant := record.grant
	if grant.RevokedAt != nil {
		return resultFromJITGrant(grant, false, "jit access grant revoked")
	}
	if !now.Before(grant.ExpiresAt) {
		return resultFromJITGrant(grant, false, "jit access grant expired")
	}
	if reqResource := strings.TrimSpace(in.Resource); reqResource != "" && reqResource != grant.Resource {
		return resultFromJITGrant(grant, false, "jit access grant resource mismatch")
	}
	if reqAction := strings.TrimSpace(in.Action); reqAction != "" && reqAction != grant.Action {
		return resultFromJITGrant(grant, false, "jit access grant action mismatch")
	}
	return resultFromJITGrant(grant, true, "")
}

func (s *JITAccessGrantStore) expireLocked(now time.Time) {
	for _, item := range s.grants {
		if item.grant.RevokedAt != nil {
			continue
		}
		if !now.Before(item.grant.ExpiresAt) {
			expiredAt := item.grant.ExpiresAt
			item.grant.RevokedAt = &expiredAt
		}
	}
}

func resultFromJITGrant(grant JITAccessGrant, allowed bool, reason string) JITAccessGrantValidationResult {
	return JITAccessGrantValidationResult{
		Allowed:   allowed,
		Reason:    reason,
		GrantID:   grant.ID,
		Subject:   grant.Subject,
		Resource:  grant.Resource,
		Action:    grant.Action,
		ExpiresAt: grant.ExpiresAt,
	}
}

func cloneJITAccessGrant(in JITAccessGrant) JITAccessGrant {
	out := in
	if in.RevokedAt != nil {
		revokedAt := *in.RevokedAt
		out.RevokedAt = &revokedAt
	}
	return out
}

func generateJITAccessGrantToken() (string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return "mcjit_" + hex.EncodeToString(entropy), nil
}

func hashJITAccessGrantToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
