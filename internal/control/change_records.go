package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChangeRecordStatus string

const (
	ChangeRecordProposed  ChangeRecordStatus = "proposed"
	ChangeRecordApproved  ChangeRecordStatus = "approved"
	ChangeRecordRejected  ChangeRecordStatus = "rejected"
	ChangeRecordExecuting ChangeRecordStatus = "executing"
	ChangeRecordCompleted ChangeRecordStatus = "completed"
	ChangeRecordFailed    ChangeRecordStatus = "failed"
)

type ChangeApproval struct {
	Actor     string    `json:"actor"`
	Decision  string    `json:"decision"` // approve|reject
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ChangeRecord struct {
	ID            string             `json:"id"`
	Summary       string             `json:"summary"`
	TicketSystem  string             `json:"ticket_system,omitempty"`
	TicketID      string             `json:"ticket_id,omitempty"`
	TicketURL     string             `json:"ticket_url,omitempty"`
	ConfigPath    string             `json:"config_path,omitempty"`
	RequestedBy   string             `json:"requested_by,omitempty"`
	Status        ChangeRecordStatus `json:"status"`
	Approvals     []ChangeApproval   `json:"approvals,omitempty"`
	LinkedJobID   string             `json:"linked_job_id,omitempty"`
	FailureReason string             `json:"failure_reason,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type ChangeRecordStore struct {
	mu      sync.RWMutex
	nextID  int64
	records map[string]*ChangeRecord
}

func NewChangeRecordStore() *ChangeRecordStore {
	return &ChangeRecordStore{records: map[string]*ChangeRecord{}}
}

func (s *ChangeRecordStore) Create(in ChangeRecord) (ChangeRecord, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return ChangeRecord{}, errors.New("change record summary is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := time.Now().UTC()
	in.ID = "cr-" + itoa(s.nextID)
	in.Status = ChangeRecordProposed
	in.CreatedAt = now
	in.UpdatedAt = now
	in.Approvals = nil
	cp := cloneChangeRecord(in)
	s.records[in.ID] = &cp
	return cp, nil
}

func (s *ChangeRecordStore) List() []ChangeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ChangeRecord, 0, len(s.records))
	for _, rec := range s.records {
		out = append(out, cloneChangeRecord(*rec))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ChangeRecordStore) Get(id string) (ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		return ChangeRecord{}, errors.New("change record not found")
	}
	return cloneChangeRecord(*rec), nil
}

func (s *ChangeRecordStore) Approve(id, actor, comment string) (ChangeRecord, error) {
	return s.recordDecision(id, actor, "approve", comment)
}

func (s *ChangeRecordStore) Reject(id, actor, comment string) (ChangeRecord, error) {
	return s.recordDecision(id, actor, "reject", comment)
}

func (s *ChangeRecordStore) recordDecision(id, actor, decision, comment string) (ChangeRecord, error) {
	id = strings.TrimSpace(id)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return ChangeRecord{}, errors.New("actor is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return ChangeRecord{}, errors.New("change record not found")
	}
	now := time.Now().UTC()
	approval := ChangeApproval{
		Actor:     actor,
		Decision:  decision,
		Comment:   strings.TrimSpace(comment),
		CreatedAt: now,
	}
	rec.Approvals = append(rec.Approvals, approval)
	if decision == "approve" {
		rec.Status = ChangeRecordApproved
	} else {
		rec.Status = ChangeRecordRejected
	}
	rec.UpdatedAt = now
	return cloneChangeRecord(*rec), nil
}

func (s *ChangeRecordStore) AttachJob(id, jobID string) (ChangeRecord, error) {
	id = strings.TrimSpace(id)
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ChangeRecord{}, errors.New("job_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return ChangeRecord{}, errors.New("change record not found")
	}
	rec.LinkedJobID = jobID
	rec.Status = ChangeRecordExecuting
	rec.UpdatedAt = time.Now().UTC()
	return cloneChangeRecord(*rec), nil
}

func (s *ChangeRecordStore) MarkCompleted(id string) (ChangeRecord, error) {
	return s.setTerminalStatus(id, ChangeRecordCompleted, "")
}

func (s *ChangeRecordStore) MarkFailed(id, reason string) (ChangeRecord, error) {
	return s.setTerminalStatus(id, ChangeRecordFailed, reason)
}

func (s *ChangeRecordStore) setTerminalStatus(id string, status ChangeRecordStatus, reason string) (ChangeRecord, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return ChangeRecord{}, errors.New("change record not found")
	}
	rec.Status = status
	rec.FailureReason = strings.TrimSpace(reason)
	rec.UpdatedAt = time.Now().UTC()
	return cloneChangeRecord(*rec), nil
}

func cloneChangeRecord(in ChangeRecord) ChangeRecord {
	out := in
	out.Approvals = append([]ChangeApproval{}, in.Approvals...)
	return out
}
