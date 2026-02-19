package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/masterchef/masterchef/internal/config"
)

func (s *Server) handleTopologyAdvisor(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fleetSize := parseIntQuery(r, "fleet_size", -1)
		teamSize := parseIntQuery(r, "team_size", -1)
		if fleetSize < 0 {
			fleetSize = estimateFleetSize(baseDir, s)
		}
		if teamSize < 0 {
			teamSize = 10
		}

		queue := s.queue.ControlStatus()
		profile, rationale, recommendations := adviseTopology(fleetSize, teamSize, queue.Pending)
		writeJSON(w, http.StatusOK, map[string]any{
			"fleet_size":          fleetSize,
			"team_size":           teamSize,
			"queue_pending":       queue.Pending,
			"recommended_profile": profile,
			"rationale":           rationale,
			"recommendations":     recommendations,
		})
	}
}

func estimateFleetSize(baseDir string, s *Server) int {
	total := len(s.nodes.List(""))
	cfgPath := filepath.Join(baseDir, "masterchef.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		if cfg, err := config.Load(cfgPath); err == nil {
			total += len(cfg.Inventory.Hosts)
		}
	}
	return total
}

func adviseTopology(fleetSize, teamSize, queuePending int) (string, string, []string) {
	switch {
	case fleetSize <= 50 && teamSize <= 8:
		return "single-node", "small fleet and compact team footprint", []string{
			"use embedded queue and local object store",
			"run single control-plane binary with nightly backups",
			"enable candidate channel in a staging workspace before production upgrades",
		}
	case fleetSize <= 2000:
		return "ha-control-plane", "mid-size fleet requiring failover and separated state", []string{
			"deploy 3 control-plane replicas behind load balancer",
			"move object store and queue to managed shared backing services",
			"partition workloads by environment using disruption budgets",
		}
	case fleetSize <= 20000:
		return "regional-shards", "large fleet benefits from shard isolation and regional blast-radius control", []string{
			"split control plane by region and service criticality",
			"use event_bus dispatch mode for agent fanout",
			"promote changes through shard-specific gitops promotion pipelines",
		}
	default:
		msg := "very large fleet needs mesh topology and strict queue pressure controls"
		if queuePending > 1000 {
			msg += " with current queue backlog pressure"
		}
		return "global-mesh", msg, []string{
			"run global control plane with regional execution meshes",
			"enforce branch-per-environment materialization and immutable artifact pinning",
			"scale agent dispatch via event bus and monitor fleet health error budgets",
		}
	}
}
