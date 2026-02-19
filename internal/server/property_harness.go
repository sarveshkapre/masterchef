package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePropertyHarnessCases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.propertyHarness.ListCases()})
	case http.MethodPost:
		var req control.PropertyHarnessCase
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.propertyHarness.UpsertCase(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePropertyHarnessRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		caseID := strings.TrimSpace(r.URL.Query().Get("case_id"))
		writeJSON(w, http.StatusOK, map[string]any{"items": s.propertyHarness.ListRuns(caseID, limit)})
	case http.MethodPost:
		var req control.PropertyHarnessRunInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.propertyHarness.Run(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if strings.EqualFold(item.Status, "fail") {
			code = http.StatusConflict
		}
		writeJSON(w, code, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePropertyHarnessRunAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/release/tests/property-harness/runs/{id}
	if len(parts) != 6 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "release") || !strings.EqualFold(parts[2], "tests") || !strings.EqualFold(parts[3], "property-harness") || !strings.EqualFold(parts[4], "runs") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid property harness run path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.propertyHarness.GetRun(parts[5])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
