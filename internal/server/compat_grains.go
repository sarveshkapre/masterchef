package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleCompatGrains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	node := strings.TrimSpace(r.URL.Query().Get("node"))
	if node != "" {
		item, ok := s.facts.Get(node)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "grain record not found"})
			return
		}
		writeJSON(w, http.StatusOK, control.FactRecordToGrains(item))
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items := s.facts.Query(control.FactCacheQuery{
		Field:    strings.TrimSpace(r.URL.Query().Get("grain")),
		Equals:   strings.TrimSpace(r.URL.Query().Get("equals")),
		Contains: strings.TrimSpace(r.URL.Query().Get("contains")),
		Limit:    limit,
	})
	out := make([]control.GrainRecord, 0, len(items))
	for _, item := range items {
		out = append(out, control.FactRecordToGrains(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(out),
		"items": out,
	})
}

func (s *Server) handleCompatGrainsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.GrainQueryInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	query := control.GrainQueryToFactQuery(req)
	items := s.facts.Query(query)
	out := make([]control.GrainRecord, 0, len(items))
	for _, item := range items {
		out = append(out, control.FactRecordToGrains(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(out),
		"items": out,
		"query": req,
	})
}
