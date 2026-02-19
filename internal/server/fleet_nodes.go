package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
)

type fleetNode struct {
	Host          string    `json:"host"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	LastRunStatus string    `json:"last_run_status,omitempty"`
	EventCount    int       `json:"event_count"`
	AlertCount    int       `json:"alert_count"`
	FailureCount  int       `json:"failure_count"`
	RiskScore     int       `json:"risk_score"`
	Workloads     []string  `json:"workloads,omitempty"`
}

type fleetNodeAccumulator struct {
	workloads map[string]struct{}
	node      fleetNode
}

func (s *Server) handleFleetNodes(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		mode := parseFleetRenderMode(r)
		limit := 200
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if mode == "virtualized" && limit > 1000 {
			limit = 1000
		}
		if limit > 2000 {
			limit = 2000
		}
		offset := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
				offset = n
			}
		}
		nodes := s.computeFleetNodes(baseDir)
		total := len(nodes)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := nodes[offset:end]
		nextCursor := ""
		if end < total {
			nextCursor = strconv.Itoa(end)
		}

		items := make([]any, 0, len(page))
		for _, node := range page {
			switch mode {
			case "low-bandwidth":
				items = append(items, map[string]any{
					"host":            node.Host,
					"status":          node.LastRunStatus,
					"risk":            node.RiskScore,
					"alerts":          node.AlertCount,
					"events":          node.EventCount,
					"last_seen_unix":  node.LastSeenAt.Unix(),
					"link_quality":    "degraded-ready",
					"workload_count":  len(node.Workloads),
					"failure_count":   node.FailureCount,
					"bandwidth_bytes": estimateLowBandwidthPayload(node),
				})
			case "virtualized":
				items = append(items, map[string]any{
					"row_id":       node.Host,
					"host":         node.Host,
					"last_seen_at": node.LastSeenAt,
					"status":       node.LastRunStatus,
					"risk_score":   node.RiskScore,
					"alert_count":  node.AlertCount,
				})
			case "compact":
				items = append(items, map[string]any{
					"host":         node.Host,
					"last_seen_at": node.LastSeenAt,
					"risk_score":   node.RiskScore,
					"alert_count":  node.AlertCount,
				})
			default:
				items = append(items, node)
			}
		}
		resp := map[string]any{
			"items":       items,
			"count":       len(items),
			"total":       total,
			"cursor":      strconv.Itoa(offset),
			"next_cursor": nextCursor,
			"limit":       limit,
			"mode":        mode,
		}
		if mode == "virtualized" {
			resp["virtualization"] = map[string]any{
				"enabled":       true,
				"row_height_px": 28,
				"window_start":  offset,
				"window_end":    end,
				"total_rows":    total,
			}
		}
		if mode == "low-bandwidth" {
			resp["transport_hints"] = map[string]any{
				"compression_recommended": true,
				"cache_ttl_seconds":       10,
				"fields_profile":          "minimal",
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func parseFleetRenderMode(r *http.Request) string {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	switch mode {
	case "compact", "virtualized", "low-bandwidth":
		return mode
	case "":
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("compact")), "true") {
			return "compact"
		}
		return "full"
	default:
		return "full"
	}
}

func estimateLowBandwidthPayload(node fleetNode) int {
	// Estimate for clients to reason about paging and degraded network behavior.
	base := 72
	base += len(node.Host)
	base += len(node.LastRunStatus)
	base += (len(node.Workloads) * 8)
	return base
}

func (s *Server) computeFleetNodes(baseDir string) []fleetNode {
	events := s.events.Query(control.EventQuery{Limit: 10_000, Desc: false})
	runs, _ := state.New(baseDir).ListRuns(10_000)

	acc := map[string]*fleetNodeAccumulator{}
	for _, run := range runs {
		for _, res := range run.Results {
			host := strings.TrimSpace(res.Host)
			if host == "" {
				continue
			}
			item := acc[host]
			if item == nil {
				item = &fleetNodeAccumulator{
					workloads: map[string]struct{}{},
					node: fleetNode{
						Host: host,
					},
				}
				acc[host] = item
			}
			if run.StartedAt.After(item.node.LastSeenAt) {
				item.node.LastSeenAt = run.StartedAt
				item.node.LastRunStatus = string(run.Status)
			}
			if run.Status == state.RunFailed {
				item.node.FailureCount++
			}
		}
	}

	for _, evt := range events {
		host := firstNonEmptyField(evt.Fields, "host", "node", "hostname")
		if host == "" {
			continue
		}
		item := acc[host]
		if item == nil {
			item = &fleetNodeAccumulator{
				workloads: map[string]struct{}{},
				node: fleetNode{
					Host: host,
				},
			}
			acc[host] = item
		}
		item.node.EventCount++
		if evt.Time.After(item.node.LastSeenAt) {
			item.node.LastSeenAt = evt.Time
		}
		if strings.Contains(strings.ToLower(evt.Type), "alert") {
			item.node.AlertCount++
		}
		if workload := normalizeWorkload(firstNonEmptyField(evt.Fields, "workload", "service", "application", "app")); workload != "" {
			item.workloads[workload] = struct{}{}
		}
	}

	out := make([]fleetNode, 0, len(acc))
	for _, item := range acc {
		for workload := range item.workloads {
			item.node.Workloads = append(item.node.Workloads, workload)
		}
		sort.Strings(item.node.Workloads)
		item.node.RiskScore = item.node.AlertCount*15 + item.node.FailureCount*20
		if item.node.RiskScore > 100 {
			item.node.RiskScore = 100
		}
		out = append(out, item.node)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RiskScore != out[j].RiskScore {
			return out[i].RiskScore > out[j].RiskScore
		}
		if !out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].LastSeenAt.After(out[j].LastSeenAt)
		}
		return out[i].Host < out[j].Host
	})
	return out
}
