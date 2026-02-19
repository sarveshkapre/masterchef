package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAgentCertPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.agentPKI.Policy())
	case http.MethodPost:
		var req control.AgentCertificatePolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item := s.agentPKI.SetPolicy(req)
		s.recordEvent(control.Event{
			Type:    "agents.pki.policy.updated",
			Message: "agent cert policy updated",
			Fields: map[string]any{
				"auto_approve":        item.AutoApprove,
				"required_attributes": item.RequiredAttributes,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentCSRs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.agentPKI.ListCSRs())
	case http.MethodPost:
		var req control.AgentCSRInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.agentPKI.SubmitCSR(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "agents.pki.csr.submitted",
			Message: "agent csr submitted",
			Fields: map[string]any{
				"csr_id":   item.ID,
				"agent_id": item.AgentID,
				"status":   item.Status,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentCSRAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/agents/csrs/{id}/approve|reject
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "agents" || parts[2] != "csrs" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	action := parts[4]
	var (
		item control.AgentCSR
		err  error
	)
	switch action {
	case "approve":
		item, err = s.agentPKI.DecideCSR(parts[3], "approve", req.Reason)
	case "reject":
		item, err = s.agentPKI.DecideCSR(parts[3], "reject", req.Reason)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown csr action"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAgentCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.agentPKI.ListCertificates())
}

func (s *Server) handleAgentCertificateAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/agents/certificates/{id}/revoke
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "agents" || parts[2] != "certificates" || parts[4] != "revoke" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.agentPKI.RevokeCertificate(parts[3])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAgentCertificateRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := s.agentPKI.RotateAgentCertificate(req.AgentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
