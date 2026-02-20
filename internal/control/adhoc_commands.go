package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AdHocGuardrailPolicy struct {
	BlockedPatterns   []string  `json:"blocked_patterns"`
	RequireReason     bool      `json:"require_reason"`
	MaxTimeoutSeconds int       `json:"max_timeout_seconds"`
	AllowExecution    bool      `json:"allow_execution"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AdHocCommandRequest struct {
	Command        string `json:"command"`
	Reason         string `json:"reason,omitempty"`
	RequestedBy    string `json:"requested_by,omitempty"`
	Host           string `json:"host,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

type AdHocCommandResult struct {
	ID             string    `json:"id"`
	Command        string    `json:"command"`
	Reason         string    `json:"reason,omitempty"`
	RequestedBy    string    `json:"requested_by,omitempty"`
	Host           string    `json:"host,omitempty"`
	DryRun         bool      `json:"dry_run"`
	Allowed        bool      `json:"allowed"`
	Status         string    `json:"status"` // approved|blocked|succeeded|failed
	ExitCode       int       `json:"exit_code,omitempty"`
	Output         string    `json:"output,omitempty"`
	BlockedReasons []string  `json:"blocked_reasons,omitempty"`
	DurationMillis int64     `json:"duration_millis,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AdHocCommandStore struct {
	mu      sync.RWMutex
	nextID  int64
	limit   int
	policy  AdHocGuardrailPolicy
	history []AdHocCommandResult
}

func NewAdHocCommandStore(limit int) *AdHocCommandStore {
	if limit <= 0 {
		limit = 2000
	}
	return &AdHocCommandStore{
		limit: limit,
		policy: AdHocGuardrailPolicy{
			BlockedPatterns: []string{
				"rm -rf /",
				"shutdown",
				"reboot",
				"mkfs",
				"dd if=",
				":(){:|:&};:",
			},
			RequireReason:     true,
			MaxTimeoutSeconds: 120,
			AllowExecution:    true,
			UpdatedAt:         time.Now().UTC(),
		},
		history: make([]AdHocCommandResult, 0, limit),
	}
}

func (s *AdHocCommandStore) Policy() AdHocGuardrailPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAdHocPolicy(s.policy)
}

func (s *AdHocCommandStore) SetPolicy(in AdHocGuardrailPolicy) (AdHocGuardrailPolicy, error) {
	policy := AdHocGuardrailPolicy{
		BlockedPatterns:   normalizeStringList(in.BlockedPatterns),
		RequireReason:     in.RequireReason,
		MaxTimeoutSeconds: in.MaxTimeoutSeconds,
		AllowExecution:    in.AllowExecution,
		UpdatedAt:         time.Now().UTC(),
	}
	if policy.MaxTimeoutSeconds <= 0 {
		policy.MaxTimeoutSeconds = 120
	}
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return cloneAdHocPolicy(policy), nil
}

func (s *AdHocCommandStore) Evaluate(req AdHocCommandRequest) (AdHocCommandRequest, bool, []string, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return AdHocCommandRequest{}, false, nil, errors.New("command is required")
	}
	requestedBy := strings.TrimSpace(req.RequestedBy)
	if requestedBy == "" {
		requestedBy = "unknown"
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "localhost"
	}
	timeout := req.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}

	policy := s.Policy()
	if timeout > policy.MaxTimeoutSeconds {
		timeout = policy.MaxTimeoutSeconds
	}
	normalized := AdHocCommandRequest{
		Command:        command,
		Reason:         strings.TrimSpace(req.Reason),
		RequestedBy:    requestedBy,
		Host:           host,
		TimeoutSeconds: timeout,
		DryRun:         req.DryRun,
	}
	blockedReasons := make([]string, 0, 4)
	if policy.RequireReason && normalized.Reason == "" {
		blockedReasons = append(blockedReasons, "reason is required by guardrail policy")
	}
	lower := strings.ToLower(command)
	for _, pattern := range policy.BlockedPatterns {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		if strings.Contains(lower, pat) {
			blockedReasons = append(blockedReasons, "command matches blocked pattern: "+pattern)
		}
	}
	if !normalized.DryRun && !policy.AllowExecution {
		blockedReasons = append(blockedReasons, "command execution is disabled by guardrail policy")
	}
	allowed := len(blockedReasons) == 0
	return normalized, allowed, blockedReasons, nil
}

func (s *AdHocCommandStore) Record(result AdHocCommandResult) AdHocCommandResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	result.ID = "adhoc-" + itoa(s.nextID)
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	result.BlockedReasons = append([]string{}, result.BlockedReasons...)
	if len(s.history) >= s.limit {
		copy(s.history[0:], s.history[1:])
		s.history[len(s.history)-1] = result
	} else {
		s.history = append(s.history, result)
	}
	return result
}

func (s *AdHocCommandStore) List(limit int) []AdHocCommandResult {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]AdHocCommandResult, len(s.history))
	copy(out, s.history)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		out[i].BlockedReasons = append([]string{}, out[i].BlockedReasons...)
	}
	return out
}

func cloneAdHocPolicy(in AdHocGuardrailPolicy) AdHocGuardrailPolicy {
	out := in
	out.BlockedPatterns = append([]string{}, in.BlockedPatterns...)
	return out
}
