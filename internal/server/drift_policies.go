package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func parseBoolQuery(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (s *Server) handleDriftSuppressions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		includeExpired := parseBoolQuery(r.URL.Query().Get("include_expired"))
		writeJSON(w, http.StatusOK, s.driftPolicies.ListSuppressions(includeExpired))
	case http.MethodPost:
		var req control.DriftSuppressionInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.driftPolicies.AddSuppression(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "drift.suppression.created",
			Message: "drift suppression created",
			Fields: map[string]any{
				"drift_suppression_id": item.ID,
				"scope_type":           item.ScopeType,
				"scope_value":          item.ScopeValue,
				"until":                item.Until.Format(timeRFC3339),
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDriftSuppressionByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/drift/suppressions/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "drift" || parts[2] != "suppressions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(parts[3])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "suppression id is required"})
		return
	}
	if !s.driftPolicies.DeleteSuppression(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift suppression not found"})
		return
	}
	s.recordEvent(control.Event{
		Type:    "drift.suppression.deleted",
		Message: "drift suppression deleted",
		Fields: map[string]any{
			"drift_suppression_id": id,
		},
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": id})
}

func (s *Server) handleDriftAllowlists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		includeExpired := parseBoolQuery(r.URL.Query().Get("include_expired"))
		writeJSON(w, http.StatusOK, s.driftPolicies.ListAllowlist(includeExpired))
	case http.MethodPost:
		var req control.DriftAllowlistInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.driftPolicies.AddAllowlist(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "drift.allowlist.created",
			Message: "drift allowlist created",
			Fields: map[string]any{
				"drift_allowlist_id": item.ID,
				"scope_type":         item.ScopeType,
				"scope_value":        item.ScopeValue,
				"expires_at":         item.ExpiresAt.Format(timeRFC3339),
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDriftAllowlistByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/drift/allowlists/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "drift" || parts[2] != "allowlists" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(parts[3])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "allowlist id is required"})
		return
	}
	if !s.driftPolicies.DeleteAllowlist(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift allowlist not found"})
		return
	}
	s.recordEvent(control.Event{
		Type:    "drift.allowlist.deleted",
		Message: "drift allowlist deleted",
		Fields: map[string]any{
			"drift_allowlist_id": id,
		},
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": id})
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func parsePositiveInt(v string, fallback int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
