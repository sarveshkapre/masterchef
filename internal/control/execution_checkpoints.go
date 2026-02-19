package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExecutionCheckpointInput struct {
	RunID      string            `json:"run_id,omitempty"`
	JobID      string            `json:"job_id,omitempty"`
	ConfigPath string            `json:"config_path,omitempty"`
	StepID     string            `json:"step_id,omitempty"`
	StepOrder  int               `json:"step_order,omitempty"`
	Status     string            `json:"status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ExecutionCheckpoint struct {
	ID         string            `json:"id"`
	RunID      string            `json:"run_id,omitempty"`
	JobID      string            `json:"job_id,omitempty"`
	ConfigPath string            `json:"config_path,omitempty"`
	StepID     string            `json:"step_id,omitempty"`
	StepOrder  int               `json:"step_order,omitempty"`
	Status     string            `json:"status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	RecordedAt time.Time         `json:"recorded_at"`
}

type ExecutionCheckpointStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]*ExecutionCheckpoint
}

func NewExecutionCheckpointStore() *ExecutionCheckpointStore {
	return &ExecutionCheckpointStore{items: map[string]*ExecutionCheckpoint{}}
}

func (s *ExecutionCheckpointStore) Record(in ExecutionCheckpointInput) (ExecutionCheckpoint, error) {
	if strings.TrimSpace(in.ConfigPath) == "" {
		return ExecutionCheckpoint{}, errors.New("config_path is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &ExecutionCheckpoint{
		ID:         "checkpoint-" + itoa(s.nextID),
		RunID:      strings.TrimSpace(in.RunID),
		JobID:      strings.TrimSpace(in.JobID),
		ConfigPath: strings.TrimSpace(in.ConfigPath),
		StepID:     strings.TrimSpace(in.StepID),
		StepOrder:  in.StepOrder,
		Status:     strings.TrimSpace(in.Status),
		Metadata:   cloneStringMap(in.Metadata),
		RecordedAt: now,
	}
	s.items[item.ID] = item
	return cloneExecutionCheckpoint(*item), nil
}

func (s *ExecutionCheckpointStore) Get(id string) (ExecutionCheckpoint, bool) {
	s.mu.RLock()
	item, ok := s.items[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ExecutionCheckpoint{}, false
	}
	return cloneExecutionCheckpoint(*item), true
}

func (s *ExecutionCheckpointStore) List(runID, jobID string, limit int) []ExecutionCheckpoint {
	runID = strings.TrimSpace(runID)
	jobID = strings.TrimSpace(jobID)
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]ExecutionCheckpoint, 0, len(s.items))
	for _, item := range s.items {
		if runID != "" && item.RunID != runID {
			continue
		}
		if jobID != "" && item.JobID != jobID {
			continue
		}
		out = append(out, cloneExecutionCheckpoint(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.After(out[j].RecordedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneExecutionCheckpoint(in ExecutionCheckpoint) ExecutionCheckpoint {
	out := in
	out.Metadata = cloneStringMap(in.Metadata)
	return out
}
