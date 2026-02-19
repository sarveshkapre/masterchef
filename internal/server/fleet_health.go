package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func (s *Server) handleFleetHealth(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		hours := parseIntQuery(r, "hours", 24)
		if hours <= 0 {
			hours = 24
		}
		if hours > 24*30 {
			hours = 24 * 30
		}
		slo := parseFloatQuery(r, "slo", 99.9)
		if slo < 50 {
			slo = 50
		}
		if slo > 100 {
			slo = 100
		}
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		runs, err := state.New(baseDir).ListRuns(5000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total := 0
		succeeded := 0
		failed := 0
		hostFailures := map[string]int{}
		for _, run := range runs {
			ref := run.StartedAt
			if ref.IsZero() {
				ref = run.EndedAt
			}
			if ref.IsZero() || ref.Before(since) {
				continue
			}
			total++
			if run.Status == state.RunSucceeded {
				succeeded++
			}
			if run.Status == state.RunFailed {
				failed++
				for _, res := range run.Results {
					host := strings.TrimSpace(res.Host)
					if host == "" {
						host = "unknown-host"
					}
					hostFailures[host]++
				}
			}
		}
		availability := 100.0
		if total > 0 {
			availability = (float64(succeeded) / float64(total)) * 100
		}
		allowedFailures := (float64(total) * (100 - slo)) / 100
		remainingBudget := allowedFailures - float64(failed)
		burnRate := 0.0
		if allowedFailures > 0 {
			burnRate = float64(failed) / allowedFailures
		} else if failed > 0 {
			burnRate = 9999
		}
		status := "healthy"
		if remainingBudget < 0 {
			status = "breached"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"window_hours":       hours,
			"slo_target_percent": slo,
			"since":              since,
			"runs_total":         total,
			"runs_succeeded":     succeeded,
			"runs_failed":        failed,
			"availability_pct":   availability,
			"error_budget": map[string]any{
				"allowed_failures":   allowedFailures,
				"consumed_failures":  failed,
				"remaining_failures": remainingBudget,
				"burn_rate":          burnRate,
				"status":             status,
			},
			"top_failing_hosts": topHostCounts(hostFailures, 10),
		})
	}
}

func parseIntQuery(r *http.Request, key string, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func parseFloatQuery(r *http.Request, key string, def float64) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return n
}

func topHostCounts(m map[string]int, limit int) []map[string]any {
	type kv struct {
		Host  string
		Count int
	}
	items := make([]kv, 0, len(m))
	for k, v := range m {
		items = append(items, kv{Host: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Host < items[j].Host
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"host": item.Host, "failed_runs": item.Count})
	}
	return out
}
