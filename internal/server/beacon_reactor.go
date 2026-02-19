package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleBeaconReactorRules(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name            string                  `json:"name"`
		BeaconPrefix    string                  `json:"beacon_prefix"`
		MatchMode       string                  `json:"match_mode"`
		Conditions      []control.RuleCondition `json:"conditions,omitempty"`
		Actions         []control.RuleAction    `json:"actions"`
		CooldownSeconds int                     `json:"cooldown_seconds,omitempty"`
		Enabled         *bool                   `json:"enabled,omitempty"`
	}

	switch r.Method {
	case http.MethodGet:
		all := s.rules.List()
		out := make([]control.Rule, 0, len(all))
		for _, rule := range all {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rule.SourcePrefix)), "beacon.") {
				out = append(out, rule)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sourcePrefix := normalizeBeaconPrefix(req.BeaconPrefix)
		rule, err := s.rules.Create(control.Rule{
			Name:            req.Name,
			SourcePrefix:    sourcePrefix,
			MatchMode:       req.MatchMode,
			Conditions:      req.Conditions,
			Actions:         req.Actions,
			CooldownSeconds: req.CooldownSeconds,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Enabled != nil && !*req.Enabled {
			rule, _ = s.rules.SetEnabled(rule.ID, false)
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleBeaconReactorRuleAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/compat/beacon-reactor/rules/{id} or /enable|disable
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "compat" || parts[2] != "beacon-reactor" || parts[3] != "rules" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]

	rule, err := s.rules.Get(id)
	if err != nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(rule.SourcePrefix)), "beacon.") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "beacon-reactor rule not found"})
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, rule)
		return
	}
	if len(parts) != 6 || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch parts[5] {
	case "enable":
		rule, err = s.rules.SetEnabled(id, true)
	case "disable":
		rule, err = s.rules.SetEnabled(id, false)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleBeaconReactorEmit(w http.ResponseWriter, r *http.Request) {
	type emitReq struct {
		Beacon  string         `json:"beacon"`
		Message string         `json:"message,omitempty"`
		Host    string         `json:"host,omitempty"`
		Fields  map[string]any `json:"fields,omitempty"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req emitReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	beacon := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(req.Beacon), "beacon."))
	if beacon == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "beacon is required"})
		return
	}
	fields := req.Fields
	if fields == nil {
		fields = map[string]any{}
	}
	if strings.TrimSpace(req.Host) != "" {
		fields["host"] = strings.TrimSpace(req.Host)
	}
	s.recordEvent(control.Event{
		Type:    "beacon." + beacon,
		Message: strings.TrimSpace(req.Message),
		Fields:  fields,
	}, true)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":     "ingested",
		"event_type": "beacon." + beacon,
	})
}

func normalizeBeaconPrefix(raw string) string {
	prefix := strings.TrimSpace(strings.ToLower(raw))
	if prefix == "" {
		return "beacon."
	}
	prefix = strings.TrimPrefix(prefix, "beacon.")
	return "beacon." + prefix
}
