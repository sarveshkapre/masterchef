package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

type auditTimelineItem struct {
	Time     time.Time      `json:"time"`
	Category string         `json:"category"` // identity|resource|other
	Type     string         `json:"type"`
	Message  string         `json:"message,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

func (s *Server) handleAuditTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 2000 {
		limit = 2000
	}
	sinceWindow := 24 * time.Hour
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "since must be a positive duration (for example 6h, 30m)"})
			return
		}
		sinceWindow = d
	}
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	if category == "" {
		category = "all"
	}
	switch category {
	case "all", "identity", "resource":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category must be all, identity, or resource"})
		return
	}

	events := s.events.Query(control.EventQuery{
		Since: time.Now().UTC().Add(-sinceWindow),
		Desc:  true,
		Limit: limit * 3,
	})
	items := make([]auditTimelineItem, 0, minInt(limit, len(events)))
	for _, evt := range events {
		cat := auditCategory(evt.Type)
		if category != "all" && cat != category {
			continue
		}
		items = append(items, auditTimelineItem{
			Time:     evt.Time,
			Category: cat,
			Type:     evt.Type,
			Message:  evt.Message,
			Fields:   evt.Fields,
		})
		if len(items) >= limit {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Time.After(items[j].Time)
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"category": category,
		"since":    sinceWindow.String(),
		"count":    len(items),
		"items":    items,
	})
}

func auditCategory(eventType string) string {
	typ := strings.ToLower(strings.TrimSpace(eventType))
	switch {
	case strings.HasPrefix(typ, "identity."),
		strings.HasPrefix(typ, "access."),
		strings.HasPrefix(typ, "security."),
		strings.HasPrefix(typ, "compliance."):
		return "identity"
	case strings.HasPrefix(typ, "run."),
		strings.HasPrefix(typ, "drift."),
		strings.HasPrefix(typ, "policy."),
		strings.HasPrefix(typ, "package."),
		strings.HasPrefix(typ, "job."),
		strings.HasPrefix(typ, "execution."),
		strings.HasPrefix(typ, "deploy."),
		strings.HasPrefix(typ, "resource."),
		strings.HasPrefix(typ, "control."):
		return "resource"
	default:
		return "other"
	}
}
