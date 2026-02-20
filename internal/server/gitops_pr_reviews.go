package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsPRComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		repository := strings.TrimSpace(r.URL.Query().Get("repository"))
		prNumber := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("pr_number")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				prNumber = n
			}
		}
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": s.gitopsPRReviews.ListComments(repository, prNumber, limit),
		})
	case http.MethodPost:
		var req control.GitOpsPRCommentInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.gitopsPRReviews.AddComment(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.pr.comment.created",
			Message: "gitops plan comment posted",
			Fields: map[string]any{
				"comment_id":              item.ID,
				"repository":              item.Repository,
				"pr_number":               item.PRNumber,
				"risk_level":              item.RiskLevel,
				"environment":             item.Environment,
				"suggested_actions_count": len(item.SuggestedActions),
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGitOpsApprovalGates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		repository := strings.TrimSpace(r.URL.Query().Get("repository"))
		environment := strings.TrimSpace(r.URL.Query().Get("environment"))
		writeJSON(w, http.StatusOK, map[string]any{
			"items": s.gitopsPRReviews.ListGates(repository, environment),
		})
	case http.MethodPost:
		var req control.GitOpsApprovalGate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.gitopsPRReviews.UpsertGate(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.approval_gate.upserted",
			Message: "gitops approval gate upserted",
			Fields: map[string]any{
				"gate_id":            item.ID,
				"repository":         item.Repository,
				"environment":        item.Environment,
				"min_approvals":      item.MinApprovals,
				"required_checks":    item.RequiredChecks,
				"required_reviewers": item.RequiredReviewers,
				"block_risk_levels":  item.BlockRiskLevels,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGitOpsApprovalGateAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/approval-gates/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "approval-gates" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.gitopsPRReviews.GetGate(parts[3])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleGitOpsApprovalGateEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.GitOpsApprovalEvaluationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := s.gitopsPRReviews.Evaluate(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "gitops.approval_gate.evaluated",
		Message: "gitops approval gate evaluated for pull request plan",
		Fields: map[string]any{
			"gate_id":            result.GateID,
			"repository":         result.Repository,
			"environment":        result.Environment,
			"pr_number":          result.PRNumber,
			"approval_count":     result.ApprovalCount,
			"required_approvals": result.RequiredApprovals,
			"allowed":            result.Allowed,
			"reason":             result.Reason,
		},
	}, true)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
