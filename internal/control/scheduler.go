package control

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type Schedule struct {
	ID         string        `json:"id"`
	ConfigPath string        `json:"config_path"`
	Interval   time.Duration `json:"interval"`
	Jitter     time.Duration `json:"jitter"`
	Enabled    bool          `json:"enabled"`
	CreatedAt  time.Time     `json:"created_at"`
	LastRunAt  time.Time     `json:"last_run_at,omitempty"`
	NextRunAt  time.Time     `json:"next_run_at,omitempty"`
}

type Scheduler struct {
	mu        sync.RWMutex
	queue     *Queue
	schedules map[string]*Schedule
	cancel    map[string]context.CancelFunc
	nextID    int64
}

func NewScheduler(q *Queue) *Scheduler {
	return &Scheduler{
		queue:     q,
		schedules: map[string]*Schedule{},
		cancel:    map[string]context.CancelFunc{},
	}
}

func (s *Scheduler) Create(configPath string, interval, jitter time.Duration) *Schedule {
	if interval <= 0 {
		interval = time.Minute
	}
	if jitter < 0 {
		jitter = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := "sched-" + itoa(s.nextID)
	now := time.Now().UTC()
	sc := &Schedule{
		ID:         id,
		ConfigPath: configPath,
		Interval:   interval,
		Jitter:     jitter,
		Enabled:    true,
		CreatedAt:  now,
		NextRunAt:  now.Add(interval),
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
				_, _ = s.queue.Enqueue(sc.ConfigPath, "", false)
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
