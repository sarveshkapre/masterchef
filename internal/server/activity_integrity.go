package server

import "net/http"

func (s *Server) handleActivityIntegrity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	report := s.events.VerifyIntegrity()
	code := http.StatusOK
	if !report.Valid {
		code = http.StatusConflict
	}
	writeJSON(w, code, report)
}
