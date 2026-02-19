package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleHostSecurityProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.hostSecurityProfiles.List())
	case http.MethodPost:
		var req control.HostSecurityProfileInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.hostSecurityProfiles.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.host_profiles.upserted",
			Message: "host security profile updated",
			Fields: map[string]any{
				"profile_id":  item.ID,
				"mode":        item.Mode,
				"target_kind": item.TargetKind,
				"target":      item.Target,
				"state":       item.State,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHostSecurityEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.HostSecurityEvaluateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.hostSecurityProfiles.Evaluate(req)
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
