package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAgentAttestationPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.agentAttestation.Policy())
	case http.MethodPost:
		var req control.AgentAttestationPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item := s.agentAttestation.SetPolicy(req)
		s.recordEvent(control.Event{
			Type:    "agents.attestation.policy.updated",
			Message: "agent attestation policy updated",
			Fields: map[string]any{
				"require_before_cert": item.RequireBeforeCert,
				"allowed_providers":   item.AllowedProviders,
				"max_age_minutes":     item.MaxAgeMinutes,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentAttestations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.agentAttestation.List())
	case http.MethodPost:
		var req control.AgentAttestationInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.agentAttestation.Submit(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "agents.attestation.submitted",
			Message: "agent attestation evidence submitted",
			Fields: map[string]any{
				"attestation_id": item.ID,
				"agent_id":       item.AgentID,
				"provider":       item.Provider,
				"verified":       item.Verified,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentAttestationAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/agents/attestations/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "agents" || parts[2] != "attestations" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.agentAttestation.Get(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent attestation evidence not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAgentAttestationCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.agentAttestation.CheckForCertificate(req.AgentID)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
