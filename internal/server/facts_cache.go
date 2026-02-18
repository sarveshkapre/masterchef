package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleFactCache(w http.ResponseWriter, r *http.Request) {
	type upsertReq struct {
		Node       string         `json:"node"`
		Facts      map[string]any `json:"facts"`
		TTLSeconds int            `json:"ttl_seconds"`
	}
	switch r.Method {
	case http.MethodGet:
		field := r.URL.Query().Get("field")
		equals := r.URL.Query().Get("equals")
		contains := r.URL.Query().Get("contains")
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		items := s.facts.Query(control.FactCacheQuery{
			Field:    field,
			Equals:   equals,
			Contains: contains,
			Limit:    limit,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(items),
			"items": items,
		})
	case http.MethodPost:
		var req upsertReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		ttl := time.Duration(req.TTLSeconds) * time.Second
		item := s.facts.Upsert(req.Node, req.Facts, ttl)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFactCacheNode(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid fact cache node path"})
		return
	}
	node := parts[3]
	switch r.Method {
	case http.MethodGet:
		item, ok := s.facts.Get(node)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "fact record not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if !s.facts.Delete(node) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "fact record not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFactMineQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.FactCacheQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	items := s.facts.Query(req)
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(items),
		"items": items,
		"query": req,
	})
}
