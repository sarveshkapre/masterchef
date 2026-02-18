package control

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type Schedule struct {
	ID            string        `json:"id"`
	ConfigPath    string        `json:"config_path"`
	Priority      string        `json:"priority"`
	ExecutionCost int           `json:"execution_cost"`
	Host          string        `json:"host,omitempty"`
	Cluster       string        `json:"cluster,omitempty"`
	Environment   string        `json:"environment,omitempty"`
	Interval      time.Duration `json:"interval"`
	Jitter        time.Duration `json:"jitter"`
	Enabled       bool          `json:"enabled"`
	CreatedAt     time.Time     `json:"created_at"`
	LastRunAt     time.Time     `json:"last_run_at,omitempty"`
	NextRunAt     time.Time     `json:"next_run_at,omitempty"`
}

type Scheduler struct {
	mu               sync.RWMutex
	queue            *Queue
	maint            *MaintenanceStore
	schedules        map[string]*Schedule
	cancel           map[string]context.CancelFunc
	nextID           int64
	maxBacklog       int
	maxExecutionCost int
	hostHealth       map[string]bool
}

func NewScheduler(q *Queue) *Scheduler {
	return &Scheduler{
		queue:            q,
		maint:            NewMaintenanceStore(),
		schedules:        map[string]*Schedule{},
		cancel:           map[string]context.CancelFunc{},
		maxBacklog:       100,
		maxExecutionCost: 10,
		hostHealth:       map[string]bool{},
	}
}

func (s *Scheduler) Create(configPath string, interval, jitter time.Duration) *Schedule {
	return s.CreateWithPriority(configPath, interval, jitter, "normal")
}

func (s *Scheduler) CreateWithPriority(configPath string, interval, jitter time.Duration, priority string) *Schedule {
	return s.CreateWithOptions(ScheduleOptions{
		ConfigPath: configPath,
		Interval:   interval,
		Jitter:     jitter,
		Priority:   priority,
	})
}

type ScheduleOptions struct {
	ConfigPath    string
	Priority      string
	ExecutionCost int
	Host          string
	Cluster       string
	Environment   string
	Interval      time.Duration
	Jitter        time.Duration
}

func (s *Scheduler) CreateWithOptions(opts ScheduleOptions) *Schedule {
	interval := opts.Interval
	jitter := opts.Jitter
	if interval <= 0 {
		interval = time.Minute
	}
	if jitter < 0 {
		jitter = 0
	}
	cost := normalizeExecutionCost(opts.ExecutionCost)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := "sched-" + itoa(s.nextID)
	now := time.Now().UTC()
	sc := &Schedule{
		ID:            id,
		ConfigPath:    opts.ConfigPath,
		Priority:      normalizePriority(opts.Priority),
		ExecutionCost: cost,
		Host:          opts.Host,
		Cluster:       opts.Cluster,
		Environment:   opts.Environment,
		Interval:      interval,
		Jitter:        jitter,
		Enabled:       true,
		CreatedAt:     now,
		NextRunAt:     now.Add(interval),
	}
	s.schedules[id] = sc
	s.startLocked(sc)
	return cloneSchedule(sc)
}

func (s *Scheduler) List() []Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Schedule, 0, len(s.schedules))
	for _, sc := range s.schedules {
		out = append(out, *cloneSchedule(sc))
	}
	return out
}

func (s *Scheduler) Disable(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.schedules[id]
	if !ok {
		return false
	}
	sc.Enabled = false
	if cancel, ok := s.cancel[id]; ok {
		cancel()
		delete(s.cancel, id)
	}
	return true
}

func (s *Scheduler) Enable(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.schedules[id]
	if !ok {
		return false
	}
	if sc.Enabled {
		return true
	}
	sc.Enabled = true
	s.startLocked(sc)
	return true
}

