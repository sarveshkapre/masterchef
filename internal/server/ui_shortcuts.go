package server

import (
	"net/http"
	"strings"
)

func (s *Server) handleUIShortcuts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	items := s.shortcuts.Search(query)
	includeGlobalOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("global_only")), "true")
	if includeGlobalOnly {
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			if item.Global {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":          filtered,
			"count":          len(filtered),
			"query":          query,
			"global_only":    true,
			"active_profile": s.accessibility.ActiveProfile(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":          items,
		"count":          len(items),
		"query":          query,
		"global_only":    false,
		"active_profile": s.accessibility.ActiveProfile(),
	})
}
