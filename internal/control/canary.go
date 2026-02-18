package control

import (
	"context"
	"errors"
	"sync"
	"time"
)

type CanaryStatus string

const (
	CanaryUnknown   CanaryStatus = "unknown"
	CanaryHealthy   CanaryStatus = "healthy"
	CanaryUnhealthy CanaryStatus = "unhealthy"
)

type CanaryCheck struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	ConfigPath          string        `json:"config_path"`
	Priority            string        `json:"priority"`
	Interval            time.Duration `json:"interval"`
	Jitter              time.Duration `json:"jitter"`
	Enabled             bool          `json:"enabled"`
	FailureThreshold    int           `json:"failure_threshold"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	LastRunAt           time.Time     `json:"last_run_at,omitempty"`
	LastStatus          JobStatus     `json:"last_status,omitempty"`
	Health              CanaryStatus  `json:"health"`
	CreatedAt           time.Time     `json:"created_at"`
}

type CanaryCreate struct {
	Name             string
	ConfigPath       string
	Priority         string
	Interval         time.Duration
	Jitter           time.Duration
	FailureThreshold int
}

type CanaryStore struct {
	mu       sync.RWMutex
	nextID   int64
	queue    *Queue
	canaries map[string]*CanaryCheck
	cancels  map[string]context.CancelFunc
	jobRefs  map[string]string
}

func NewCanaryStore(queue *Queue) *CanaryStore {
	cs := &CanaryStore{
		queue:    queue,
		canaries: map[string]*CanaryCheck{},
		cancels:  map[string]context.CancelFunc{},
		jobRefs:  map[string]string{},
	}
	if queue != nil {
		queue.Subscribe(cs.onJob)
	}
	return cs
}

func (s *CanaryStore) Create(in CanaryCreate) (CanaryCheck, error) {
	if in.Name == "" {
		return CanaryCheck{}, errors.New("canary name is required")
	}
	if in.ConfigPath == "" {
		return CanaryCheck{}, errors.New("config_path is required")
	}
	if in.Interval <= 0 {
		in.Interval = 60 * time.Second
	}
	if in.Jitter < 0 {
		in.Jitter = 0
	}
	if in.FailureThreshold <= 0 {
		in.FailureThreshold = 3
	}

	s.mu.Lock()
	s.nextID++
	id := "canary-" + itoa(s.nextID)
	canary := &CanaryCheck{
		ID:               id,
		Name:             in.Name,
		ConfigPath:       in.ConfigPath,
		Priority:         normalizePriority(in.Priority),
		Interval:         in.Interval,
		Jitter:           in.Jitter,
		Enabled:          true,
		FailureThreshold: in.FailureThreshold,
		Health:           CanaryUnknown,
		CreatedAt:        time.Now().UTC(),
	}
	s.canaries[id] = canary
	s.mu.Unlock()

	s.start(id)
	return s.Get(id)
}

func (s *CanaryStore) start(id string) {
	s.mu.Lock()
	canary, ok := s.canaries[id]
	if !ok || !canary.Enabled {
		s.mu.Unlock()
		return
	}
	if cancel, ok := s.cancels[id]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancels[id] = cancel
	interval := canary.Interval
	jitter := canary.Jitter
	priority := canary.Priority
	configPath := canary.ConfigPath
	s.mu.Unlock()

	go func(canaryID string) {
		for {
			wait := interval + randomJitter(jitter)
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				job, err := s.queue.Enqueue(configPath, "", false, priority)
				if err != nil {
					s.markFailure(canaryID)
					continue
				}
				s.mu.Lock()
				if c, ok := s.canaries[canaryID]; ok {
					c.LastRunAt = time.Now().UTC()
					s.jobRefs[job.ID] = canaryID
				}
				s.mu.Unlock()
			}
		}
	}(id)
}

func (s *CanaryStore) markFailure(canaryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.canaries[canaryID]
	if !ok {
		return
	}
	c.ConsecutiveFailures++
	c.LastStatus = JobFailed
	if c.ConsecutiveFailures >= c.FailureThreshold {
		c.Health = CanaryUnhealthy
	}
}

func (s *CanaryStore) onJob(job Job) {
	if job.Status != JobSucceeded && job.Status != JobFailed {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	canaryID, ok := s.jobRefs[job.ID]
	if !ok {
		return
	}
	delete(s.jobRefs, job.ID)
	c, ok := s.canaries[canaryID]
	if !ok {
		return
	}
	c.LastStatus = job.Status
	if job.Status == JobSucceeded {
		c.ConsecutiveFailures = 0
		c.Health = CanaryHealthy
		return
	}
	c.ConsecutiveFailures++
	if c.ConsecutiveFailures >= c.FailureThreshold {
		c.Health = CanaryUnhealthy
	}
}

func (s *CanaryStore) List() []CanaryCheck {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CanaryCheck, 0, len(s.canaries))
	for _, c := range s.canaries {
		out = append(out, *cloneCanary(c))
	}
	return out
}

func (s *CanaryStore) Get(id string) (CanaryCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.canaries[id]
	if !ok {
		return CanaryCheck{}, errors.New("canary not found")
	}
	return *cloneCanary(c), nil
}

func (s *CanaryStore) SetEnabled(id string, enabled bool) (CanaryCheck, error) {
	s.mu.Lock()
	c, ok := s.canaries[id]
	if !ok {
		s.mu.Unlock()
		return CanaryCheck{}, errors.New("canary not found")
	}
	c.Enabled = enabled
	if !enabled {
		if cancel, ok := s.cancels[id]; ok {
			cancel()
			delete(s.cancels, id)
		}
	}
	s.mu.Unlock()

	if enabled {
		s.start(id)
	}
	return s.Get(id)
}

func (s *CanaryStore) HealthSummary() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := len(s.canaries)
	healthy := 0
	unhealthy := 0
	unknown := 0
	for _, c := range s.canaries {
		switch c.Health {
		case CanaryHealthy:
			healthy++
		case CanaryUnhealthy:
			unhealthy++
		default:
			unknown++
		}
	}
	status := "ok"
	if unhealthy > 0 {
		status = "degraded"
	}
	return map[string]any{
		"status":    status,
		"total":     total,
		"healthy":   healthy,
		"unhealthy": unhealthy,
		"unknown":   unknown,
	}
}

func (s *CanaryStore) Shutdown() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for id, cancel := range s.cancels {
		cancels = append(cancels, cancel)
		delete(s.cancels, id)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func cloneCanary(c *CanaryCheck) *CanaryCheck {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}
