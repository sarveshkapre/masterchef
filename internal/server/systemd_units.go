package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSystemdUnits(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.systemdUnits.List())
	case http.MethodPost:
		var req control.SystemdUnitInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.systemdUnits.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSystemdUnitAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	// /v1/execution/systemd/units/{name}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "systemd" || parts[3] != "units" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid systemd unit path"})
		return
	}
	name := strings.TrimSpace(parts[4])
	item, ok := s.systemdUnits.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "systemd unit not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleSystemdRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SystemdRenderInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	out, err := s.systemdUnits.Render(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}
