package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type StepSnapshotInput struct {
	RunID        string            `json:"run_id,omitempty"`
	JobID        string            `json:"job_id,omitempty"`
	StepID       string            `json:"step_id"`
	ResourceType string            `json:"resource_type,omitempty"`
	Host         string            `json:"host,omitempty"`
	Status       string            `json:"status"`
	StartedAt    string            `json:"started_at,omitempty"`
	EndedAt      string            `json:"ended_at,omitempty"`
	StdoutHash   string            `json:"stdout_hash,omitempty"`
	StderrHash   string            `json:"stderr_hash,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type StepSnapshot struct {
	SnapshotID   string            `json:"snapshot_id"`
	RunID        string            `json:"run_id,omitempty"`
	JobID        string            `json:"job_id,omitempty"`
	StepID       string            `json:"step_id"`
	ResourceType string            `json:"resource_type,omitempty"`
	Host         string            `json:"host,omitempty"`
	Status       string            `json:"status"`
	StartedAt    time.Time         `json:"started_at"`
	EndedAt      time.Time         `json:"ended_at,omitempty"`
	DurationMS   int64             `json:"duration_ms,omitempty"`
	StdoutHash   string            `json:"stdout_hash,omitempty"`
	StderrHash   string            `json:"stderr_hash,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RecordedAt   time.Time         `json:"recorded_at"`
}

type StepSnapshotQuery struct {
	RunID  string
	JobID  string
	StepID string
	Limit  int
}

type StepSnapshotStore struct {
	mu       sync.RWMutex
	nextID   int64
	items    []StepSnapshot
	byID     map[string]StepSnapshot
	maxItems int
}

func NewStepSnapshotStore(maxItems int) *StepSnapshotStore {
	if maxItems <= 0 {
		maxItems = 10_000
	}
	return &StepSnapshotStore{
		items:    make([]StepSnapshot, 0, maxItems),
		byID:     map[string]StepSnapshot{},
		maxItems: maxItems,
	}
}

func (s *StepSnapshotStore) Record(in StepSnapshotInput) (StepSnapshot, error) {
	stepID := strings.TrimSpace(in.StepID)
	status := normalizeSnapshotStatus(in.Status)
	if stepID == "" || status == "" {
		return StepSnapshot{}, errors.New("step_id and valid status are required")
	}
	started := parseSnapshotTime(in.StartedAt)
	ended := parseSnapshotTime(in.EndedAt)
	if started.IsZero() {
		started = time.Now().UTC()
	}
	if !ended.IsZero() && ended.Before(started) {
		return StepSnapshot{}, errors.New("ended_at cannot be before started_at")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := StepSnapshot{
		SnapshotID:   "step-snap-" + itoa(s.nextID),
		RunID:        strings.TrimSpace(in.RunID),
		JobID:        strings.TrimSpace(in.JobID),
		StepID:       stepID,
		ResourceType: strings.TrimSpace(in.ResourceType),
		Host:         strings.TrimSpace(in.Host),
		Status:       status,
		StartedAt:    started,
		EndedAt:      ended,
		DurationMS:   snapshotDurationMS(started, ended),
		StdoutHash:   strings.TrimSpace(in.StdoutHash),
		StderrHash:   strings.TrimSpace(in.StderrHash),
		Metadata:     cloneStringMap(in.Metadata),
		RecordedAt:   now,
	}
	s.items = append(s.items, item)
	s.byID[item.SnapshotID] = item
	if len(s.items) > s.maxItems {
		over := len(s.items) - s.maxItems
		for i := 0; i < over; i++ {
			delete(s.byID, s.items[i].SnapshotID)
		}
		s.items = append([]StepSnapshot{}, s.items[over:]...)
	}
	return item, nil
}

func (s *StepSnapshotStore) Get(id string) (StepSnapshot, bool) {
	s.mu.RLock()
	item, ok := s.byID[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return StepSnapshot{}, false
	}
	item.Metadata = cloneStringMap(item.Metadata)
	return item, true
}

func (s *StepSnapshotStore) List(q StepSnapshotQuery) []StepSnapshot {
	runID := strings.TrimSpace(q.RunID)
	jobID := strings.TrimSpace(q.JobID)
	stepID := strings.TrimSpace(q.StepID)
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]StepSnapshot, 0, minInt(limit, len(s.items)))
	for i := len(s.items) - 1; i >= 0; i-- {
		item := s.items[i]
		if runID != "" && item.RunID != runID {
			continue
		}
		if jobID != "" && item.JobID != jobID {
			continue
		}
		if stepID != "" && item.StepID != stepID {
			continue
		}
		item.Metadata = cloneStringMap(item.Metadata)
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.After(out[j].RecordedAt) })
	return out
}

func normalizeSnapshotStatus(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "pending", "running", "succeeded", "failed", "skipped":
		return strings.ToLower(strings.TrimSpace(in))
	default:
		return ""
	}
}

func parseSnapshotTime(in string) time.Time {
	trimmed := strings.TrimSpace(in)
	if trimmed == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func snapshotDurationMS(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Millisecond)
}
