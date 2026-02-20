package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type GitOpsPRComment struct {
	ID               string    `json:"id"`
	Repository       string    `json:"repository"`
	PRNumber         int       `json:"pr_number"`
	CommitSHA        string    `json:"commit_sha,omitempty"`
	Environment      string    `json:"environment,omitempty"`
	PlanSummary      string    `json:"plan_summary"`
	RiskLevel        string    `json:"risk_level"`
	SuggestedActions []string  `json:"suggested_actions,omitempty"`
	PostedBy         string    `json:"posted_by"`
	PostedAt         time.Time `json:"posted_at"`
}

type GitOpsPRCommentInput struct {
	Repository       string   `json:"repository"`
	PRNumber         int      `json:"pr_number"`
	CommitSHA        string   `json:"commit_sha,omitempty"`
	Environment      string   `json:"environment,omitempty"`
	PlanSummary      string   `json:"plan_summary"`
	RiskLevel        string   `json:"risk_level,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`
	PostedBy         string   `json:"posted_by,omitempty"`
}

type GitOpsApprovalGate struct {
	ID                string    `json:"id"`
	Repository        string    `json:"repository"`
	Environment       string    `json:"environment"`
	MinApprovals      int       `json:"min_approvals"`
	RequiredChecks    []string  `json:"required_checks,omitempty"`
	RequiredReviewers []string  `json:"required_reviewers,omitempty"`
	BlockRiskLevels   []string  `json:"block_risk_levels,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type GitOpsApprovalEvaluationInput struct {
	GateID        string   `json:"gate_id,omitempty"`
	Repository    string   `json:"repository"`
	Environment   string   `json:"environment,omitempty"`
	PRNumber      int      `json:"pr_number,omitempty"`
	RiskLevel     string   `json:"risk_level,omitempty"`
	ApprovalCount int      `json:"approval_count"`
	PassedChecks  []string `json:"passed_checks,omitempty"`
	Reviewers     []string `json:"reviewers,omitempty"`
}

type GitOpsApprovalEvaluation struct {
	Allowed           bool      `json:"allowed"`
	GateID            string    `json:"gate_id,omitempty"`
	Repository        string    `json:"repository"`
	Environment       string    `json:"environment,omitempty"`
	PRNumber          int       `json:"pr_number,omitempty"`
	RequiredApprovals int       `json:"required_approvals"`
	ApprovalCount     int       `json:"approval_count"`
	MissingChecks     []string  `json:"missing_checks,omitempty"`
	MissingReviewers  []string  `json:"missing_reviewers,omitempty"`
	BlockedByRisk     bool      `json:"blocked_by_risk"`
	Reason            string    `json:"reason"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type GitOpsPRReviewStore struct {
	mu         sync.RWMutex
	nextGateID int64
	nextNoteID int64
	gates      map[string]*GitOpsApprovalGate
	comments   map[string]*GitOpsPRComment
	commentIDs []string
}

func NewGitOpsPRReviewStore() *GitOpsPRReviewStore {
	return &GitOpsPRReviewStore{
		gates:      map[string]*GitOpsApprovalGate{},
		comments:   map[string]*GitOpsPRComment{},
		commentIDs: make([]string, 0, 1024),
	}
}

func (s *GitOpsPRReviewStore) UpsertGate(in GitOpsApprovalGate) (GitOpsApprovalGate, error) {
	repository := normalizeRepository(in.Repository)
	if repository == "" {
		return GitOpsApprovalGate{}, errors.New("repository is required")
	}
	environment := normalizeEnvironment(in.Environment)
	if environment == "" {
		return GitOpsApprovalGate{}, errors.New("environment is required")
	}
	minApprovals := in.MinApprovals
	if minApprovals <= 0 {
		minApprovals = 1
	}
	if minApprovals > 10 {
		return GitOpsApprovalGate{}, errors.New("min_approvals must be between 1 and 10")
	}

	now := time.Now().UTC()
	requiredChecks := normalizeStringList(in.RequiredChecks)
	requiredReviewers := normalizeStringList(in.RequiredReviewers)
	blockRiskLevels := normalizeRiskLevels(in.BlockRiskLevels)

	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(in.ID)
	if id == "" {
		s.nextGateID++
		id = "gitops-gate-" + itoa(s.nextGateID)
	}
	item, ok := s.gates[id]
	if !ok {
		item = &GitOpsApprovalGate{
			ID:        id,
			CreatedAt: now,
		}
		s.gates[id] = item
	}
	item.Repository = repository
	item.Environment = environment
	item.MinApprovals = minApprovals
	item.RequiredChecks = requiredChecks
	item.RequiredReviewers = requiredReviewers
	item.BlockRiskLevels = blockRiskLevels
	item.UpdatedAt = now
	return cloneGitOpsApprovalGate(*item), nil
}

func (s *GitOpsPRReviewStore) ListGates(repository, environment string) []GitOpsApprovalGate {
	repository = normalizeRepository(repository)
	environment = normalizeEnvironment(environment)
	s.mu.RLock()
	out := make([]GitOpsApprovalGate, 0, len(s.gates))
	for _, item := range s.gates {
		if repository != "" && item.Repository != repository {
			continue
		}
		if environment != "" && item.Environment != environment {
			continue
		}
		out = append(out, cloneGitOpsApprovalGate(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *GitOpsPRReviewStore) GetGate(id string) (GitOpsApprovalGate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return GitOpsApprovalGate{}, errors.New("gate id is required")
	}
	s.mu.RLock()
	item, ok := s.gates[id]
	s.mu.RUnlock()
	if !ok {
		return GitOpsApprovalGate{}, errors.New("approval gate not found")
	}
	return cloneGitOpsApprovalGate(*item), nil
}

func (s *GitOpsPRReviewStore) AddComment(in GitOpsPRCommentInput) (GitOpsPRComment, error) {
	repository := normalizeRepository(in.Repository)
	if repository == "" {
		return GitOpsPRComment{}, errors.New("repository is required")
	}
	if in.PRNumber <= 0 {
		return GitOpsPRComment{}, errors.New("pr_number must be greater than zero")
	}
	planSummary := strings.TrimSpace(in.PlanSummary)
	if planSummary == "" {
		return GitOpsPRComment{}, errors.New("plan_summary is required")
	}

	riskLevel := normalizePRRiskLevel(in.RiskLevel)
	if riskLevel == "" {
		riskLevel = "medium"
	}
	postedBy := strings.TrimSpace(in.PostedBy)
	if postedBy == "" {
		postedBy = "masterchef-bot"
	}
	now := time.Now().UTC()
	item := GitOpsPRComment{
		Repository:       repository,
		PRNumber:         in.PRNumber,
		CommitSHA:        strings.TrimSpace(in.CommitSHA),
		Environment:      normalizeEnvironment(in.Environment),
		PlanSummary:      planSummary,
		RiskLevel:        riskLevel,
		SuggestedActions: normalizeStringList(in.SuggestedActions),
		PostedBy:         postedBy,
		PostedAt:         now,
	}

	s.mu.Lock()
	s.nextNoteID++
	item.ID = "gitops-pr-comment-" + itoa(s.nextNoteID)
	s.comments[item.ID] = &item
	s.commentIDs = append(s.commentIDs, item.ID)
	s.mu.Unlock()

	return cloneGitOpsPRComment(item), nil
}

func (s *GitOpsPRReviewStore) ListComments(repository string, prNumber, limit int) []GitOpsPRComment {
	repository = normalizeRepository(repository)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]GitOpsPRComment, 0, len(s.commentIDs))
	for i := len(s.commentIDs) - 1; i >= 0; i-- {
		item := s.comments[s.commentIDs[i]]
		if item == nil {
			continue
		}
		if repository != "" && item.Repository != repository {
			continue
		}
		if prNumber > 0 && item.PRNumber != prNumber {
			continue
		}
		out = append(out, cloneGitOpsPRComment(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *GitOpsPRReviewStore) Evaluate(in GitOpsApprovalEvaluationInput) (GitOpsApprovalEvaluation, error) {
	repository := normalizeRepository(in.Repository)
	if repository == "" {
		return GitOpsApprovalEvaluation{}, errors.New("repository is required")
	}
	environment := normalizeEnvironment(in.Environment)
	if environment == "" {
		environment = "prod"
	}

	gate, hasGate := s.resolveGate(strings.TrimSpace(in.GateID), repository, environment)
	if !hasGate {
		return GitOpsApprovalEvaluation{
			Allowed:           true,
			Repository:        repository,
			Environment:       environment,
			PRNumber:          in.PRNumber,
			RequiredApprovals: 0,
			ApprovalCount:     in.ApprovalCount,
			BlockedByRisk:     false,
			Reason:            "no approval gate configured for repository/environment",
			EvaluatedAt:       time.Now().UTC(),
		}, nil
	}

	passedChecks := normalizeStringList(in.PassedChecks)
	reviewers := normalizeStringList(in.Reviewers)
	missingChecks := missingStrings(gate.RequiredChecks, passedChecks)
	missingReviewers := missingStrings(gate.RequiredReviewers, reviewers)
	riskLevel := normalizePRRiskLevel(in.RiskLevel)
	if riskLevel == "" {
		riskLevel = "medium"
	}
	blockedByRisk := containsString(gate.BlockRiskLevels, riskLevel)
	requiredApprovals := gate.MinApprovals
	if blockedByRisk {
		requiredApprovals++
	}

	allowed := true
	reason := "approval gate passed"
	if in.ApprovalCount < requiredApprovals {
		allowed = false
		reason = "insufficient approvals for gate policy"
	}
	if allowed && len(missingChecks) > 0 {
		allowed = false
		reason = "required checks are missing"
	}
	if allowed && len(missingReviewers) > 0 {
		allowed = false
		reason = "required reviewers are missing"
	}

	return GitOpsApprovalEvaluation{
		Allowed:           allowed,
		GateID:            gate.ID,
		Repository:        repository,
		Environment:       environment,
		PRNumber:          in.PRNumber,
		RequiredApprovals: requiredApprovals,
		ApprovalCount:     in.ApprovalCount,
		MissingChecks:     missingChecks,
		MissingReviewers:  missingReviewers,
		BlockedByRisk:     blockedByRisk,
		Reason:            reason,
		EvaluatedAt:       time.Now().UTC(),
	}, nil
}

func (s *GitOpsPRReviewStore) resolveGate(id, repository, environment string) (GitOpsApprovalGate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != "" {
		item, ok := s.gates[id]
		if !ok {
			return GitOpsApprovalGate{}, false
		}
		return cloneGitOpsApprovalGate(*item), true
	}
	for _, item := range s.gates {
		if item.Repository == repository && item.Environment == environment {
			return cloneGitOpsApprovalGate(*item), true
		}
	}
	return GitOpsApprovalGate{}, false
}

func cloneGitOpsPRComment(in GitOpsPRComment) GitOpsPRComment {
	out := in
	out.SuggestedActions = cloneStringSlice(in.SuggestedActions)
	return out
}

func cloneGitOpsApprovalGate(in GitOpsApprovalGate) GitOpsApprovalGate {
	out := in
	out.RequiredChecks = cloneStringSlice(in.RequiredChecks)
	out.RequiredReviewers = cloneStringSlice(in.RequiredReviewers)
	out.BlockRiskLevels = cloneStringSlice(in.BlockRiskLevels)
	return out
}

func normalizeRepository(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeEnvironment(raw string) string {
	env := strings.ToLower(strings.TrimSpace(raw))
	if env == "" {
		return ""
	}
	return env
}

func normalizeRiskLevels(levels []string) []string {
	if len(levels) == 0 {
		return []string{"high", "critical"}
	}
	out := make([]string, 0, len(levels))
	for _, item := range normalizeStringList(levels) {
		risk := normalizePRRiskLevel(item)
		if risk == "" {
			continue
		}
		out = append(out, risk)
	}
	if len(out) == 0 {
		return []string{"high", "critical"}
	}
	return normalizeStringList(out)
}

func normalizePRRiskLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func missingStrings(required, actual []string) []string {
	if len(required) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range actual {
		set[item] = struct{}{}
	}
	missing := make([]string, 0)
	for _, item := range required {
		if _, ok := set[item]; !ok {
			missing = append(missing, item)
		}
	}
	return missing
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}
