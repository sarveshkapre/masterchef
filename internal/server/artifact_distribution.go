package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleArtifactDistributionPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.artifactDistribution.List())
	case http.MethodPost:
		var req control.ArtifactDistributionPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.artifactDistribution.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.artifact_distribution.policy.updated",
			Message: "artifact distribution policy updated",
			Fields: map[string]any{
				"policy_id":                  item.ID,
				"environment":                item.Environment,
				"max_transfer_mbps":          item.MaxTransferMbps,
				"cache_warm_threshold_mb":    item.CacheWarmThresholdMB,
				"prefer_regional_cache":      item.PreferRegionalCache,
				"relay_on_constrained_links": item.RelayOnConstrainedLinks,
				"compression":                item.Compression,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleArtifactDistributionPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ArtifactDistributionPlanInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision, err := s.artifactDistribution.Plan(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
