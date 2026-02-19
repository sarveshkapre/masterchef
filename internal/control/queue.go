package control

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

type Job struct {
	ID             string    `json:"id"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	ConfigPath     string    `json:"config_path"`
	Priority       string    `json:"priority"` // high, normal, low
	Status         JobStatus `json:"status"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	EndedAt        time.Time `json:"ended_at,omitempty"`
}

type WorkerLifecyclePolicy struct {
	Mode             string    `json:"mode"` // persistent, stateless
	MaxJobsPerWorker int       `json:"max_jobs_per_worker,omitempty"`
	RestartDelayMS   int       `json:"restart_delay_ms,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type WorkerLifecycleInput struct {
	Mode             string `json:"mode,omitempty"`
	MaxJobsPerWorker int    `json:"max_jobs_per_worker,omitempty"`
	RestartDelayMS   int    `json:"restart_delay_ms,omitempty"`
}

type WorkerLifecycleStatus struct {
	Policy           WorkerLifecyclePolicy `json:"policy"`
	Generation       int64                 `json:"generation"`
	Recycles         int64                 `json:"recycles"`
	CurrentQueueLoad QueueControlStatus    `json:"current_queue_load"`
}

type Executor interface {
	ApplyPath(configPath string) error
}

type Queue struct {
	mu              sync.RWMutex
	nextID          int64
	jobs            map[string]*Job
	byIdempotency   map[string]string
	pendingHigh     chan string
	pendingNormal   chan string
	pendingLow      chan string
	workerShutdown  chan struct{}
	subscribers     []func(Job)
	emergencyStop   bool
	emergencySince  time.Time
	emergencyReason string
	freezeUntil     time.Time
	freezeReason    string
	paused          bool
	running         int
	rrIndex         int
	workerPolicy    WorkerLifecyclePolicy
	generation      int64
	recycles        int64
}

func NewQueue(buffer int) *Queue {
	if buffer <= 0 {
		buffer = 128
	}
	return &Queue{
		jobs:           map[string]*Job{},
		byIdempotency:  map[string]string{},
		pendingHigh:    make(chan string, buffer),
		pendingNormal:  make(chan string, buffer),
		pendingLow:     make(chan string, buffer),
		workerShutdown: make(chan struct{}),
		workerPolicy: WorkerLifecyclePolicy{
			Mode:             "persistent",
			MaxJobsPerWorker: 0,
			RestartDelayMS:   0,
			UpdatedAt:        time.Now().UTC(),
		},
	}
}

func (q *Queue) Subscribe(fn func(Job)) {
	if fn == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.subscribers = append(q.subscribers, fn)
}

func (q *Queue) Enqueue(configPath, key string, force bool, priority string) (*Job, error) {
	q.mu.Lock()
	if key != "" {
		if existingID, ok := q.byIdempotency[key]; ok {
			cp := q.clone(q.jobs[existingID])
			q.mu.Unlock()
			return cp, nil
		}
	}
	if q.emergencyStop && !force {
		q.mu.Unlock()
		return nil, errors.New("emergency stop active; new applies are halted")
	}
	if !force && !q.freezeUntil.IsZero() && time.Now().UTC().Before(q.freezeUntil) {
		until := q.freezeUntil.Format(time.RFC3339)
		reason := strings.TrimSpace(q.freezeReason)
		q.mu.Unlock()
		if reason != "" {
			return nil, errors.New("change freeze active until " + until + ": " + reason)
		}
		return nil, errors.New("change freeze active until " + until)
	}

	p := normalizePriority(priority)
	q.nextID++
	id := "job-" + time.Now().UTC().Format("20060102T150405") + "-" + itoa(q.nextID)
	j := &Job{
		ID:             id,
		IdempotencyKey: key,
		ConfigPath:     configPath,
		Priority:       p,
		Status:         JobPending,
		CreatedAt:      time.Now().UTC(),
	}
	q.jobs[id] = j
	if key != "" {
		q.byIdempotency[key] = id
	}
	if err := q.pushPending(id, p); err != nil {
		delete(q.jobs, id)
		delete(q.byIdempotency, key)
		q.mu.Unlock()
		return nil, err
	}
	cp := q.clone(j)
	q.mu.Unlock()
	q.publish(*cp)
	return cp, nil
}

func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	j, ok := q.jobs[id]
	if !ok {
		return nil, false
	}
	return q.clone(j), true
}

func (q *Queue) List() []Job {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		out = append(out, *q.clone(j))
	}
	return out
}

func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	j, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return errors.New("job not found")
	}
	if j.Status == JobSucceeded || j.Status == JobFailed {
		q.mu.Unlock()
		return errors.New("job already finished")
	}
	j.Status = JobCanceled
	j.EndedAt = time.Now().UTC()
	cp := *j
	q.mu.Unlock()
	q.publish(cp)
	return nil
}

func (q *Queue) StartWorker(ctx context.Context, exec Executor) {
	go func() {
		defer close(q.workerShutdown)
		q.mu.Lock()
		q.generation = 1
		q.mu.Unlock()
		for {
			policy := q.WorkerLifecyclePolicy()
			jobsProcessed, done := q.runWorkerGeneration(ctx, exec, policy)
			if done {
				return
			}
			if jobsProcessed > 0 {
				q.mu.Lock()
				q.recycles++
				q.generation++
				q.mu.Unlock()
			}
			delay := time.Duration(policy.RestartDelayMS) * time.Millisecond
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
		}
	}()
}

func (q *Queue) Wait() {
	<-q.workerShutdown
}

func (q *Queue) runOne(id string, exec Executor) {
	q.mu.Lock()
	j, ok := q.jobs[id]
	if !ok || j.Status == JobCanceled {
		q.mu.Unlock()
		return
	}
	j.Status = JobRunning
	j.StartedAt = time.Now().UTC()
	q.running++
	cp := *j
	q.mu.Unlock()
	q.publish(cp)

	err := exec.ApplyPath(j.ConfigPath)

	q.mu.Lock()
	j = q.jobs[id]
	if j.Status == JobCanceled {
		if q.running > 0 {
			q.running--
		}
		q.mu.Unlock()
		return
	}
	if err != nil {
		j.Status = JobFailed
		j.Error = err.Error()
	} else {
		j.Status = JobSucceeded
	}
	j.EndedAt = time.Now().UTC()
	if q.running > 0 {
		q.running--
	}
	cp = *j
	q.mu.Unlock()
	q.publish(cp)
}

func (q *Queue) runWorkerGeneration(ctx context.Context, exec Executor, policy WorkerLifecyclePolicy) (int, bool) {
	maxJobs := normalizedMaxJobs(policy)
	processed := 0
	for {
		if q.IsPaused() {
			select {
			case <-ctx.Done():
				return processed, true
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		id, ok := q.nextPending(ctx)
		if !ok {
			return processed, true
		}
		q.runOne(id, exec)
		processed++
		if maxJobs > 0 && processed >= maxJobs {
			return processed, false
		}
	}
}

func (q *Queue) pushPending(id, priority string) error {
	class := normalizePriority(priority)
	var ch chan string
	switch class {
	case "high":
		ch = q.pendingHigh
	case "low":
		ch = q.pendingLow
	default:
		ch = q.pendingNormal
	}
	select {
	case ch <- id:
		return nil
	default:
		return errors.New("pending queue full for priority class: " + class)
	}
}

func (q *Queue) nextPending(ctx context.Context) (string, bool) {
	classes := []string{"high", "normal", "low"}

	// Fair polling by rotating start index across priority classes.
	for i := 0; i < len(classes); i++ {
		idx := (q.rrIndex + i) % len(classes)
		switch classes[idx] {
		case "high":
			select {
			case id := <-q.pendingHigh:
				q.rrIndex = (idx + 1) % len(classes)
				return id, true
			default:
			}
		case "normal":
			select {
			case id := <-q.pendingNormal:
				q.rrIndex = (idx + 1) % len(classes)
				return id, true
			default:
			}
		case "low":
			select {
			case id := <-q.pendingLow:
				q.rrIndex = (idx + 1) % len(classes)
				return id, true
			default:
			}
		}
	}

	select {
	case <-ctx.Done():
		return "", false
	case id := <-q.pendingHigh:
		return id, true
	case id := <-q.pendingNormal:
		return id, true
	case id := <-q.pendingLow:
		return id, true
	}
}

func (q *Queue) clone(j *Job) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	return &cp
}

func (q *Queue) publish(job Job) {
	q.mu.RLock()
	subs := make([]func(Job), len(q.subscribers))
	copy(subs, q.subscribers)
	q.mu.RUnlock()
	for _, fn := range subs {
		fn(job)
	}
}

type EmergencyStatus struct {
	Active bool      `json:"active"`
	Since  time.Time `json:"since,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

type FreezeStatus struct {
	Active bool      `json:"active"`
	Until  time.Time `json:"until,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

func (q *Queue) SetEmergencyStop(active bool, reason string) EmergencyStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.emergencyStop = active
	if active {
		if q.emergencySince.IsZero() {
			q.emergencySince = time.Now().UTC()
		}
		q.emergencyReason = reason
	} else {
		q.emergencySince = time.Time{}
		q.emergencyReason = ""
	}
	return EmergencyStatus{
		Active: q.emergencyStop,
		Since:  q.emergencySince,
		Reason: q.emergencyReason,
	}
}

func (q *Queue) EmergencyStatus() EmergencyStatus {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return EmergencyStatus{
		Active: q.emergencyStop,
		Since:  q.emergencySince,
		Reason: q.emergencyReason,
	}
}

func (q *Queue) SetFreezeUntil(until time.Time, reason string) FreezeStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	if until.IsZero() || !until.After(now) {
		q.freezeUntil = time.Time{}
		q.freezeReason = ""
		return FreezeStatus{}
	}
	q.freezeUntil = until.UTC()
	q.freezeReason = strings.TrimSpace(reason)
	return FreezeStatus{
		Active: true,
		Until:  q.freezeUntil,
		Reason: q.freezeReason,
	}
}

func (q *Queue) ClearFreeze() FreezeStatus {
	return q.SetFreezeUntil(time.Time{}, "")
}

func (q *Queue) FreezeStatus() FreezeStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.freezeUntil.IsZero() {
		return FreezeStatus{}
	}
	now := time.Now().UTC()
	if !now.Before(q.freezeUntil) {
		q.freezeUntil = time.Time{}
		q.freezeReason = ""
		return FreezeStatus{}
	}
	return FreezeStatus{
		Active: true,
		Until:  q.freezeUntil,
		Reason: q.freezeReason,
	}
}

type QueueControlStatus struct {
	Paused        bool `json:"paused"`
	Running       int  `json:"running"`
	Pending       int  `json:"pending"`
	PendingHigh   int  `json:"pending_high"`
	PendingNormal int  `json:"pending_normal"`
	PendingLow    int  `json:"pending_low"`
}

func (q *Queue) SetWorkerLifecyclePolicy(in WorkerLifecycleInput) WorkerLifecyclePolicy {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "persistent"
	}
	switch mode {
	case "persistent", "stateless":
	default:
		mode = "persistent"
	}
	maxJobs := in.MaxJobsPerWorker
	if mode == "stateless" && maxJobs <= 0 {
		maxJobs = 1
	}
	if maxJobs < 0 {
		maxJobs = 0
	}
	restart := in.RestartDelayMS
	if restart < 0 {
		restart = 0
	}
	policy := WorkerLifecyclePolicy{
		Mode:             mode,
		MaxJobsPerWorker: maxJobs,
		RestartDelayMS:   restart,
		UpdatedAt:        time.Now().UTC(),
	}
	q.mu.Lock()
	q.workerPolicy = policy
	q.mu.Unlock()
	return policy
}

func (q *Queue) WorkerLifecyclePolicy() WorkerLifecyclePolicy {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.workerPolicy
}

func (q *Queue) WorkerLifecycleStatus() WorkerLifecycleStatus {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return WorkerLifecycleStatus{
		Policy:           q.workerPolicy,
		Generation:       q.generation,
		Recycles:         q.recycles,
		CurrentQueueLoad: q.controlStatusLocked(),
	}
}

func (q *Queue) Pause() QueueControlStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = true
	return q.controlStatusLocked()
}

func (q *Queue) Resume() QueueControlStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = false
	return q.controlStatusLocked()
}

func (q *Queue) IsPaused() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.paused
}

func (q *Queue) ControlStatus() QueueControlStatus {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.controlStatusLocked()
}

func (q *Queue) controlStatusLocked() QueueControlStatus {
	high := len(q.pendingHigh)
	normal := len(q.pendingNormal)
	low := len(q.pendingLow)
	return QueueControlStatus{
		Paused:        q.paused,
		Running:       q.running,
		Pending:       high + normal + low,
		PendingHigh:   high,
		PendingNormal: normal,
		PendingLow:    low,
	}
}

func (q *Queue) SafeDrain(timeout time.Duration) (QueueControlStatus, error) {
	q.Pause()
	deadline := time.Now().Add(timeout)
	for {
		st := q.ControlStatus()
		if st.Running == 0 {
			return st, nil
		}
		if timeout > 0 && time.Now().After(deadline) {
			return st, errors.New("safe-drain timeout waiting for running jobs to complete")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (q *Queue) RecoverStuckJobs(maxAge time.Duration) []Job {
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}
	now := time.Now().UTC()
	q.mu.Lock()
	defer q.mu.Unlock()

	recovered := make([]Job, 0)
	for _, j := range q.jobs {
		if j.Status != JobRunning || j.StartedAt.IsZero() {
			continue
		}
		if now.Sub(j.StartedAt) < maxAge {
			continue
		}
		j.Status = JobFailed
		j.Error = "stale run lease recovered by control plane"
		j.EndedAt = now
		if q.running > 0 {
			q.running--
		}
		recovered = append(recovered, *q.clone(j))
	}
	go func(items []Job) {
		for _, j := range items {
			q.publish(j)
		}
	}(append([]Job{}, recovered...))
	return recovered
}

func normalizePriority(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "normal"
	}
}

func normalizedMaxJobs(policy WorkerLifecyclePolicy) int {
	if policy.Mode == "stateless" {
		if policy.MaxJobsPerWorker <= 0 {
			return 1
		}
		return policy.MaxJobsPerWorker
	}
	if policy.MaxJobsPerWorker <= 0 {
		return 0
	}
	return policy.MaxJobsPerWorker
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(b[i:])
}
