package server

import (
	"net/http"
	"strings"
)

func (s *Server) handleActionDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items := s.actionDocs.List()
	if q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q"))); q != "" {
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			text := strings.ToLower(item.ID + " " + item.Title + " " + item.Summary + " " + strings.Join(item.Tags, " "))
			if strings.Contains(text, q) {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": filtered,
			"count": len(filtered),
			"query": q,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (s *Server) handleActionDocByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action docs path"})
		return
	}
	id := parts[3]
	item, err := s.actionDocs.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
