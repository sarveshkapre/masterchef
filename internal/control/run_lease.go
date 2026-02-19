package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RunLeaseAcquireInput struct {
	JobID      string `json:"job_id"`
	Holder     string `json:"holder"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type RunLeaseHeartbeatInput struct {
	LeaseID string `json:"lease_id,omitempty"`
	JobID   string `json:"job_id,omitempty"`
}

type RunLease struct {
	LeaseID       string    `json:"lease_id"`
	JobID         string    `json:"job_id"`
	Holder        string    `json:"holder"`
	TTLSeconds    int       `json:"ttl_seconds"`
	CreatedAt     time.Time `json:"created_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	ExpiresAt     time.Time `json:"expires_at"`
	RecoveredAt   time.Time `json:"recovered_at,omitempty"`
	Status        string    `json:"status"` // active|released|recovered
}

type RunLeaseStore struct {
	mu      sync.RWMutex
	nextID  int64
	byLease map[string]*RunLease
	byJob   map[string]string
}

func NewRunLeaseStore() *RunLeaseStore {
	return &RunLeaseStore{
		byLease: map[string]*RunLease{},
		byJob:   map[string]string{},
	}
}

func (s *RunLeaseStore) Acquire(in RunLeaseAcquireInput) (RunLease, error) {
	jobID := strings.TrimSpace(in.JobID)
	holder := strings.TrimSpace(in.Holder)
	if jobID == "" || holder == "" {
		return RunLease{}, errors.New("job_id and holder are required")
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 30
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	if leaseID, ok := s.byJob[jobID]; ok {
		if existing := s.byLease[leaseID]; existing != nil {
			existing.Holder = holder
			existing.TTLSeconds = ttl
			existing.LastHeartbeat = now
			existing.ExpiresAt = now.Add(time.Duration(ttl) * time.Second)
			existing.Status = "active"
			existing.RecoveredAt = time.Time{}
			return cloneRunLease(*existing), nil
		}
	}

	s.nextID++
	item := &RunLease{
		LeaseID:       "lease-" + itoa(s.nextID),
		JobID:         jobID,
		Holder:        holder,
		TTLSeconds:    ttl,
		CreatedAt:     now,
		LastHeartbeat: now,
		ExpiresAt:     now.Add(time.Duration(ttl) * time.Second),
		Status:        "active",
	}
	s.byLease[item.LeaseID] = item
	s.byJob[jobID] = item.LeaseID
	return cloneRunLease(*item), nil
}

func (s *RunLeaseStore) Heartbeat(in RunLeaseHeartbeatInput) (RunLease, error) {
	leaseID := strings.TrimSpace(in.LeaseID)
	jobID := strings.TrimSpace(in.JobID)
	if leaseID == "" && jobID == "" {
		return RunLease{}, errors.New("lease_id or job_id is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.resolveLeaseLocked(leaseID, jobID)
	if err != nil {
		return RunLease{}, err
	}
	if item.Status != "active" {
		return RunLease{}, errors.New("lease is not active")
	}
	item.LastHeartbeat = now
	item.ExpiresAt = now.Add(time.Duration(item.TTLSeconds) * time.Second)
	return cloneRunLease(*item), nil
}

func (s *RunLeaseStore) Release(in RunLeaseHeartbeatInput) (RunLease, error) {
	leaseID := strings.TrimSpace(in.LeaseID)
	jobID := strings.TrimSpace(in.JobID)
	if leaseID == "" && jobID == "" {
		return RunLease{}, errors.New("lease_id or job_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.resolveLeaseLocked(leaseID, jobID)
	if err != nil {
		return RunLease{}, err
	}
	item.Status = "released"
	item.ExpiresAt = time.Now().UTC()
	return cloneRunLease(*item), nil
}

func (s *RunLeaseStore) List(includeRecovered bool) []RunLease {
	s.mu.RLock()
	out := make([]RunLease, 0, len(s.byLease))
	for _, item := range s.byLease {
		if !includeRecovered && item.Status == "recovered" {
			continue
		}
		out = append(out, cloneRunLease(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *RunLeaseStore) RecoverExpired(now time.Time) []RunLease {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	recovered := make([]RunLease, 0)
	for _, item := range s.byLease {
		if item.Status != "active" {
			continue
		}
		if now.Before(item.ExpiresAt) {
			continue
		}
		item.Status = "recovered"
		item.RecoveredAt = now
		recovered = append(recovered, cloneRunLease(*item))
	}
	s.mu.Unlock()
	sort.Slice(recovered, func(i, j int) bool { return recovered[i].LeaseID < recovered[j].LeaseID })
	return recovered
}

func (s *RunLeaseStore) resolveLeaseLocked(leaseID, jobID string) (*RunLease, error) {
	if leaseID != "" {
		item := s.byLease[leaseID]
		if item == nil {
			return nil, errors.New("lease not found")
		}
		return item, nil
	}
	mapped := s.byJob[jobID]
	if mapped == "" {
		return nil, errors.New("lease not found")
	}
	item := s.byLease[mapped]
	if item == nil {
		return nil, errors.New("lease not found")
	}
	return item, nil
}

func cloneRunLease(in RunLease) RunLease {
	return in
}
