package control

import (
	"errors"
	"sync"
	"time"
)

type QueueBacklogSLOPolicy struct {
	Threshold         int       `json:"threshold"`
	WarningPercent    int       `json:"warning_percent"`
	RecoveryPercent   int       `json:"recovery_percent"`
	ProjectionSeconds int       `json:"projection_seconds"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type QueueBacklogSLOStatus struct {
	At                  time.Time `json:"at"`
	Pending             int       `json:"pending"`
	Running             int       `json:"running"`
	PendingHigh         int       `json:"pending_high"`
	PendingNormal       int       `json:"pending_normal"`
	PendingLow          int       `json:"pending_low"`
	Threshold           int       `json:"threshold"`
	WarningThreshold    int       `json:"warning_threshold"`
	RecoveryThreshold   int       `json:"recovery_threshold"`
	GrowthPerSecMilli   int64     `json:"growth_per_sec_milli"`
	PredictedPending    int       `json:"predicted_pending"`
	PredictiveSaturated bool      `json:"predictive_saturated"`
	State               string    `json:"state"` // normal|warning|saturated
}

type QueueBacklogSLOStore struct {
	mu      sync.RWMutex
	limit   int
	policy  QueueBacklogSLOPolicy
	history []QueueBacklogSLOStatus
}

func NewQueueBacklogSLOStore(threshold, limit int) *QueueBacklogSLOStore {
	if threshold <= 0 {
		threshold = 100
	}
	if limit <= 0 {
		limit = 5000
	}
	return &QueueBacklogSLOStore{
		limit: limit,
		policy: QueueBacklogSLOPolicy{
			Threshold:         threshold,
			WarningPercent:    70,
			RecoveryPercent:   50,
			ProjectionSeconds: 300,
			UpdatedAt:         time.Now().UTC(),
		},
		history: make([]QueueBacklogSLOStatus, 0, limit),
	}
}

func (s *QueueBacklogSLOStore) Policy() QueueBacklogSLOPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *QueueBacklogSLOStore) SetPolicy(in QueueBacklogSLOPolicy) (QueueBacklogSLOPolicy, error) {
	if in.Threshold <= 0 {
		return QueueBacklogSLOPolicy{}, errors.New("threshold must be > 0")
	}
	if in.WarningPercent <= 0 || in.WarningPercent >= 100 {
		return QueueBacklogSLOPolicy{}, errors.New("warning_percent must be > 0 and < 100")
	}
	if in.RecoveryPercent <= 0 || in.RecoveryPercent >= in.WarningPercent {
		return QueueBacklogSLOPolicy{}, errors.New("recovery_percent must be > 0 and < warning_percent")
	}
	if in.ProjectionSeconds < 30 || in.ProjectionSeconds > 3600 {
		return QueueBacklogSLOPolicy{}, errors.New("projection_seconds must be between 30 and 3600")
	}
	item := QueueBacklogSLOPolicy{
		Threshold:         in.Threshold,
		WarningPercent:    in.WarningPercent,
		RecoveryPercent:   in.RecoveryPercent,
		ProjectionSeconds: in.ProjectionSeconds,
		UpdatedAt:         time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = item
	s.mu.Unlock()
	return item, nil
}

func (s *QueueBacklogSLOStore) Record(in QueueBacklogSLOStatus) QueueBacklogSLOStatus {
	item := in
	if item.At.IsZero() {
		item.At = time.Now().UTC()
	} else {
		item.At = item.At.UTC()
	}
	if item.State == "" {
		item.State = "normal"
	}
	s.mu.Lock()
	if len(s.history) >= s.limit {
		copy(s.history[0:], s.history[1:])
		s.history[len(s.history)-1] = item
	} else {
		s.history = append(s.history, item)
	}
	s.mu.Unlock()
	return item
}

func (s *QueueBacklogSLOStore) Latest() (QueueBacklogSLOStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.history) == 0 {
		return QueueBacklogSLOStatus{}, false
	}
	return s.history[len(s.history)-1], true
}

func (s *QueueBacklogSLOStore) History(limit int) []QueueBacklogSLOStatus {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := append([]QueueBacklogSLOStatus{}, s.history...)
	s.mu.RUnlock()
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
