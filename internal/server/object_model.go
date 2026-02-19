package server

import (
	"net/http"
	"strings"
)

func (s *Server) handleObjectModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items := s.objectModel.List()
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (s *Server) handleObjectModelResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "term is required"})
		return
	}
	entry, err := s.objectModel.Resolve(term)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"term":  term,
		"match": entry,
	})
}
