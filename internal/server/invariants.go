package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleInvariantChecks(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Invariants []control.Invariant `json:"invariants"`
		Observed   map[string]float64  `json:"observed"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	report := control.EvaluateInvariants(req.Invariants, req.Observed)
	if !report.Pass {
		writeJSON(w, http.StatusConflict, report)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
