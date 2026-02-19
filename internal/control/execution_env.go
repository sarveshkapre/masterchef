package control

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExecutionEnvironment struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ImageDigest  string    `json:"image_digest"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Signed       bool      `json:"signed"`
	SignatureRef string    `json:"signature_ref,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExecutionEnvironmentInput struct {
	Name         string   `json:"name"`
	ImageDigest  string   `json:"image_digest"`
	Dependencies []string `json:"dependencies,omitempty"`
	Signed       bool     `json:"signed"`
	SignatureRef string   `json:"signature_ref,omitempty"`
}

type ExecutionAdmissionPolicy struct {
	RequireSigned  bool      `json:"require_signed"`
	AllowedDigests []string  `json:"allowed_digests,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ExecutionAdmissionResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type ExecutionEnvironmentStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]*ExecutionEnvironment
	policy ExecutionAdmissionPolicy
}

func NewExecutionEnvironmentStore() *ExecutionEnvironmentStore {
	return &ExecutionEnvironmentStore{
		items:  map[string]*ExecutionEnvironment{},
		policy: ExecutionAdmissionPolicy{RequireSigned: true, UpdatedAt: time.Now().UTC()},
	}
}

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func (s *ExecutionEnvironmentStore) Create(in ExecutionEnvironmentInput) (ExecutionEnvironment, error) {
	name := strings.TrimSpace(in.Name)
	digest := strings.ToLower(strings.TrimSpace(in.ImageDigest))
	if name == "" || digest == "" {
		return ExecutionEnvironment{}, errors.New("name and image_digest are required")
	}
	if !digestPattern.MatchString(digest) {
		return ExecutionEnvironment{}, errors.New("image_digest must be immutable sha256:<64-hex>")
	}
	if in.Signed && strings.TrimSpace(in.SignatureRef) == "" {
		return ExecutionEnvironment{}, errors.New("signature_ref is required when signed=true")
	}
	if !in.Signed {
		in.SignatureRef = ""
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &ExecutionEnvironment{
		ID:           "execenv-" + itoa(s.nextID),
		Name:         name,
		ImageDigest:  digest,
		Dependencies: normalizeStringSlice(in.Dependencies),
		Signed:       in.Signed,
		SignatureRef: strings.TrimSpace(in.SignatureRef),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.items[item.ID] = item
	return cloneExecutionEnvironment(*item), nil
}

func (s *ExecutionEnvironmentStore) Get(id string) (ExecutionEnvironment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		return ExecutionEnvironment{}, false
	}
	return cloneExecutionEnvironment(*item), true
}

func (s *ExecutionEnvironmentStore) List() []ExecutionEnvironment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ExecutionEnvironment, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneExecutionEnvironment(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ExecutionEnvironmentStore) Policy() ExecutionAdmissionPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneExecutionAdmissionPolicy(s.policy)
}

func (s *ExecutionEnvironmentStore) SetPolicy(policy ExecutionAdmissionPolicy) ExecutionAdmissionPolicy {
	policy.AllowedDigests = normalizeStringSlice(policy.AllowedDigests)
	policy.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return cloneExecutionAdmissionPolicy(policy)
}

func (s *ExecutionEnvironmentStore) EvaluateAdmission(env ExecutionEnvironment) ExecutionAdmissionResult {
	p := s.Policy()
	if p.RequireSigned && !env.Signed {
		return ExecutionAdmissionResult{Allowed: false, Reason: "signed execution environment required"}
	}
	if len(p.AllowedDigests) > 0 {
		allowed := false
		for _, digest := range p.AllowedDigests {
			if digest == env.ImageDigest {
				allowed = true
				break
			}
		}
		if !allowed {
			return ExecutionAdmissionResult{Allowed: false, Reason: "image digest not allowed by policy"}
		}
	}
	return ExecutionAdmissionResult{Allowed: true}
}

func cloneExecutionEnvironment(in ExecutionEnvironment) ExecutionEnvironment {
	out := in
	out.Dependencies = append([]string{}, in.Dependencies...)
	return out
}

func cloneExecutionAdmissionPolicy(in ExecutionAdmissionPolicy) ExecutionAdmissionPolicy {
	out := in
	out.AllowedDigests = append([]string{}, in.AllowedDigests...)
	return out
}
