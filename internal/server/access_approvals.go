package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleApprovalPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.accessApprovals.ListPolicies())
	case http.MethodPost:
		var req control.QuorumApprovalPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.accessApprovals.CreatePolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.approval_policy.created",
			Message: "multi-stage approval policy created",
			Fields: map[string]any{
				"policy_id": item.ID,
				"name":      item.Name,
				"stages":    item.Stages,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleApprovalPolicyAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/access/approval-policies/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "access" || parts[2] != "approval-policies" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.accessApprovals.GetPolicy(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval policy not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleBreakGlassRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.accessApprovals.ListBreakGlassRequests())
	case http.MethodPost:
		var req control.BreakGlassRequestInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.accessApprovals.CreateBreakGlassRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.break_glass.requested",
			Message: "break-glass request created",
			Fields: map[string]any{
				"request_id":   item.ID,
				"requested_by": item.RequestedBy,
				"scope":        item.Scope,
				"policy_id":    item.PolicyID,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleBreakGlassRequestAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/access/break-glass/requests/{id}[/approve|reject|revoke]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "access" || parts[2] != "break-glass" || parts[3] != "requests" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.accessApprovals.GetBreakGlassRequest(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "break-glass request not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if r.Method != http.MethodPost || len(parts) != 6 {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[5]
	var req struct {
		Actor   string `json:"actor"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	var (
		item control.BreakGlassRequest
		err  error
	)
	switch action {
	case "approve":
		item, err = s.accessApprovals.ApproveBreakGlassRequest(id, req.Actor, req.Comment)
	case "reject":
		item, err = s.accessApprovals.RejectBreakGlassRequest(id, req.Actor, req.Comment)
	case "revoke":
		item, err = s.accessApprovals.RevokeBreakGlassRequest(id, req.Actor, req.Comment)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown break-glass action"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "access.break_glass." + action,
		Message: "break-glass request updated",
		Fields: map[string]any{
			"request_id": item.ID,
			"actor":      req.Actor,
			"status":     item.Status,
		},
	}, true)
	writeJSON(w, http.StatusOK, item)
}
