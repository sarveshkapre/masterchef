package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Association struct {
	ID         string        `json:"id"`
	ConfigPath string        `json:"config_path"`
	TargetKind string        `json:"target_kind"`
	TargetName string        `json:"target_name"`
	Priority   string        `json:"priority"`
	Interval   time.Duration `json:"interval"`
	Jitter     time.Duration `json:"jitter"`
	Enabled    bool          `json:"enabled"`
	ScheduleID string        `json:"schedule_id"`
	Revision   int           `json:"revision"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type AssociationRevision struct {
	Revision  int         `json:"revision"`
	Action    string      `json:"action"`
	CreatedAt time.Time   `json:"created_at"`
	Snapshot  Association `json:"snapshot"`
}

type AssociationCreate struct {
	ConfigPath string
	TargetKind string
	TargetName string
	Priority   string
	Interval   time.Duration
	Jitter     time.Duration
	Enabled    bool
}

type AssociationStore struct {
	mu        sync.RWMutex
	nextID    int64
	items     map[string]*Association
	history   map[string][]AssociationRevision
	scheduler *Scheduler
}

func NewAssociationStore(scheduler *Scheduler) *AssociationStore {
	return &AssociationStore{
		items:     map[string]*Association{},
		history:   map[string][]AssociationRevision{},
		scheduler: scheduler,
	}
}

func (s *AssociationStore) Create(in AssociationCreate) (Association, error) {
	if strings.TrimSpace(in.ConfigPath) == "" {
		return Association{}, errors.New("config_path is required")
	}
	kind := strings.ToLower(strings.TrimSpace(in.TargetKind))
	if kind != "host" && kind != "cluster" && kind != "environment" {
		return Association{}, errors.New("target_kind must be host, cluster, or environment")
	}
	if strings.TrimSpace(in.TargetName) == "" {
		return Association{}, errors.New("target_name is required")
	}
	if in.Interval <= 0 {
		in.Interval = 60 * time.Second
	}

	opts := ScheduleOptions{
		ConfigPath: in.ConfigPath,
		Priority:   in.Priority,
		Interval:   in.Interval,
		Jitter:     in.Jitter,
	}
	applyTargetToScheduleOptions(kind, in.TargetName, &opts)
	sc := s.scheduler.CreateWithOptions(opts)
	if !in.Enabled {
		s.scheduler.Disable(sc.ID)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := "assoc-" + itoa(s.nextID)
	assoc := &Association{
		ID:         id,
		ConfigPath: in.ConfigPath,
		TargetKind: kind,
		TargetName: in.TargetName,
		Priority:   normalizePriority(in.Priority),
		Interval:   in.Interval,
		Jitter:     in.Jitter,
		Enabled:    in.Enabled,
		ScheduleID: sc.ID,
		Revision:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.items[id] = assoc
	s.history[id] = append(s.history[id], AssociationRevision{
		Revision:  assoc.Revision,
		Action:    "create",
		CreatedAt: now,
		Snapshot:  cloneAssociation(*assoc),
	})
	return cloneAssociation(*assoc), nil
}

func (s *AssociationStore) List() []Association {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Association, 0, len(s.items))
	for _, assoc := range s.items {
		out = append(out, cloneAssociation(*assoc))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *AssociationStore) Revisions(id string) ([]AssociationRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rev, ok := s.history[id]
	if !ok {
		return nil, errors.New("association not found")
	}
	out := make([]AssociationRevision, len(rev))
	copy(out, rev)
	return out, nil
}

func (s *AssociationStore) SetEnabled(id string, enabled bool) (Association, error) {
	s.mu.Lock()
	assoc, ok := s.items[id]
	if !ok {
		s.mu.Unlock()
		return Association{}, errors.New("association not found")
	}
	if enabled {
		s.scheduler.Enable(assoc.ScheduleID)
	} else {
		s.scheduler.Disable(assoc.ScheduleID)
	}
	assoc.Enabled = enabled
	assoc.Revision++
	assoc.UpdatedAt = time.Now().UTC()
	rev := AssociationRevision{
		Revision:  assoc.Revision,
		Action:    map[bool]string{true: "enable", false: "disable"}[enabled],
		CreatedAt: assoc.UpdatedAt,
		Snapshot:  cloneAssociation(*assoc),
	}
	s.history[id] = append(s.history[id], rev)
	out := cloneAssociation(*assoc)
	s.mu.Unlock()
	return out, nil
}

func (s *AssociationStore) Replay(id string, revision int) (Association, error) {
	s.mu.Lock()
	assoc, ok := s.items[id]
	if !ok {
		s.mu.Unlock()
		return Association{}, errors.New("association not found")
	}
	history := s.history[id]
	var target *AssociationRevision
	for i := range history {
		if history[i].Revision == revision {
			target = &history[i]
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		return Association{}, errors.New("revision not found")
	}

	s.scheduler.Disable(assoc.ScheduleID)
	opts := ScheduleOptions{
		ConfigPath: target.Snapshot.ConfigPath,
		Priority:   target.Snapshot.Priority,
		Interval:   target.Snapshot.Interval,
		Jitter:     target.Snapshot.Jitter,
	}
	applyTargetToScheduleOptions(target.Snapshot.TargetKind, target.Snapshot.TargetName, &opts)
	sc := s.scheduler.CreateWithOptions(opts)
	if !target.Snapshot.Enabled {
		s.scheduler.Disable(sc.ID)
	}

	assoc.ConfigPath = target.Snapshot.ConfigPath
	assoc.TargetKind = target.Snapshot.TargetKind
	assoc.TargetName = target.Snapshot.TargetName
	assoc.Priority = target.Snapshot.Priority
	assoc.Interval = target.Snapshot.Interval
	assoc.Jitter = target.Snapshot.Jitter
	assoc.Enabled = target.Snapshot.Enabled
	assoc.ScheduleID = sc.ID
	assoc.Revision++
	assoc.UpdatedAt = time.Now().UTC()
	s.history[id] = append(s.history[id], AssociationRevision{
		Revision:  assoc.Revision,
		Action:    "replay",
		CreatedAt: assoc.UpdatedAt,
		Snapshot:  cloneAssociation(*assoc),
	})
	out := cloneAssociation(*assoc)
	s.mu.Unlock()
	return out, nil
}

func applyTargetToScheduleOptions(kind, name string, opts *ScheduleOptions) {
	if opts == nil {
		return
	}
	switch kind {
	case "host":
		opts.Host = name
	case "cluster":
		opts.Cluster = name
	case "environment":
		opts.Environment = name
	}
}

func cloneAssociation(in Association) Association {
	return in
}
