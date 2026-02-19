package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAgentCheckins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.agentCheckins.List())
	case http.MethodPost:
		var req control.AgentCheckinInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.agentCheckins.Checkin(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "agent.checkin",
			Message: "agent check-in scheduled with splay",
			Fields: map[string]any{
				"agent_id":           item.AgentID,
				"interval_seconds":   item.IntervalSeconds,
				"max_splay_seconds":  item.MaxSplaySeconds,
				"applied_splay_secs": item.AppliedSplaySec,
				"next_checkin_at":    item.NextCheckinAt,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