func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancel))
	for id, cancel := range s.cancel {
		cancels = append(cancels, cancel)
		delete(s.cancel, id)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Scheduler) startLocked(sc *Schedule) {
	if !sc.Enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel[sc.ID] = cancel

	go func(scheduleID string) {
		for {
			wait := sc.Interval + randomJitter(sc.Jitter)
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if s.allowDispatch(sc) {
					_, _ = s.queue.Enqueue(sc.ConfigPath, "", false, sc.Priority)
				}
				s.mu.Lock()
				if cur, ok := s.schedules[scheduleID]; ok {
					now := time.Now().UTC()
					cur.LastRunAt = now
					cur.NextRunAt = now.Add(cur.Interval)
				}
				s.mu.Unlock()
			}
		}
	}(sc.ID)
}

func (s *Scheduler) SetMaintenance(kind, name string, enabled bool, reason string) (MaintenanceTarget, error) {
	return s.maint.Set(kind, name, enabled, reason)
}

func (s *Scheduler) MaintenanceStatus() []MaintenanceTarget {
	return s.maint.List()
}

func (s *Scheduler) skipForMaintenance(sc *Schedule) bool {
	if sc == nil {
		return false
	}
	if s.maint.IsActive("host", sc.Host) {
		return true
	}
	if s.maint.IsActive("cluster", sc.Cluster) {
		return true
	}
	if s.maint.IsActive("environment", sc.Environment) {
		return true
	}
	return false
}

type SchedulerCapacityStatus struct {
	MaxBacklog       int             `json:"max_backlog"`
	MaxExecutionCost int             `json:"max_execution_cost"`
	HostHealth       map[string]bool `json:"host_health"`
}

func (s *Scheduler) SetCapacity(maxBacklog, maxExecutionCost int) SchedulerCapacityStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxBacklog > 0 {
		s.maxBacklog = maxBacklog
	}
	if maxExecutionCost > 0 {
		s.maxExecutionCost = maxExecutionCost
	}
	return s.capacityStatusLocked()
}

func (s *Scheduler) SetHostHealth(host string, healthy bool) SchedulerCapacityStatus {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return s.CapacityStatus()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostHealth[host] = healthy
	return s.capacityStatusLocked()
}

func (s *Scheduler) CapacityStatus() SchedulerCapacityStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capacityStatusLocked()
}

func (s *Scheduler) capacityStatusLocked() SchedulerCapacityStatus {
	health := make(map[string]bool, len(s.hostHealth))
	for host, healthy := range s.hostHealth {
		health[host] = healthy
	}
	return SchedulerCapacityStatus{
		MaxBacklog:       s.maxBacklog,
		MaxExecutionCost: s.maxExecutionCost,
		HostHealth:       health,
	}
}

func (s *Scheduler) allowDispatch(sc *Schedule) bool {
	if sc == nil {
		return false
	}
	if s.skipForMaintenance(sc) {
		return false
	}

	s.mu.RLock()
	maxBacklog := s.maxBacklog
	maxExecutionCost := s.maxExecutionCost
	healthy, hasHealth := s.hostHealth[strings.ToLower(strings.TrimSpace(sc.Host))]
	s.mu.RUnlock()

	if hasHealth && !healthy {
		return false
	}
	if sc.ExecutionCost > maxExecutionCost {
		return false
	}

	queueState := s.queue.ControlStatus()
	if queueState.Pending >= maxBacklog {
		return false
	}
	if sc.Priority == "low" && maxBacklog > 1 && queueState.Pending >= (maxBacklog/2) {
		return false
	}
	return true
}

func normalizeExecutionCost(cost int) int {
	if cost <= 0 {
		return 1
	}
	if cost > 1_000 {
		return 1_000
	}
	return cost
}

func randomJitter(j time.Duration) time.Duration {
	if j <= 0 {
		return 0
	}
	n := rand.Int63n(int64(j) + 1)
	return time.Duration(n)
}

func cloneSchedule(s *Schedule) *Schedule {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}
