package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleReadinessScorecards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		environment := strings.TrimSpace(r.URL.Query().Get("environment"))
		service := strings.TrimSpace(r.URL.Query().Get("service"))
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": s.readinessScorecards.List(environment, service, limit),
		})
	case http.MethodPost:
		var req control.ReadinessScorecardInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.readinessScorecards.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if !item.Report.Pass {
			code = http.StatusConflict
		}
		writeJSON(w, code, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReadinessScorecardAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/release/readiness/scorecards/{id}
	if len(parts) != 5 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "release") || !strings.EqualFold(parts[2], "readiness") || !strings.EqualFold(parts[3], "scorecards") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid readiness scorecard path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.readinessScorecards.Get(parts[4])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
