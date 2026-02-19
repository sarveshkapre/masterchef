package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleHABootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.HABootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	plan, err := control.BuildHABootstrapPlan(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "control.bootstrap.ha.plan",
		Message: "single-region HA bootstrap plan generated",
		Fields: map[string]any{
			"cluster_name":  plan.ClusterName,
			"region":        plan.Region,
			"replicas":      plan.Replicas,
			"object_store":  plan.ObjectStore,
			"queue_backend": plan.QueueBackend,
		},
	}, true)
	writeJSON(w, http.StatusOK, plan)
}
