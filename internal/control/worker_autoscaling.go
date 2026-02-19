package control

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type WorkerAutoscalingPolicy struct {
	Enabled             bool      `json:"enabled"`
	MinWorkers          int       `json:"min_workers"`
	MaxWorkers          int       `json:"max_workers"`
	QueueDepthPerWorker int       `json:"queue_depth_per_worker"`
	TargetP95LatencyMs  int64     `json:"target_p95_latency_ms"`
	ScaleUpStep         int       `json:"scale_up_step"`
	ScaleDownStep       int       `json:"scale_down_step"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type WorkerAutoscalingInput struct {
	QueueDepth     int   `json:"queue_depth"`
	CurrentWorkers int   `json:"current_workers"`
	P95LatencyMs   int64 `json:"p95_latency_ms"`
}

type WorkerAutoscalingDecision struct {
	CurrentWorkers int    `json:"current_workers"`
	Recommended    int    `json:"recommended_workers"`
	Delta          int    `json:"delta"`
	Reason         string `json:"reason"`
	QueueDepth     int    `json:"queue_depth"`
	P95LatencyMs   int64  `json:"p95_latency_ms"`
}

type WorkerAutoscalingStore struct {
	mu     sync.RWMutex
	policy WorkerAutoscalingPolicy
}

func NewWorkerAutoscalingStore() *WorkerAutoscalingStore {
	return &WorkerAutoscalingStore{policy: WorkerAutoscalingPolicy{
		Enabled:             true,
		MinWorkers:          2,
		MaxWorkers:          200,
		QueueDepthPerWorker: 25,
		TargetP95LatencyMs:  120000,
		ScaleUpStep:         5,
		ScaleDownStep:       2,
		UpdatedAt:           time.Now().UTC(),
	}}
}

func (s *WorkerAutoscalingStore) Policy() WorkerAutoscalingPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *WorkerAutoscalingStore) SetPolicy(in WorkerAutoscalingPolicy) (WorkerAutoscalingPolicy, error) {
	if in.MinWorkers <= 0 || in.MaxWorkers <= 0 || in.MinWorkers > in.MaxWorkers {
		return WorkerAutoscalingPolicy{}, errors.New("invalid min/max worker bounds")
	}
	if in.QueueDepthPerWorker <= 0 {
		return WorkerAutoscalingPolicy{}, errors.New("queue_depth_per_worker must be > 0")
	}
	if in.ScaleUpStep <= 0 || in.ScaleDownStep <= 0 {
		return WorkerAutoscalingPolicy{}, errors.New("scale steps must be > 0")
	}
	if in.TargetP95LatencyMs <= 0 {
		in.TargetP95LatencyMs = 120000
	}
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = in
	s.mu.Unlock()
	return in, nil
}

func (s *WorkerAutoscalingStore) Recommend(in WorkerAutoscalingInput) WorkerAutoscalingDecision {
	policy := s.Policy()
	current := in.CurrentWorkers
	if current <= 0 {
		current = policy.MinWorkers
	}
	if !policy.Enabled {
		return WorkerAutoscalingDecision{
			CurrentWorkers: current,
			Recommended:    current,
			Delta:          0,
			Reason:         "autoscaling disabled",
			QueueDepth:     in.QueueDepth,
			P95LatencyMs:   in.P95LatencyMs,
		}
	}
	loadRecommended := policy.MinWorkers
	if in.QueueDepth > 0 {
		loadRecommended = (in.QueueDepth + policy.QueueDepthPerWorker - 1) / policy.QueueDepthPerWorker
	}
	if loadRecommended < policy.MinWorkers {
		loadRecommended = policy.MinWorkers
	}
	if loadRecommended > policy.MaxWorkers {
		loadRecommended = policy.MaxWorkers
	}

	recommended := current
	reason := "within autoscaling targets"
	if loadRecommended > current || in.P95LatencyMs > policy.TargetP95LatencyMs {
		recommended = current + policy.ScaleUpStep
		if loadRecommended > recommended {
			recommended = loadRecommended
		}
		reason = "scale up due to queue pressure or latency"
	} else if loadRecommended < current && in.P95LatencyMs < policy.TargetP95LatencyMs {
		recommended = current - policy.ScaleDownStep
		if loadRecommended > recommended {
			recommended = loadRecommended
		}
		reason = "scale down due to sustained low pressure"
	}
	if recommended < policy.MinWorkers {
		recommended = policy.MinWorkers
	}
	if recommended > policy.MaxWorkers {
		recommended = policy.MaxWorkers
	}
	return WorkerAutoscalingDecision{
		CurrentWorkers: current,
		Recommended:    recommended,
		Delta:          recommended - current,
		Reason:         strings.TrimSpace(reason),
		QueueDepth:     in.QueueDepth,
		P95LatencyMs:   in.P95LatencyMs,
	}
}
