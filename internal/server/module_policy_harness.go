package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleModulePolicyHarnessCases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.modulePolicyHarness.ListCases())
	case http.MethodPost:
		var req control.ModulePolicyHarnessCaseInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.modulePolicyHarness.UpsertCase(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModulePolicyHarnessRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.modulePolicyHarness.ListRuns(limit))
	case http.MethodPost:
		var req control.ModulePolicyHarnessRunInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		run, err := s.modulePolicyHarness.Run(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if run.Status == "failed" {
			writeJSON(w, http.StatusConflict, run)
			return
		}
		writeJSON(w, http.StatusOK, run)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModulePolicyHarnessRunAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) != 6 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "release") || !strings.EqualFold(parts[2], "tests") || !strings.EqualFold(parts[3], "harness") || !strings.EqualFold(parts[4], "runs") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid harness run path"})
		return
	}
	run, ok := s.modulePolicyHarness.GetRun(parts[5])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "harness run not found"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}
