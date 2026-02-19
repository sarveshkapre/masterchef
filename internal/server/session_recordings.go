package server

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleSessionRecordings(w http.ResponseWriter, r *http.Request) {
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
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	transport := strings.TrimSpace(r.URL.Query().Get("transport"))
	writeJSON(w, http.StatusOK, s.sessionRecordings.List(limit, host, transport))
}

func (s *Server) handleSessionRecordingAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	// /v1/execution/session-recordings/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "session-recordings" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session recording path"})
		return
	}
	item, err := s.sessionRecordings.Get(parts[3])
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid session id") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session recording not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
