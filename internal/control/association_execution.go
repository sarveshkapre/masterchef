package control

import (
	"sort"
	"sync"
	"time"
)

type AssociationExecutionRecord struct {
	ID            string    `json:"id"`
	AssociationID string    `json:"association_id"`
	JobID         string    `json:"job_id"`
	ConfigPath    string    `json:"config_path"`
	Priority      string    `json:"priority"`
	Status        JobStatus `json:"status"`
	Error         string    `json:"error,omitempty"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
}

type AssociationExecutionStore struct {
	mu     sync.RWMutex
	nextID int64
	limit  int
	items  map[string]*AssociationExecutionRecord // association_id|job_id -> record
}

func NewAssociationExecutionStore(limit int) *AssociationExecutionStore {
	if limit <= 0 {
		limit = 5000
	}
	return &AssociationExecutionStore{
		limit: limit,
		items: map[string]*AssociationExecutionRecord{},
	}
}

func (s *AssociationExecutionStore) RecordJob(associationID string, job Job) AssociationExecutionRecord {
	now := time.Now().UTC()
	key := associationID + "|" + job.ID

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.items[key]; ok {
		existing.Priority = job.Priority
		existing.Status = job.Status
		existing.Error = job.Error
		existing.LastSeenAt = now
		return cloneAssociationExecution(*existing)
	}
	s.nextID++
	rec := &AssociationExecutionRecord{
		ID:            "assoc-exec-" + itoa(s.nextID),
		AssociationID: associationID,
		JobID:         job.ID,
		ConfigPath:    job.ConfigPath,
		Priority:      job.Priority,
		Status:        job.Status,
		Error:         job.Error,
		FirstSeenAt:   now,
		LastSeenAt:    now,
	}
	s.items[key] = rec
	s.pruneLocked()
	return cloneAssociationExecution(*rec)
}

func (s *AssociationExecutionStore) List(associationID string, limit int) []AssociationExecutionRecord {
	if limit <= 0 {
		limit = 200
	}
	s.mu.RLock()
	out := make([]AssociationExecutionRecord, 0, len(s.items))
	for _, item := range s.items {
		if associationID != "" && item.AssociationID != associationID {
			continue
		}
		out = append(out, cloneAssociationExecution(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *AssociationExecutionStore) pruneLocked() {
	if len(s.items) <= s.limit {
		return
	}
	type recRef struct {
		key  string
		time time.Time
	}
	refs := make([]recRef, 0, len(s.items))
	for k, item := range s.items {
		refs = append(refs, recRef{key: k, time: item.LastSeenAt})
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].time.Before(refs[j].time)
	})
	removeCount := len(s.items) - s.limit
	for i := 0; i < removeCount && i < len(refs); i++ {
		delete(s.items, refs[i].key)
	}
}

func cloneAssociationExecution(in AssociationExecutionRecord) AssociationExecutionRecord {
	return in
}
