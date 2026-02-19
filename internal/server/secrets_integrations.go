package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSecretIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.secretIntegrations.List())
	case http.MethodPost:
		var req control.SecretsIntegrationInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.secretIntegrations.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "secrets.integration.upserted",
			Message: "secrets manager integration upserted",
			Fields: map[string]any{
				"integration_id": item.ID,
				"provider":       item.Provider,
				"enabled":        item.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSecretResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SecretResolveInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.secretIntegrations.Resolve(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "secrets.resolved",
		Message: "secret resolved from integration",
		Fields: map[string]any{
			"integration_id": result.IntegrationID,
			"path":           result.Path,
			"version":        result.Version,
			"used_by":        strings.TrimSpace(req.UsedBy),
			"value":          "<redacted>",
		},
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"integration_id": result.IntegrationID,
		"path":           result.Path,
		"version":        result.Version,
		"value":          result.Value,
		"resolved_at":    result.ResolvedAt,
	})
}

func (s *Server) handleSecretUsageTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.secretIntegrations.ListUsageTraces(limit))
}
