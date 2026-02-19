package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDashboardWidgets(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		ViewID      string `json:"view_id"`
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
		Width       int    `json:"width,omitempty"`
		Height      int    `json:"height,omitempty"`
		Column      int    `json:"column,omitempty"`
		Row         int    `json:"row,omitempty"`
		Pinned      bool   `json:"pinned,omitempty"`
	}
	switch r.Method {
	case http.MethodGet:
		items := s.dashboardWidgets.List()
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("pinned_only")), "true") {
			filtered := make([]control.DashboardWidget, 0, len(items))
			for _, item := range items {
				if item.Pinned {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"count": len(items),
		})
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		view, err := s.views.Get(req.ViewID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "view not found"})
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = view.Name
		}
		item, err := s.dashboardWidgets.Create(control.DashboardWidget{
			ViewID:      view.ID,
			Title:       title,
			Description: req.Description,
			Width:       req.Width,
			Height:      req.Height,
			Column:      req.Column,
			Row:         req.Row,
			Pinned:      req.Pinned,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDashboardWidgetAction(w http.ResponseWriter, r *http.Request) {
	// /v1/ui/dashboard/widgets/{id}[/pin|refresh]
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid dashboard widget path"})
		return
	}
	id := strings.TrimSpace(parts[4])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "widget id is required"})
		return
	}
	if len(parts) == 5 {
		switch r.Method {
		case http.MethodGet:
			item, err := s.dashboardWidgets.Get(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := s.dashboardWidgets.Delete(id); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[5]
	switch action {
	case "pin":
		var req struct {
			Pinned bool `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.dashboardWidgets.SetPinned(id, req.Pinned)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "refresh":
		item, err := s.dashboardWidgets.Refresh(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown widget action"})
	}
}

func parseDashboardInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
