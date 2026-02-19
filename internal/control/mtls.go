package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MTLSAuthority struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CABundle  string    `json:"ca_bundle"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MTLSAuthorityInput struct {
	Name     string `json:"name"`
	CABundle string `json:"ca_bundle"`
}

type MTLSComponentPolicy struct {
	Component          string    `json:"component"`
	MinTLSVersion      string    `json:"min_tls_version"`
	RequireClientCert  bool      `json:"require_client_cert"`
	AllowedAuthorities []string  `json:"allowed_authorities,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type MTLSHandshakeCheckInput struct {
	Component   string `json:"component"`
	AuthorityID string `json:"authority_id"`
	TLSVersion  string `json:"tls_version"`
	ClientCert  bool   `json:"client_cert"`
}

type MTLSHandshakeCheckResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type MTLSStore struct {
	mu          sync.RWMutex
	nextAuthID  int64
	authorities map[string]*MTLSAuthority
	policies    map[string]*MTLSComponentPolicy
}

func NewMTLSStore() *MTLSStore {
	return &MTLSStore{
		authorities: map[string]*MTLSAuthority{},
		policies:    map[string]*MTLSComponentPolicy{},
	}
}

func (s *MTLSStore) CreateAuthority(in MTLSAuthorityInput) (MTLSAuthority, error) {
	name := strings.TrimSpace(in.Name)
	ca := strings.TrimSpace(in.CABundle)
	if name == "" || ca == "" {
		return MTLSAuthority{}, errors.New("name and ca_bundle are required")
	}
	now := time.Now().UTC()
	item := MTLSAuthority{
		Name:      name,
		CABundle:  ca,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextAuthID++
	item.ID = "mtls-ca-" + itoa(s.nextAuthID)
	s.authorities[item.ID] = &item
	return cloneMTLSAuthority(item), nil
}

func (s *MTLSStore) ListAuthorities() []MTLSAuthority {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MTLSAuthority, 0, len(s.authorities))
	for _, item := range s.authorities {
		out = append(out, cloneMTLSAuthority(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MTLSStore) SetPolicy(policy MTLSComponentPolicy) (MTLSComponentPolicy, error) {
	component := strings.TrimSpace(policy.Component)
	if component == "" {
		return MTLSComponentPolicy{}, errors.New("component is required")
	}
	minTLS := strings.TrimSpace(policy.MinTLSVersion)
	if minTLS == "" {
		minTLS = "1.2"
	}
	if minTLS != "1.2" && minTLS != "1.3" {
		return MTLSComponentPolicy{}, errors.New("min_tls_version must be 1.2 or 1.3")
	}
	allowedAuthorities := normalizeStringSlice(policy.AllowedAuthorities)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, authorityID := range allowedAuthorities {
		if _, ok := s.authorities[authorityID]; !ok {
			return MTLSComponentPolicy{}, errors.New("authority not found: " + authorityID)
		}
	}
	item := MTLSComponentPolicy{
		Component:          component,
		MinTLSVersion:      minTLS,
		RequireClientCert:  policy.RequireClientCert,
		AllowedAuthorities: allowedAuthorities,
		UpdatedAt:          time.Now().UTC(),
	}
	s.policies[component] = &item
	return cloneMTLSPolicy(item), nil
}

func (s *MTLSStore) ListPolicies() []MTLSComponentPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MTLSComponentPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, cloneMTLSPolicy(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Component < out[j].Component })
	return out
}

func (s *MTLSStore) CheckHandshake(in MTLSHandshakeCheckInput) MTLSHandshakeCheckResult {
	component := strings.TrimSpace(in.Component)
	authorityID := strings.TrimSpace(in.AuthorityID)
	tlsVersion := strings.TrimSpace(in.TLSVersion)
	if component == "" || authorityID == "" || tlsVersion == "" {
		return MTLSHandshakeCheckResult{Allowed: false, Reason: "component, authority_id, and tls_version are required"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.authorities[authorityID]; !ok {
		return MTLSHandshakeCheckResult{Allowed: false, Reason: "authority not found"}
	}
	policy, ok := s.policies[component]
	if !ok {
		return MTLSHandshakeCheckResult{Allowed: false, Reason: "component policy not found"}
	}
	if !tlsVersionAllowed(policy.MinTLSVersion, tlsVersion) {
		return MTLSHandshakeCheckResult{Allowed: false, Reason: "tls_version below policy minimum"}
	}
	if policy.RequireClientCert && !in.ClientCert {
		return MTLSHandshakeCheckResult{Allowed: false, Reason: "client certificate required by policy"}
	}
	if len(policy.AllowedAuthorities) > 0 {
		matched := false
		for _, id := range policy.AllowedAuthorities {
			if id == authorityID {
				matched = true
				break
			}
		}
		if !matched {
			return MTLSHandshakeCheckResult{Allowed: false, Reason: "authority not allowed for component"}
		}
	}
	return MTLSHandshakeCheckResult{Allowed: true}
}

func tlsVersionAllowed(min, actual string) bool {
	min = strings.TrimPrefix(strings.TrimSpace(min), "TLS")
	actual = strings.TrimPrefix(strings.TrimSpace(actual), "TLS")
	switch min {
	case "1.3":
		return actual == "1.3"
	case "1.2":
		return actual == "1.2" || actual == "1.3"
	default:
		return false
	}
}

func cloneMTLSAuthority(in MTLSAuthority) MTLSAuthority {
	return in
}

func cloneMTLSPolicy(in MTLSComponentPolicy) MTLSComponentPolicy {
	out := in
	out.AllowedAuthorities = append([]string{}, in.AllowedAuthorities...)
	return out
}
