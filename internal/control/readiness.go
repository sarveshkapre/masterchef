package control

import "time"

type ReadinessSignals struct {
	QualityScore          float64 `json:"quality_score"`
	ReliabilityScore      float64 `json:"reliability_score"`
	PerformanceScore      float64 `json:"performance_score"`
	TestPassRate          float64 `json:"test_pass_rate"`
	FlakeRate             float64 `json:"flake_rate"`
	OpenCriticalIncidents int     `json:"open_critical_incidents"`
	P95ApplyLatencyMs     int64   `json:"p95_apply_latency_ms"`
}

type ReadinessThresholds struct {
	MinQualityScore      float64 `json:"min_quality_score"`
	MinReliabilityScore  float64 `json:"min_reliability_score"`
	MinPerformanceScore  float64 `json:"min_performance_score"`
	MinTestPassRate      float64 `json:"min_test_pass_rate"`
	MaxFlakeRate         float64 `json:"max_flake_rate"`
	MaxCriticalIncidents int     `json:"max_critical_incidents"`
	MaxP95ApplyLatencyMs int64   `json:"max_p95_apply_latency_ms"`
}

type ReadinessReport struct {
	GeneratedAt    time.Time           `json:"generated_at"`
	Signals        ReadinessSignals    `json:"signals"`
	Thresholds     ReadinessThresholds `json:"thresholds"`
	AggregateScore float64             `json:"aggregate_score"`
	Pass           bool                `json:"pass"`
	Blockers       []string            `json:"blockers"`
}

func DefaultReadinessThresholds() ReadinessThresholds {
	return ReadinessThresholds{
		MinQualityScore:      0.85,
		MinReliabilityScore:  0.90,
		MinPerformanceScore:  0.85,
		MinTestPassRate:      0.98,
		MaxFlakeRate:         0.02,
		MaxCriticalIncidents: 0,
		MaxP95ApplyLatencyMs: 120000,
	}
}

func EvaluateReadiness(signals ReadinessSignals, thresholds ReadinessThresholds) ReadinessReport {
	if thresholds.MinQualityScore <= 0 {
		thresholds = DefaultReadinessThresholds()
	}
	blockers := make([]string, 0)
	if signals.QualityScore < thresholds.MinQualityScore {
		blockers = append(blockers, "quality_score below minimum")
	}
	if signals.ReliabilityScore < thresholds.MinReliabilityScore {
		blockers = append(blockers, "reliability_score below minimum")
	}
	if signals.PerformanceScore < thresholds.MinPerformanceScore {
		blockers = append(blockers, "performance_score below minimum")
	}
	if signals.TestPassRate < thresholds.MinTestPassRate {
		blockers = append(blockers, "test_pass_rate below minimum")
	}
	if signals.FlakeRate > thresholds.MaxFlakeRate {
		blockers = append(blockers, "flake_rate above maximum")
	}
	if signals.OpenCriticalIncidents > thresholds.MaxCriticalIncidents {
		blockers = append(blockers, "open_critical_incidents above maximum")
	}
	if signals.P95ApplyLatencyMs > thresholds.MaxP95ApplyLatencyMs {
		blockers = append(blockers, "p95_apply_latency_ms above maximum")
	}

	agg := (signals.QualityScore + signals.ReliabilityScore + signals.PerformanceScore + signals.TestPassRate + (1.0 - signals.FlakeRate)) / 5.0
	if agg < 0 {
		agg = 0
	}
	if agg > 1 {
		agg = 1
	}

	return ReadinessReport{
		GeneratedAt:    time.Now().UTC(),
		Signals:        signals,
		Thresholds:     thresholds,
		AggregateScore: agg,
		Pass:           len(blockers) == 0,
		Blockers:       blockers,
	}
}
