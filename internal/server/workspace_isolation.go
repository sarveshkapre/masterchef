package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleWorkspaceIsolationPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.workspaceIsolation.List())
	case http.MethodPost:
		var req control.WorkspaceIsolationPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.workspaceIsolation.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.workspace_isolation.policy.updated",
			Message: "workspace isolation policy updated",
			Fields: map[string]any{
				"policy_id":                  item.ID,
				"tenant":                     item.Tenant,
				"workspace":                  item.Workspace,
				"environment":                item.Environment,
				"network_segment":            item.NetworkSegment,
				"compute_pool":               item.ComputePool,
				"data_scope":                 item.DataScope,
				"allow_cross_workspace_read": item.AllowCrossWorkspaceRead,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkspaceIsolationEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.WorkspaceIsolationEvaluateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.workspaceIsolation.Evaluate(req)
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
