package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleReportProcessors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.reportProcessors.List())
	case http.MethodPost:
		var req control.ReportProcessorPluginInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.reportProcessors.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReportProcessorAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/reports/processors/{id}[/enable|disable]
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "reports" || parts[2] != "processors" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.reportProcessors.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "report processor not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var enabled bool
		switch parts[4] {
		case "enable":
			enabled = true
		case "disable":
			enabled = false
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		item, err := s.reportProcessors.SetEnabled(id, enabled)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleReportProcessorDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ReportProcessorDispatchInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result := s.reportProcessors.Dispatch(req)
	if !result.Dispatched {
		writeJSON(w, http.StatusConflict, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
