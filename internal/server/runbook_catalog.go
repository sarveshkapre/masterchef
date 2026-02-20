package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRunbookCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	query := control.RunbookCatalogQuery{
		Owner:        strings.TrimSpace(r.URL.Query().Get("owner")),
		Tag:          strings.TrimSpace(r.URL.Query().Get("tag")),
		MaxRiskLevel: strings.TrimSpace(r.URL.Query().Get("max_risk_level")),
		Limit:        limit,
	}
	items := s.runbooks.Catalog(query)
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"query": query,
	})
}
