package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleTopologyPlacementPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.topologyPlacement.List())
	case http.MethodPost:
		var req control.TopologyPlacementPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.topologyPlacement.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.topology_placement.policy.updated",
			Message: "topology placement policy updated",
			Fields: map[string]any{
				"policy_id":      item.ID,
				"environment":    item.Environment,
				"region":         item.Region,
				"zone":           item.Zone,
				"cluster":        item.Cluster,
				"failure_domain": item.FailureDomain,
				"max_parallel":   item.MaxParallel,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTopologyPlacementDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.TopologyPlacementDecisionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.topologyPlacement.Decide(req)
	writeJSON(w, http.StatusOK, decision)
}
