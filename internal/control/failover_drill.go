package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RegionalFailoverDrillInput struct {
	Region              string `json:"region"`
	TargetRTOSeconds    int    `json:"target_rto_seconds"`
	SimulatedRecoveryMs int64  `json:"simulated_recovery_ms"`
	Notes               string `json:"notes,omitempty"`
}

type RegionalFailoverDrillRun struct {
	ID             string    `json:"id"`
	Region         string    `json:"region"`
	TargetRTOMs    int64     `json:"target_rto_ms"`
	RecoveryTimeMs int64     `json:"recovery_time_ms"`
	Pass           bool      `json:"pass"`
	Notes          string    `json:"notes,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
}

type RegionalFailoverScorecard struct {
	Region            string    `json:"region"`
	WindowHours       int       `json:"window_hours"`
	DrillCount        int       `json:"drill_count"`
	PassCount         int       `json:"pass_count"`
	PassRate          float64   `json:"pass_rate"`
	AverageRecoveryMs int64     `json:"average_recovery_ms"`
	WorstRecoveryMs   int64     `json:"worst_recovery_ms"`
	LatestCompletedAt time.Time `json:"latest_completed_at"`
}

type RegionalFailoverDrillStore struct {
	mu   sync.RWMutex
	next int64
	runs []RegionalFailoverDrillRun
}

func NewRegionalFailoverDrillStore() *RegionalFailoverDrillStore {
	return &RegionalFailoverDrillStore{runs: make([]RegionalFailoverDrillRun, 0, 1024)}
}

func (s *RegionalFailoverDrillStore) Run(in RegionalFailoverDrillInput) (RegionalFailoverDrillRun, error) {
	region := strings.ToLower(strings.TrimSpace(in.Region))
	if region == "" {
		return RegionalFailoverDrillRun{}, errors.New("region is required")
	}
	target := int64(in.TargetRTOSeconds) * 1000
	if target <= 0 {
		target = 300000
	}
	recovery := in.SimulatedRecoveryMs
	if recovery <= 0 {
		recovery = target - 1000
		if recovery < 500 {
			recovery = 500
		}
	}
	start := time.Now().UTC()
	item := RegionalFailoverDrillRun{
		Region:         region,
		TargetRTOMs:    target,
		RecoveryTimeMs: recovery,
		Pass:           recovery <= target,
		Notes:          strings.TrimSpace(in.Notes),
		StartedAt:      start,
		CompletedAt:    start.Add(time.Duration(recovery) * time.Millisecond),
	}
	s.mu.Lock()
	s.next++
	item.ID = "failover-drill-" + itoa(s.next)
	s.runs = append(s.runs, item)
	if len(s.runs) > 5000 {
		s.runs = s.runs[len(s.runs)-5000:]
	}
	s.mu.Unlock()
	return item, nil
}

func (s *RegionalFailoverDrillStore) List(limit int) []RegionalFailoverDrillRun {
	s.mu.RLock()
	out := append([]RegionalFailoverDrillRun{}, s.runs...)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CompletedAt.After(out[j].CompletedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *RegionalFailoverDrillStore) Scorecards(windowHours int) []RegionalFailoverScorecard {
	if windowHours <= 0 {
		windowHours = 24 * 30
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	groups := map[string][]RegionalFailoverDrillRun{}
	for _, run := range s.List(0) {
		if run.CompletedAt.Before(cutoff) {
			continue
		}
		groups[run.Region] = append(groups[run.Region], run)
	}
	cards := make([]RegionalFailoverScorecard, 0, len(groups))
	for region, runs := range groups {
		if len(runs) == 0 {
			continue
		}
		var pass int
		var sum int64
		var worst int64
		latest := runs[0].CompletedAt
		for i, run := range runs {
			if run.Pass {
				pass++
			}
			sum += run.RecoveryTimeMs
			if i == 0 || run.RecoveryTimeMs > worst {
				worst = run.RecoveryTimeMs
			}
			if run.CompletedAt.After(latest) {
				latest = run.CompletedAt
			}
		}
		cards = append(cards, RegionalFailoverScorecard{
			Region:            region,
			WindowHours:       windowHours,
			DrillCount:        len(runs),
			PassCount:         pass,
			PassRate:          float64(pass) / float64(len(runs)),
			AverageRecoveryMs: sum / int64(len(runs)),
			WorstRecoveryMs:   worst,
			LatestCompletedAt: latest,
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Region < cards[j].Region })
	return cards
}
