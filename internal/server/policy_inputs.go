package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePolicyInputResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PolicyInputResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := control.ResolvePolicyInputs(r.Context(), s.varSources, req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "conflict") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{
			"error":  err.Error(),
			"result": result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result": result,
	})
}
