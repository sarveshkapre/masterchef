package control

import (
	"errors"
	"sync"
	"time"
)

type DriftSLOPolicy struct {
	TargetPercent      float64   `json:"target_percent"`
	WindowHours        int       `json:"window_hours"`
	MinSamples         int       `json:"min_samples"`
	AutoCreateIncident bool      `json:"auto_create_incident"`
	IncidentHook       string    `json:"incident_hook,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DriftSLOEvaluationInput struct {
	WindowHours        int  `json:"window_hours"`
	Samples            int  `json:"samples"`
	Changed            int  `json:"changed"`
	Suppressed         int  `json:"suppressed"`
	Allowlisted        int  `json:"allowlisted"`
	FailedRuns         int  `json:"failed_runs"`
	IncludeSuppressed  bool `json:"include_suppressed"`
	IncludeAllowlisted bool `json:"include_allowlisted"`
}

type DriftSLOEvaluation struct {
	ID                  string    `json:"id"`
	WindowHours         int       `json:"window_hours"`
	Samples             int       `json:"samples"`
	Changed             int       `json:"changed"`
	Suppressed          int       `json:"suppressed"`
	Allowlisted         int       `json:"allowlisted"`
	FailedRuns          int       `json:"failed_runs"`
	DriftRatePercent    float64   `json:"drift_rate_percent"`
	CompliancePercent   float64   `json:"compliance_percent"`
	TargetPercent       float64   `json:"target_percent"`
	Breached            bool      `json:"breached"`
	Status              string    `json:"status"` // healthy|breached|insufficient_samples
	IncidentRecommended bool      `json:"incident_recommended"`
	IncidentHook        string    `json:"incident_hook,omitempty"`
	EvaluatedAt         time.Time `json:"evaluated_at"`
}

type DriftSLOStore struct {
	mu      sync.RWMutex
	nextID  int64
	limit   int
	policy  DriftSLOPolicy
	history []DriftSLOEvaluation
}

func NewDriftSLOStore(limit int) *DriftSLOStore {
	if limit <= 0 {
		limit = 2000
	}
	return &DriftSLOStore{
		limit: limit,
		policy: DriftSLOPolicy{
			TargetPercent:      99.0,
			WindowHours:        24,
			MinSamples:         50,
			AutoCreateIncident: true,
			IncidentHook:       "event://incident.create.drift_slo",
			UpdatedAt:          time.Now().UTC(),
		},
		history: make([]DriftSLOEvaluation, 0, limit),
	}
}

func (s *DriftSLOStore) Policy() DriftSLOPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *DriftSLOStore) SetPolicy(in DriftSLOPolicy) (DriftSLOPolicy, error) {
	if in.TargetPercent <= 0 || in.TargetPercent > 100 {
		return DriftSLOPolicy{}, errors.New("target_percent must be > 0 and <= 100")
	}
	if in.WindowHours <= 0 {
		return DriftSLOPolicy{}, errors.New("window_hours must be > 0")
	}
	if in.MinSamples < 0 {
		return DriftSLOPolicy{}, errors.New("min_samples must be >= 0")
	}
	policy := DriftSLOPolicy{
		TargetPercent:      in.TargetPercent,
		WindowHours:        in.WindowHours,
		MinSamples:         in.MinSamples,
		AutoCreateIncident: in.AutoCreateIncident,
		IncidentHook:       in.IncidentHook,
		UpdatedAt:          time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return policy, nil
}

func (s *DriftSLOStore) Evaluate(in DriftSLOEvaluationInput) DriftSLOEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	policy := s.policy
	window := in.WindowHours
	if window <= 0 {
		window = policy.WindowHours
	}
	samples := in.Samples
	if samples < 0 {
		samples = 0
	}
	changed := in.Changed
	if changed < 0 {
		changed = 0
	}
	if changed > samples {
		changed = samples
	}
	driftRate := 0.0
	if samples > 0 {
		driftRate = (float64(changed) / float64(samples)) * 100
	}
	compliance := 100.0 - driftRate
	if compliance < 0 {
		compliance = 0
	}
	status := "healthy"
	breached := false
	if samples < policy.MinSamples {
		status = "insufficient_samples"
	} else if compliance < policy.TargetPercent {
		status = "breached"
		breached = true
	}
	eval := DriftSLOEvaluation{
		ID:                  "drift-slo-eval-" + itoa(s.nextID),
		WindowHours:         window,
		Samples:             samples,
		Changed:             changed,
		Suppressed:          in.Suppressed,
		Allowlisted:         in.Allowlisted,
		FailedRuns:          in.FailedRuns,
		DriftRatePercent:    driftRate,
		CompliancePercent:   compliance,
		TargetPercent:       policy.TargetPercent,
		Breached:            breached,
		Status:              status,
		IncidentRecommended: breached && policy.AutoCreateIncident,
		IncidentHook:        policy.IncidentHook,
		EvaluatedAt:         time.Now().UTC(),
	}
	if len(s.history) >= s.limit {
		copy(s.history[0:], s.history[1:])
		s.history[len(s.history)-1] = eval
	} else {
		s.history = append(s.history, eval)
	}
	return eval
}

func (s *DriftSLOStore) List(limit int) []DriftSLOEvaluation {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]DriftSLOEvaluation, len(s.history))
	copy(out, s.history)
	s.mu.RUnlock()
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	// Return newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
