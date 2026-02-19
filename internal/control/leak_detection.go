package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type LeakDetectionPolicy struct {
	MinSamples               int       `json:"min_samples"`
	MemoryGrowthPercent      float64   `json:"memory_growth_percent"`
	GoroutineGrowthPercent   float64   `json:"goroutine_growth_percent"`
	FileDescriptorGrowthPerc float64   `json:"file_descriptor_growth_percent"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ResourceSnapshot struct {
	Component  string `json:"component"`
	MemoryMB   int    `json:"memory_mb"`
	Goroutines int    `json:"goroutines"`
	OpenFDs    int    `json:"open_fds"`
}

type LeakReport struct {
	Component              string    `json:"component"`
	Samples                int       `json:"samples"`
	MemoryGrowthPercent    float64   `json:"memory_growth_percent"`
	GoroutineGrowthPercent float64   `json:"goroutine_growth_percent"`
	FDGrowthPercent        float64   `json:"fd_growth_percent"`
	LeakDetected           bool      `json:"leak_detected"`
	Reasons                []string  `json:"reasons,omitempty"`
	LastSeenAt             time.Time `json:"last_seen_at"`
}

type snapshotRecord struct {
	ResourceSnapshot
	ObservedAt time.Time
}

type LeakDetectionStore struct {
	mu      sync.RWMutex
	policy  LeakDetectionPolicy
	samples map[string][]snapshotRecord
}

func NewLeakDetectionStore() *LeakDetectionStore {
	return &LeakDetectionStore{
		policy: LeakDetectionPolicy{
			MinSamples:               4,
			MemoryGrowthPercent:      20,
			GoroutineGrowthPercent:   25,
			FileDescriptorGrowthPerc: 30,
			UpdatedAt:                time.Now().UTC(),
		},
		samples: map[string][]snapshotRecord{},
	}
}

func (s *LeakDetectionStore) Policy() LeakDetectionPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *LeakDetectionStore) SetPolicy(in LeakDetectionPolicy) (LeakDetectionPolicy, error) {
	if in.MinSamples <= 1 {
		in.MinSamples = 4
	}
	if in.MemoryGrowthPercent < 0 || in.GoroutineGrowthPercent < 0 || in.FileDescriptorGrowthPerc < 0 {
		return LeakDetectionPolicy{}, errors.New("growth thresholds must be non-negative")
	}
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = in
	s.mu.Unlock()
	return in, nil
}

func (s *LeakDetectionStore) Observe(in ResourceSnapshot) (LeakReport, error) {
	component := strings.TrimSpace(in.Component)
	if component == "" {
		return LeakReport{}, errors.New("component is required")
	}
	if in.MemoryMB < 0 || in.Goroutines < 0 || in.OpenFDs < 0 {
		return LeakReport{}, errors.New("resource metrics must be non-negative")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	records := append(s.samples[component], snapshotRecord{ResourceSnapshot: in, ObservedAt: now})
	if len(records) > 200 {
		records = records[len(records)-200:]
	}
	s.samples[component] = records
	policy := s.policy
	s.mu.Unlock()
	return evaluateLeakReport(component, records, policy), nil
}

func (s *LeakDetectionStore) Reports() []LeakReport {
	s.mu.RLock()
	components := make([]string, 0, len(s.samples))
	for component := range s.samples {
		components = append(components, component)
	}
	policy := s.policy
	data := make(map[string][]snapshotRecord, len(s.samples))
	for component, records := range s.samples {
		cloned := make([]snapshotRecord, len(records))
		copy(cloned, records)
		data[component] = cloned
	}
	s.mu.RUnlock()
	sort.Strings(components)
	out := make([]LeakReport, 0, len(components))
	for _, component := range components {
		out = append(out, evaluateLeakReport(component, data[component], policy))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LeakDetected != out[j].LeakDetected {
			return out[i].LeakDetected
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out
}

func evaluateLeakReport(component string, records []snapshotRecord, policy LeakDetectionPolicy) LeakReport {
	report := LeakReport{
		Component: component,
		Samples:   len(records),
	}
	if len(records) == 0 {
		return report
	}
	first := records[0]
	last := records[len(records)-1]
	report.LastSeenAt = last.ObservedAt
	report.MemoryGrowthPercent = growthPercent(first.MemoryMB, last.MemoryMB)
	report.GoroutineGrowthPercent = growthPercent(first.Goroutines, last.Goroutines)
	report.FDGrowthPercent = growthPercent(first.OpenFDs, last.OpenFDs)
	if len(records) < policy.MinSamples {
		return report
	}
	reasons := make([]string, 0)
	if report.MemoryGrowthPercent >= policy.MemoryGrowthPercent {
		reasons = append(reasons, "memory growth exceeded threshold")
	}
	if report.GoroutineGrowthPercent >= policy.GoroutineGrowthPercent {
		reasons = append(reasons, "goroutine growth exceeded threshold")
	}
	if report.FDGrowthPercent >= policy.FileDescriptorGrowthPerc {
		reasons = append(reasons, "file descriptor growth exceeded threshold")
	}
	report.LeakDetected = len(reasons) > 0
	report.Reasons = reasons
	return report
}

func growthPercent(first, last int) float64 {
	if first <= 0 {
		if last <= 0 {
			return 0
		}
		return 100
	}
	return (float64(last-first) / float64(first)) * 100
}
