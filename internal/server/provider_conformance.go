package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleProviderConformanceSuites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.providerConformance.ListSuites()})
	case http.MethodPost:
		var req control.ProviderConformanceSuite
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.providerConformance.UpsertSuite(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderConformanceRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		suiteID := strings.TrimSpace(r.URL.Query().Get("suite_id"))
		writeJSON(w, http.StatusOK, map[string]any{"items": s.providerConformance.ListRuns(suiteID, limit)})
	case http.MethodPost:
		var req control.ProviderConformanceRunInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.providerConformance.Run(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderConformanceRunAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/providers/conformance/runs/{id}
	if len(parts) != 5 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "providers") || !strings.EqualFold(parts[2], "conformance") || !strings.EqualFold(parts[3], "runs") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider conformance run path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.providerConformance.GetRun(parts[4])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
