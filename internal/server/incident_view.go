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

type observabilityLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type incidentDriftSignals struct {
	WindowHours      int          `json:"window_hours"`
	Samples          int          `json:"samples"`
	Changed          int          `json:"changed"`
	Suppressed       int          `json:"suppressed"`
	Allowlisted      int          `json:"allowlisted"`
	DriftRatePercent float64      `json:"drift_rate_percent"`
	TopHosts         []driftTrend `json:"top_hosts"`
	TopTypes         []driftTrend `json:"top_types"`
}

type incidentHealthSignals struct {
	MatchedTargets int                           `json:"matched_targets"`
	Gate           control.HealthProbeGateResult `json:"gate"`
}

func (s *Server) handleIncidentView(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		workload := normalizeWorkload(r.URL.Query().Get("workload"))
		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		hours := 6
		if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				hours = n
			}
		}
		if hours > 24*7 {
			hours = 24 * 7
		}
		limit := 300
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 2000 {
			limit = 2000
		}

		windowStart := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		events := s.events.Query(control.EventQuery{Since: windowStart, Limit: limit, Desc: true})
		correlatedEvents := make([]control.Event, 0, len(events))
		for _, evt := range events {
			if !incidentMatches(evt, workload, runID) {
				continue
			}
			correlatedEvents = append(correlatedEvents, evt)
		}

		alerts := s.alerts.List("all", limit)
		correlatedAlerts := make([]control.AlertItem, 0, len(alerts))
		for _, alert := range alerts {
			if incidentAlertMatches(alert, workload, runID) {
				correlatedAlerts = append(correlatedAlerts, alert)
			}
		}

		runs, _ := state.New(baseDir).ListRuns(limit)
		correlatedRuns := make([]state.RunRecord, 0, len(runs))
		for _, run := range runs {
			if incidentRunMatches(run, workload, runID) {
				correlatedRuns = append(correlatedRuns, run)
			}
		}

		links := collectObservabilityLinks(correlatedEvents)
		canary := s.canaries.HealthSummary()
		driftSignals := buildIncidentDriftSignals(correlatedRuns, s.driftPolicies, windowStart, hours)
		healthSignals := buildIncidentHealthSignals(workload, correlatedRuns, s.healthProbes)
		riskScore := incidentRiskScore(correlatedAlerts, correlatedRuns, canary, driftSignals, healthSignals)
		riskLevel := "low"
		if riskScore >= 60 {
			riskLevel = "high"
		} else if riskScore >= 30 {
			riskLevel = "medium"
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"workload":             workload,
			"run_id":               runID,
			"window_hours":         hours,
			"window_start":         windowStart,
			"generated_at":         time.Now().UTC(),
			"risk_score":           riskScore,
			"risk_level":           riskLevel,
			"canary_health":        canary,
			"events":               correlatedEvents,
			"alerts":               correlatedAlerts,
			"runs":                 correlatedRuns,
			"drift_signals":        driftSignals,
			"health_signals":       healthSignals,
			"observability_links":  links,
			"summary":              incidentSummary(correlatedAlerts, correlatedRuns, correlatedEvents, driftSignals, healthSignals),
			"suggested_next_steps": incidentNextSteps(riskScore, correlatedAlerts, correlatedRuns, driftSignals, healthSignals),
		})
	}
}

func incidentMatches(evt control.Event, workload, runID string) bool {
	if workload == "" && runID == "" {
		return true
	}
	if runID != "" {
		if v := firstNonEmptyField(evt.Fields, "run_id", "job_id"); strings.EqualFold(strings.TrimSpace(v), runID) {
			return true
		}
		if strings.Contains(strings.ToLower(evt.Message), strings.ToLower(runID)) {
			return true
		}
	}
	if workload != "" {
		if normalizeWorkload(firstNonEmptyField(evt.Fields, "workload", "service", "application", "app")) == workload {
			return true
		}
		if strings.Contains(normalizeWorkload(evt.Message), workload) {
			return true
		}
	}
	return false
}

func incidentAlertMatches(alert control.AlertItem, workload, runID string) bool {
	if workload == "" && runID == "" {
		return true
	}
	if runID != "" {
		if strings.EqualFold(firstNonEmptyField(alert.Fields, "run_id", "job_id"), runID) {
			return true
		}
		if strings.Contains(strings.ToLower(alert.Message), strings.ToLower(runID)) {
			return true
		}
	}
	if workload != "" {
		if normalizeWorkload(firstNonEmptyField(alert.Fields, "workload", "service", "application", "app")) == workload {
			return true
		}
		if strings.Contains(normalizeWorkload(alert.Message), workload) {
			return true
		}
	}
	return false
}

