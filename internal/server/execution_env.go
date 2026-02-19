package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleExecutionEnvironments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.executionEnvs.List())
	case http.MethodPost:
		var req control.ExecutionEnvironmentInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.executionEnvs.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.environment.created",
			Message: "hermetic execution environment registered",
			Fields: map[string]any{
				"environment_id": item.ID,
				"image_digest":   item.ImageDigest,
				"signed":         item.Signed,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExecutionEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/execution/environments/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "environments" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.executionEnvs.Get(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution environment not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleExecutionAdmissionPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.executionEnvs.Policy())
	case http.MethodPost:
		var req control.ExecutionAdmissionPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		policy := s.executionEnvs.SetPolicy(req)
		s.recordEvent(control.Event{
			Type:    "execution.admission.policy",
			Message: "execution admission policy updated",
			Fields: map[string]any{
				"require_signed":  policy.RequireSigned,
				"allowed_digests": policy.AllowedDigests,
			},
		}, true)
		writeJSON(w, http.StatusOK, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExecutionAdmissionCheck(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		EnvironmentID string `json:"environment_id"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	envID := strings.TrimSpace(req.EnvironmentID)
	if envID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment_id is required"})
		return
	}
	env, ok := s.executionEnvs.Get(envID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution environment not found"})
		return
	}
	result := s.executionEnvs.EvaluateAdmission(env)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, map[string]any{
		"environment": env,
		"result":      result,
	})
}
