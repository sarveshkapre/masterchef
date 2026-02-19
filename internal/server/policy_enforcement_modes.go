package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePolicyEnforcementModes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.policyModes.List())
	case http.MethodPost:
		var req control.PolicyEnforcementModeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.policyModes.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "policy.enforcement.mode.updated",
			Message: "policy enforcement mode updated",
			Fields: map[string]any{
				"policy_ref": item.PolicyRef,
				"mode":       item.Mode,
				"updated_by": item.UpdatedBy,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePolicyEnforcementModeAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/policy/enforcement-modes/{policy_ref}|evaluate
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "policy" || parts[2] != "enforcement-modes" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		policyRef := strings.TrimSpace(parts[3])
		item, ok := s.policyModes.Get(policyRef)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy enforcement mode not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 4 && parts[3] == "evaluate" && r.Method == http.MethodPost:
		var req struct {
			PolicyRef            string `json:"policy_ref"`
			DriftDetected        bool   `json:"drift_detected"`
			HighRisk             bool   `json:"high_risk,omitempty"`
			CanAutocorrect       bool   `json:"can_autocorrect,omitempty"`
			SimConfidencePercent int    `json:"simulation_confidence_percent,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		policyRef := strings.TrimSpace(req.PolicyRef)
		if policyRef == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy_ref is required"})
			return
		}
		mode := control.PolicyEnforcementAudit
		item, ok := s.policyModes.Get(policyRef)
		if ok {
			mode = item.Mode
		}
		action := "audit-only"
		decision := "allow"
		reason := "audit mode does not apply changes"
		switch mode {
		case control.PolicyEnforcementApplyAndMonitor:
			action = "apply-and-monitor"
			reason = "apply mode executes changes and expects post-run monitoring"
		case control.PolicyEnforcementApplyAndAutocorrect:
			action = "apply-and-autocorrect"
			reason = "auto-correct mode can remediate approved drift"
			if !req.DriftDetected {
				action = "monitor-only"
				reason = "no drift detected"
			} else if !req.CanAutocorrect {
				action = "monitor-only"
				reason = "drift cannot be auto-corrected by policy"
			}
			if req.HighRisk || (req.SimConfidencePercent > 0 && req.SimConfidencePercent < 80) {
				action = "monitor-only"
				decision = "blocked"
				reason = "high-risk drift requires manual approval"
			}
		default:
			action = "audit-only"
			reason = "policy mode is audit"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"policy_ref": policyRef,
			"mode":       mode,
			"decision":   decision,
			"action":     action,
			"reason":     reason,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
