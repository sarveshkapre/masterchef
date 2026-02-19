package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleTestScenarios(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.scenarioTests.ListScenarios()})
	case http.MethodPost:
		var req control.ScenarioDefinition
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.scenarioTests.UpsertScenario(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTestScenarioRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.scenarioTests.ListRuns()})
	case http.MethodPost:
		var req control.ScenarioRunInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.scenarioTests.Run(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTestScenarioRunAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/release/tests/scenario-runs/{id}
	if len(parts) != 5 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "release") || !strings.EqualFold(parts[2], "tests") || !strings.EqualFold(parts[3], "scenario-runs") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scenario run path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.scenarioTests.GetRun(parts[4])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
