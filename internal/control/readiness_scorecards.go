package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ReadinessScorecardInput struct {
	Environment string              `json:"environment"`
	Service     string              `json:"service"`
	Owner       string              `json:"owner,omitempty"`
	Signals     ReadinessSignals    `json:"signals"`
	Thresholds  ReadinessThresholds `json:"thresholds,omitempty"`
}

type ReadinessScorecard struct {
	ID          string          `json:"id"`
	Environment string          `json:"environment"`
	Service     string          `json:"service"`
	Owner       string          `json:"owner,omitempty"`
	Grade       string          `json:"grade"`
	Report      ReadinessReport `json:"report"`
	CreatedAt   time.Time       `json:"created_at"`
}

type ReadinessScorecardStore struct {
	mu      sync.RWMutex
	nextID  int64
	items   map[string]*ReadinessScorecard
	ordered []string
}

func NewReadinessScorecardStore() *ReadinessScorecardStore {
	return &ReadinessScorecardStore{
		items:   map[string]*ReadinessScorecard{},
		ordered: make([]string, 0),
	}
}

func (s *ReadinessScorecardStore) Create(in ReadinessScorecardInput) (ReadinessScorecard, error) {
	environment := strings.TrimSpace(in.Environment)
	service := strings.TrimSpace(in.Service)
	if environment == "" || service == "" {
		return ReadinessScorecard{}, errors.New("environment and service are required")
	}
	report := EvaluateReadiness(in.Signals, in.Thresholds)
	grade := readinessGrade(report)
	item := ReadinessScorecard{
		Environment: environment,
		Service:     service,
		Owner:       strings.TrimSpace(in.Owner),
		Grade:       grade,
		Report:      report,
		CreatedAt:   report.GeneratedAt,
	}
	s.mu.Lock()
	s.nextID++
	item.ID = "readiness-scorecard-" + itoa(s.nextID)
	s.items[item.ID] = &item
	s.ordered = append(s.ordered, item.ID)
	s.mu.Unlock()
	return cloneReadinessScorecard(item), nil
}

func (s *ReadinessScorecardStore) List(environment, service string, limit int) []ReadinessScorecard {
	environment = strings.TrimSpace(environment)
	service = strings.TrimSpace(service)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]ReadinessScorecard, 0, len(s.ordered))
	for i := len(s.ordered) - 1; i >= 0; i-- {
		item := s.items[s.ordered[i]]
		if item == nil {
			continue
		}
		if environment != "" && !strings.EqualFold(item.Environment, environment) {
			continue
		}
		if service != "" && !strings.EqualFold(item.Service, service) {
			continue
		}
		out = append(out, cloneReadinessScorecard(*item))
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ReadinessScorecardStore) Get(id string) (ReadinessScorecard, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ReadinessScorecard{}, errors.New("scorecard id is required")
	}
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return ReadinessScorecard{}, errors.New("scorecard not found")
	}
	return cloneReadinessScorecard(*item), nil
}

func readinessGrade(report ReadinessReport) string {
	score := report.AggregateScore
	if !report.Pass {
		if score >= 0.90 {
			return "B"
		}
		if score >= 0.75 {
			return "C"
		}
		return "D"
	}
	if score >= 0.98 {
		return "A+"
	}
	if score >= 0.93 {
		return "A"
	}
	if score >= 0.88 {
		return "B"
	}
	return "C"
}

func cloneReadinessScorecard(in ReadinessScorecard) ReadinessScorecard {
	in.Report.Blockers = cloneStringSlice(in.Report.Blockers)
	return in
}
