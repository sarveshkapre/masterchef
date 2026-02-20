package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleCompatibilityShims(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.compatibilityShims.List())
	case http.MethodPost:
		var req control.CompatibilityShimInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.compatibilityShims.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCompatibilityShimAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/compat/shims/{id} or /v1/compat/shims/{id}/{action}
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "compat" || parts[2] != "shims" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.compatibilityShims.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "compatibility shim not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) != 5 || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var (
		item control.CompatibilityShim
		err  error
	)
	switch parts[4] {
	case "enable":
		item, err = s.compatibilityShims.Enable(id)
	case "disable":
		item, err = s.compatibilityShims.Disable(id)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleCompatibilityShimsResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.CompatibilityShimResolveInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	writeJSON(w, http.StatusOK, s.compatibilityShims.Resolve(req))
}
