package server

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

type workloadView struct {
	Workload    string    `json:"workload"`
	Services    []string  `json:"services,omitempty"`
	Hosts       []string  `json:"hosts,omitempty"`
	EventCount  int       `json:"event_count"`
	AlertCount  int       `json:"alert_count"`
	LastEventAt time.Time `json:"last_event_at"`
	RiskScore   int       `json:"risk_score"`
}

type workloadAccumulator struct {
	services map[string]struct{}
	hosts    map[string]struct{}
	view     workloadView
}

func (s *Server) handleWorkloadViews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			since = parsed
		}
	}
	items := s.computeWorkloadViews(limit, since)
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
		"since": since,
		"limit": limit,
	})
}

func (s *Server) computeWorkloadViews(limit int, since time.Time) []workloadView {
	if limit <= 0 {
		limit = 200
	}
	events := s.events.Query(control.EventQuery{
		Since: since,
		Limit: 5000,
		Desc:  false,
	})

	acc := map[string]*workloadAccumulator{}
	for _, event := range events {
		workload := workloadFromEvent(event)
		if workload == "" {
			continue
		}
		item, ok := acc[workload]
		if !ok {
			item = &workloadAccumulator{
				services: map[string]struct{}{},
				hosts:    map[string]struct{}{},
				view: workloadView{
					Workload: workload,
				},
			}
			acc[workload] = item
		}
		item.view.EventCount++
		if event.Time.After(item.view.LastEventAt) {
			item.view.LastEventAt = event.Time
		}
		if strings.Contains(strings.ToLower(event.Type), "alert") {
			item.view.AlertCount++
		}
		if strings.Contains(strings.ToLower(event.Type), "failed") {
			item.view.RiskScore += 15
		}
		if strings.Contains(strings.ToLower(event.Type), "blocked") {
			item.view.RiskScore += 10
		}
		if service := firstNonEmptyField(event.Fields, "service", "application", "app"); service != "" {
			item.services[service] = struct{}{}
		}
		if host := firstNonEmptyField(event.Fields, "host", "node", "hostname"); host != "" {
			item.hosts[host] = struct{}{}
		}
	}

	out := make([]workloadView, 0, len(acc))
	for _, item := range acc {
		for service := range item.services {
			item.view.Services = append(item.view.Services, service)
		}
		for host := range item.hosts {
			item.view.Hosts = append(item.view.Hosts, host)
		}
		sort.Strings(item.view.Services)
		sort.Strings(item.view.Hosts)
		item.view.RiskScore += item.view.AlertCount * 20
		if item.view.RiskScore > 100 {
			item.view.RiskScore = 100
		}
		out = append(out, item.view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RiskScore != out[j].RiskScore {
			return out[i].RiskScore > out[j].RiskScore
		}
		if !out[i].LastEventAt.Equal(out[j].LastEventAt) {
			return out[i].LastEventAt.After(out[j].LastEventAt)
		}
		return out[i].Workload < out[j].Workload
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func workloadFromEvent(event control.Event) string {
	if v := firstNonEmptyField(event.Fields, "workload", "service", "application", "app"); v != "" {
		return normalizeWorkload(v)
	}
	if v := firstNonEmptyField(event.Fields, "config_path"); v != "" {
		base := filepath.Base(strings.TrimSpace(v))
		base = strings.TrimSuffix(base, filepath.Ext(base))
		return normalizeWorkload(base)
	}
	return ""
}

func firstNonEmptyField(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := fields[key]
		if !ok {
			continue
		}
		switch value := v.(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func normalizeWorkload(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.Join(strings.Fields(raw), "-")
	return raw
}
