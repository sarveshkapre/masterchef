package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleScaleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		nodeCount := parseIntQuery(r, "node_count", 0)
		tenantCount := parseIntQuery(r, "tenant_count", 0)
		regionCount := parseIntQuery(r, "region_count", 0)
		queueDepth := parseIntQuery(r, "queue_depth", 0)
		resp := map[string]any{
			"profiles": control.BuiltinScaleProfiles(),
		}
		if nodeCount > 0 || tenantCount > 0 || regionCount > 0 || queueDepth > 0 {
			resp["evaluation"] = control.EvaluateScaleProfile(control.ScaleProfileEvaluateInput{
				NodeCount:   nodeCount,
				TenantCount: tenantCount,
				RegionCount: regionCount,
				QueueDepth:  queueDepth,
			})
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req control.ScaleProfileEvaluateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		writeJSON(w, http.StatusOK, control.EvaluateScaleProfile(req))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