func incidentRunMatches(run state.RunRecord, workload, runID string) bool {
	if runID != "" && strings.EqualFold(run.ID, runID) {
		return true
	}
	if runID != "" && !strings.EqualFold(run.ID, runID) && workload == "" {
		return false
	}
	if workload == "" {
		return runID == ""
	}
	for _, result := range run.Results {
		host := normalizeWorkload(result.Host)
		resource := normalizeWorkload(result.ResourceID + " " + result.Type + " " + result.Message)
		if strings.Contains(host, workload) || strings.Contains(resource, workload) {
			return true
		}
	}
	return false
}

func collectObservabilityLinks(events []control.Event) []observabilityLink {
	seen := map[string]struct{}{}
	out := make([]observabilityLink, 0, 32)
	appendLink := func(label, url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		key := label + "|" + url
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, observabilityLink{Label: label, URL: url})
	}
	for _, evt := range events {
		appendLink("dashboard", firstNonEmptyField(evt.Fields, "dashboard_url"))
		appendLink("logs", firstNonEmptyField(evt.Fields, "logs_url"))
		appendLink("trace", firstNonEmptyField(evt.Fields, "trace_url"))
		if traceID := firstNonEmptyField(evt.Fields, "trace_id"); traceID != "" {
			appendLink("trace-id", "otel://trace/"+traceID)
		}
		if spanID := firstNonEmptyField(evt.Fields, "span_id"); spanID != "" {
			appendLink("span-id", "otel://span/"+spanID)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func incidentRiskScore(alerts []control.AlertItem, runs []state.RunRecord, canary map[string]any, drift incidentDriftSignals, health incidentHealthSignals) int {
	score := 0
	for _, alert := range alerts {
		switch strings.ToLower(alert.Severity) {
		case "critical":
			score += 30
		case "high":
			score += 20
		case "medium":
			score += 10
		default:
			score += 5
		}
		if alert.Status == control.AlertOpen {
			score += 5
		}
	}
	for _, run := range runs {
		if run.Status == state.RunFailed {
			score += 20
		}
	}
	if status, _ := canary["status"].(string); strings.EqualFold(status, "degraded") {
		score += 20
	}
	if drift.Samples > 0 {
		switch {
		case drift.DriftRatePercent >= 30:
			score += 25
		case drift.DriftRatePercent >= 15:
			score += 15
		case drift.DriftRatePercent > 0:
			score += 5
		}
		if drift.Changed >= 20 {
			score += 10
		}
	}
	if strings.EqualFold(health.Gate.Decision, "block") {
		score += 20
		if strings.EqualFold(health.Gate.RecommendedAction, "rollback") {
			score += 5
		}
	}
	if score > 100 {
		score = 100
	}
	return score
}

func incidentSummary(alerts []control.AlertItem, runs []state.RunRecord, events []control.Event, drift incidentDriftSignals, health incidentHealthSignals) map[string]any {
	failedRuns := 0
	for _, run := range runs {
		if run.Status == state.RunFailed {
			failedRuns++
		}
	}
	openAlerts := 0
	for _, alert := range alerts {
		if alert.Status == control.AlertOpen {
			openAlerts++
		}
	}
	return map[string]any{
		"event_count":            len(events),
		"alert_count":            len(alerts),
		"open_alerts":            openAlerts,
		"run_count":              len(runs),
		"failed_runs":            failedRuns,
		"drift_changed":          drift.Changed,
		"drift_rate_percent":     drift.DriftRatePercent,
		"health_gate_decision":   health.Gate.Decision,
		"health_checked_targets": health.Gate.CheckedTargets,
		"signals_count":          len(events) + len(alerts) + len(runs) + 2,
	}
}

func incidentNextSteps(risk int, alerts []control.AlertItem, runs []state.RunRecord, drift incidentDriftSignals, health incidentHealthSignals) []string {
	steps := make([]string, 0, 4)
	if risk >= 60 {
		steps = append(steps, "pause high-risk rollouts and initiate incident bridge")
	}
	if len(alerts) > 0 {
		steps = append(steps, "triage highest-severity open alerts and validate suppression state")
	}
	if strings.EqualFold(health.Gate.Decision, "block") {
		steps = append(steps, "health probes are failing the gate; hold rollout and follow rollback playbook if risk rises")
	}
	if drift.Changed > 0 {
		steps = append(steps, "inspect drift with /v1/drift/insights and evaluate /v1/drift/remediate for approved fixes")
	}
	for _, run := range runs {
		if run.Status == state.RunFailed {
			steps = append(steps, "inspect failed runs with /v1/runs/{id}/timeline and triage bundle export")
			break
		}
	}
	if len(steps) == 0 {
		steps = append(steps, "continue monitoring correlated signals and keep rollout guardrails active")
	}
	return steps
}

func buildIncidentDriftSignals(runs []state.RunRecord, policies *control.DriftPolicyStore, windowStart time.Time, hours int) incidentDriftSignals {
	hostTrends := map[string]*driftTrend{}
	typeTrends := map[string]*driftTrend{}
	samples := 0
	changed := 0
	suppressed := 0
	allowlisted := 0
	for _, run := range runs {
		ref := run.StartedAt
		if ref.IsZero() {
			ref = run.EndedAt
		}
		if !ref.IsZero() && ref.Before(windowStart) {
			continue
		}
		for _, res := range run.Results {
			if res.Skipped {
				continue
			}
			samples++
			if !res.Changed {
				continue
			}
			if policies != nil && policies.IsSuppressed(res.Host, res.Type, res.ResourceID, ref) {
				suppressed++
				continue
			}
			if policies != nil && policies.IsAllowlisted(res.Host, res.Type, res.ResourceID, ref) {
				allowlisted++
				continue
			}
			changed++
			hostKey := strings.TrimSpace(res.Host)
			if hostKey == "" {
				hostKey = "unknown-host"
			}
			if hostTrends[hostKey] == nil {
				hostTrends[hostKey] = &driftTrend{Key: hostKey}
			}
			hostTrends[hostKey].Count++
			hostTrends[hostKey].LastSeen = maxTime(hostTrends[hostKey].LastSeen, ref)

			typeKey := strings.TrimSpace(strings.ToLower(res.Type))
			if typeKey == "" {
				typeKey = "unknown-type"
			}
			if typeTrends[typeKey] == nil {
				typeTrends[typeKey] = &driftTrend{Key: typeKey}
			}
			typeTrends[typeKey].Count++
			typeTrends[typeKey].LastSeen = maxTime(typeTrends[typeKey].LastSeen, ref)
		}
	}
	driftRate := 0.0
	if samples > 0 {
		driftRate = (float64(changed) / float64(samples)) * 100
	}
	return incidentDriftSignals{
		WindowHours:      hours,
		Samples:          samples,
		Changed:          changed,
		Suppressed:       suppressed,
		Allowlisted:      allowlisted,
		DriftRatePercent: driftRate,
		TopHosts:         sortDriftTrends(hostTrends, 5),
		TopTypes:         sortDriftTrends(typeTrends, 5),
	}
}

func buildIncidentHealthSignals(workload string, runs []state.RunRecord, probes *control.HealthProbeStore) incidentHealthSignals {
	unavailable := incidentHealthSignals{
		MatchedTargets: 0,
		Gate: control.HealthProbeGateResult{
			Decision:    "unavailable",
			Reason:      "no matching health probes found for incident scope",
			GeneratedAt: time.Now().UTC(),
		},
	}
	if probes == nil {
		return unavailable
	}
	targets := probes.ListTargets()
	if len(targets) == 0 {
		return unavailable
	}
	runHosts := map[string]struct{}{}
	for _, run := range runs {
		for _, res := range run.Results {
			host := normalizeWorkload(res.Host)
			if host != "" {
				runHosts[host] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		targetService := normalizeWorkload(target.Service)
		targetName := normalizeWorkload(target.Name)
		match := workload == ""
		if workload != "" {
			match = strings.Contains(targetService, workload) || strings.Contains(targetName, workload)
		}
		if !match && len(runHosts) > 0 {
			if _, ok := runHosts[targetName]; ok {
				match = true
			}
			if _, ok := runHosts[targetService]; ok {
				match = true
			}
		}
		if match {
			ids = append(ids, target.ID)
		}
	}
	if len(ids) == 0 {
		return unavailable
	}
	gate := probes.EvaluateGate(control.HealthProbeGateRequest{
		TargetIDs:         ids,
		MinHealthyPercent: 100,
		RecommendRollback: true,
	})
	return incidentHealthSignals{
		MatchedTargets: len(ids),
		Gate:           gate,
	}
}
