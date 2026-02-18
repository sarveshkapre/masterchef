package control

import (
	"context"
	"errors"
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
	Status         JobStatus `json:"status"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	EndedAt        time.Time `json:"ended_at,omitempty"`
}

type Executor interface {
	ApplyPath(configPath string) error
}

type Queue struct {
	mu             sync.RWMutex
	nextID         int64
	jobs           map[string]*Job
	byIdempotency  map[string]string
	pending        chan string
	workerShutdown chan struct{}
}

func NewQueue(buffer int) *Queue {
	if buffer <= 0 {
		buffer = 128
	}
	return &Queue{
		jobs:           map[string]*Job{},
		byIdempotency:  map[string]string{},
		pending:        make(chan string, buffer),
		workerShutdown: make(chan struct{}),
	}
}

func (q *Queue) Enqueue(configPath, key string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	if key != "" {
		if existingID, ok := q.byIdempotency[key]; ok {
			return q.clone(q.jobs[existingID])
		}
	}

	q.nextID++
	id := "job-" + time.Now().UTC().Format("20060102T150405") + "-" + itoa(q.nextID)
	j := &Job{
		ID:             id,
		IdempotencyKey: key,
		ConfigPath:     configPath,
		Status:         JobPending,
		CreatedAt:      time.Now().UTC(),
	}
	q.jobs[id] = j
	if key != "" {
		q.byIdempotency[key] = id
	}
	q.pending <- id
	return q.clone(j)
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
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	if j.Status == JobSucceeded || j.Status == JobFailed {
		return errors.New("job already finished")
	}
	j.Status = JobCanceled
	j.EndedAt = time.Now().UTC()
	return nil
}

func (q *Queue) StartWorker(ctx context.Context, exec Executor) {
	go func() {
		defer close(q.workerShutdown)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-q.pending:
				q.runOne(id, exec)
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
	q.mu.Unlock()

	err := exec.ApplyPath(j.ConfigPath)

	q.mu.Lock()
	defer q.mu.Unlock()
	j = q.jobs[id]
	if j.Status == JobCanceled {
		return
	}
	if err != nil {
		j.Status = JobFailed
		j.Error = err.Error()
	} else {
		j.Status = JobSucceeded
	}
	j.EndedAt = time.Now().UTC()
}

func (q *Queue) clone(j *Job) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	return &cp
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
