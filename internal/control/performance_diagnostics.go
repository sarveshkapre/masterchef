package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProfileSessionInput struct {
	Component     string `json:"component"`
	Environment   string `json:"environment"`
	DurationSec   int    `json:"duration_sec"`
	SamplingHz    int    `json:"sampling_hz,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type ProfileSession struct {
	ID            string    `json:"id"`
	Component     string    `json:"component"`
	Environment   string    `json:"environment"`
	DurationSec   int       `json:"duration_sec"`
	SamplingHz    int       `json:"sampling_hz"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type BottleneckDiagnosticsInput struct {
	Component         string  `json:"component"`
	Environment       string  `json:"environment"`
	QueueDepth        int     `json:"queue_depth"`
	WorkerUtilization float64 `json:"worker_utilization_percent"`
	P95LatencyMs      int64   `json:"p95_latency_ms"`
	ErrorRatePercent  float64 `json:"error_rate_percent"`
	CPUPercent        float64 `json:"cpu_percent"`
	MemoryPressure    float64 `json:"memory_pressure_percent"`
}

type BottleneckDiagnostics struct {
	Severity            string   `json:"severity"`
	PrimaryBottleneck   string   `json:"primary_bottleneck"`
	ContributingSignals []string `json:"contributing_signals"`
	Recommendations     []string `json:"recommendations"`
}

type PerformanceDiagnosticsStore struct {
	mu       sync.RWMutex
	nextID   int64
	sessions map[string]*ProfileSession
}

func NewPerformanceDiagnosticsStore() *PerformanceDiagnosticsStore {
	return &PerformanceDiagnosticsStore{sessions: map[string]*ProfileSession{}}
}

func (s *PerformanceDiagnosticsStore) StartSession(in ProfileSessionInput) (ProfileSession, error) {
	component := strings.ToLower(strings.TrimSpace(in.Component))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if component == "" || environment == "" {
		return ProfileSession{}, errors.New("component and environment are required")
	}
	duration := in.DurationSec
	if duration <= 0 {
		duration = 300
	}
	sampling := in.SamplingHz
	if sampling <= 0 {
		sampling = 10
	}
	item := ProfileSession{
		Component:     component,
		Environment:   environment,
		DurationSec:   duration,
		SamplingHz:    sampling,
		CorrelationID: strings.TrimSpace(in.CorrelationID),
		CreatedAt:     time.Now().UTC(),
	}

	s.mu.Lock()
	s.nextID++
	item.ID = "profile-session-" + itoa(s.nextID)
	s.sessions[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *PerformanceDiagnosticsStore) ListSessions() []ProfileSession {
	s.mu.RLock()
	out := make([]ProfileSession, 0, len(s.sessions))
	for _, item := range s.sessions {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *PerformanceDiagnosticsStore) Diagnose(in BottleneckDiagnosticsInput) (BottleneckDiagnostics, error) {
	component := strings.ToLower(strings.TrimSpace(in.Component))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if component == "" || environment == "" {
		return BottleneckDiagnostics{}, errors.New("component and environment are required")
	}
	if in.QueueDepth < 0 || in.P95LatencyMs < 0 {
		return BottleneckDiagnostics{}, errors.New("queue_depth and p95_latency_ms must be non-negative")
	}
	if in.WorkerUtilization < 0 || in.ErrorRatePercent < 0 || in.CPUPercent < 0 || in.MemoryPressure < 0 {
		return BottleneckDiagnostics{}, errors.New("percent metrics must be non-negative")
	}

	severity := "low"
	primary := "none"
	signals := make([]string, 0, 4)
	recommendations := make([]string, 0, 4)

	if in.QueueDepth > 500 {
		severity = "high"
		primary = "queue_saturation"
		signals = append(signals, "queue_depth_above_500")
		recommendations = append(recommendations, "increase worker pool or apply stricter admission controls")
	} else if in.QueueDepth > 150 {
		if severity != "high" {
			severity = "medium"
		}
		if primary == "none" {
			primary = "queue_pressure"
		}
		signals = append(signals, "queue_depth_above_150")
		recommendations = append(recommendations, "rebalance run scheduling and inspect long-running steps")
	}
	if in.WorkerUtilization > 90 {
		if severity != "high" {
			severity = "high"
		}
		if primary == "none" || primary == "queue_pressure" {
			primary = "worker_saturation"
		}
		signals = append(signals, "worker_utilization_above_90")
		recommendations = append(recommendations, "scale workers and evaluate queue partition fairness")
	}
	if in.P95LatencyMs > 180000 {
		if severity != "high" {
			severity = "high"
		}
		if primary == "none" {
			primary = "execution_latency"
		}
		signals = append(signals, "p95_latency_above_180000ms")
		recommendations = append(recommendations, "profile provider hotspots and optimize I/O-heavy tasks")
	}
	if in.CPUPercent > 85 {
		if severity == "low" {
			severity = "medium"
		}
		if primary == "none" {
			primary = "cpu_contention"
		}
		signals = append(signals, "cpu_above_85_percent")
		recommendations = append(recommendations, "pin hot workers and reduce concurrent expensive providers")
	}
	if in.MemoryPressure > 85 {
		if severity != "high" {
			severity = "high"
		}
		if primary == "none" || primary == "cpu_contention" {
			primary = "memory_pressure"
		}
		signals = append(signals, "memory_pressure_above_85_percent")
		recommendations = append(recommendations, "enable shorter worker recycle windows and inspect memory leaks")
	}
	if in.ErrorRatePercent > 5 {
		if severity == "low" {
			severity = "medium"
		}
		signals = append(signals, "error_rate_above_5_percent")
		recommendations = append(recommendations, "triage retries and isolate failing provider/resource types")
	}
	if primary == "none" {
		primary = "healthy"
		signals = append(signals, "within_baseline")
		recommendations = append(recommendations, "no bottleneck detected; continue baseline monitoring")
	}
	return BottleneckDiagnostics{
		Severity:            severity,
		PrimaryBottleneck:   primary,
		ContributingSignals: dedupeStrings(signals),
		Recommendations:     dedupeStrings(recommendations),
	}, nil
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
