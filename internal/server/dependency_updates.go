package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDependencyUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.dependencyUpdates.Policy())
	case http.MethodPost:
		var req control.DependencyUpdatePolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.dependencyUpdates.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "release.dependency.policy.updated",
			Message: "dependency update policy updated",
			Fields: map[string]any{
				"enabled":                     item.Enabled,
				"max_updates_per_day":         item.MaxUpdatesPerDay,
				"require_compatibility_check": item.RequireCompatibilityCheck,
				"require_performance_check":   item.RequirePerformanceCheck,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDependencyUpdates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.dependencyUpdates.List(limit))
	case http.MethodPost:
		var req control.DependencyUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.dependencyUpdates.Propose(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "release.dependency.update.proposed",
			Message: "dependency update proposal created",
			Fields: map[string]any{
				"update_id":       item.ID,
				"ecosystem":       item.Ecosystem,
				"package":         item.Package,
				"current_version": item.CurrentVersion,
				"target_version":  item.TargetVersion,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDependencyUpdateAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/release/dependency-bot/updates/{id}
	// /v1/release/dependency-bot/updates/{id}/evaluate
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "release" || parts[2] != "dependency-bot" || parts[3] != "updates" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.dependencyUpdates.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "dependency update proposal not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "evaluate" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CompatibilityChecked bool    `json:"compatibility_checked"`
			CompatibilityPassed  bool    `json:"compatibility_passed"`
			PerformanceChecked   bool    `json:"performance_checked"`
			PerformanceDeltaPct  float64 `json:"performance_delta_pct"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.dependencyUpdates.Evaluate(control.DependencyUpdateEvaluationInput{
			UpdateID:             id,
			CompatibilityChecked: req.CompatibilityChecked,
			CompatibilityPassed:  req.CompatibilityPassed,
			PerformanceChecked:   req.PerformanceChecked,
			PerformanceDeltaPct:  req.PerformanceDeltaPct,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "release.dependency.update.evaluated",
			Message: "dependency update proposal evaluated",
			Fields: map[string]any{
				"update_id":       item.ID,
				"ready_for_merge": item.ReadyForMerge,
				"blocked_reasons": item.BlockedReasons,
			},
		}, true)
		status := http.StatusOK
		if !item.ReadyForMerge {
			status = http.StatusConflict
		}
		writeJSON(w, status, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
