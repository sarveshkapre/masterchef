package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExecutionLock struct {
	ID         string    `json:"id"`
	Key        string    `json:"key"`
	Holder     string    `json:"holder"`
	JobID      string    `json:"job_id,omitempty"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	Status     string    `json:"status"` // active|released|expired
}

type ExecutionLockAcquireInput struct {
	Key        string `json:"key"`
	Holder     string `json:"holder"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type ExecutionLockReleaseInput struct {
	Key   string `json:"key,omitempty"`
	JobID string `json:"job_id,omitempty"`
}

type ExecutionLockStore struct {
	mu      sync.RWMutex
	nextID  int64
	byKey   map[string]*ExecutionLock
	byJob   map[string]string
	history []ExecutionLock
}

func NewExecutionLockStore() *ExecutionLockStore {
	return &ExecutionLockStore{
		byKey:   map[string]*ExecutionLock{},
		byJob:   map[string]string{},
		history: make([]ExecutionLock, 0, 2000),
	}
}

func (s *ExecutionLockStore) Acquire(in ExecutionLockAcquireInput) (ExecutionLock, error) {
	key := normalizeLockKey(in.Key)
	holder := strings.TrimSpace(in.Holder)
	if key == "" {
		return ExecutionLock{}, errors.New("key is required")
	}
	if holder == "" {
		holder = "control-plane"
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 600
	}
	now := time.Now().UTC()
	expires := now.Add(time.Duration(ttl) * time.Second)

	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.byKey[key]; current != nil {
		if current.Status == "active" && now.Before(current.ExpiresAt) {
			return ExecutionLock{}, errors.New("execution lock already held for key")
		}
		if current.Status == "active" && !now.Before(current.ExpiresAt) {
			current.Status = "expired"
			current.ReleasedAt = now
			s.history = append(s.history, cloneExecutionLock(*current))
		}
	}
	s.nextID++
	item := &ExecutionLock{
		ID:         "exec-lock-" + itoa(s.nextID),
		Key:        key,
		Holder:     holder,
		AcquiredAt: now,
		ExpiresAt:  expires,
		Status:     "active",
	}
	s.byKey[key] = item
	return cloneExecutionLock(*item), nil
}

func (s *ExecutionLockStore) BindJob(key, jobID string) (ExecutionLock, error) {
	key = normalizeLockKey(key)
	jobID = strings.TrimSpace(jobID)
	if key == "" || jobID == "" {
		return ExecutionLock{}, errors.New("key and job_id are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.byKey[key]
	if item == nil || item.Status != "active" {
		return ExecutionLock{}, errors.New("execution lock not active")
	}
	if !now.Before(item.ExpiresAt) {
		item.Status = "expired"
		item.ReleasedAt = now
		s.history = append(s.history, cloneExecutionLock(*item))
		return ExecutionLock{}, errors.New("execution lock expired")
	}
	item.JobID = jobID
	s.byJob[jobID] = key
	return cloneExecutionLock(*item), nil
}

func (s *ExecutionLockStore) Release(in ExecutionLockReleaseInput) (ExecutionLock, bool) {
	key := normalizeLockKey(in.Key)
	jobID := strings.TrimSpace(in.JobID)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" && jobID != "" {
		key = s.byJob[jobID]
	}
	if key == "" {
		return ExecutionLock{}, false
	}
	item := s.byKey[key]
	if item == nil {
		return ExecutionLock{}, false
	}
	if item.Status == "active" {
		item.Status = "released"
		item.ReleasedAt = now
	}
	if item.JobID != "" {
		delete(s.byJob, item.JobID)
	}
	released := cloneExecutionLock(*item)
	s.history = append(s.history, released)
	delete(s.byKey, key)
	return released, true
}

func (s *ExecutionLockStore) CleanupExpired() []ExecutionLock {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := make([]ExecutionLock, 0)
	for key, item := range s.byKey {
		if item.Status != "active" {
			continue
		}
		if now.Before(item.ExpiresAt) {
			continue
		}
		item.Status = "expired"
		item.ReleasedAt = now
		expired = append(expired, cloneExecutionLock(*item))
		s.history = append(s.history, cloneExecutionLock(*item))
		if item.JobID != "" {
			delete(s.byJob, item.JobID)
		}
		delete(s.byKey, key)
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].Key < expired[j].Key })
	return expired
}

func (s *ExecutionLockStore) List(includeHistory bool) []ExecutionLock {
	s.mu.RLock()
	active := make([]ExecutionLock, 0, len(s.byKey))
	for _, item := range s.byKey {
		active = append(active, cloneExecutionLock(*item))
	}
	history := append([]ExecutionLock{}, s.history...)
	s.mu.RUnlock()
	sort.Slice(active, func(i, j int) bool { return active[i].Key < active[j].Key })
	if !includeHistory {
		return active
	}
	sort.Slice(history, func(i, j int) bool { return history[i].ReleasedAt.After(history[j].ReleasedAt) })
	return append(active, history...)
}

func normalizeLockKey(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}

func cloneExecutionLock(in ExecutionLock) ExecutionLock {
	return in
}
