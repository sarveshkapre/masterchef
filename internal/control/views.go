package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SavedView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Entity     string    `json:"entity"`
	Mode       string    `json:"mode"` // human|ast
	Query      string    `json:"query,omitempty"`
	QueryAST   string    `json:"query_ast,omitempty"` // serialized JSON
	Limit      int       `json:"limit"`
	Pinned     bool      `json:"pinned"`
	ShareToken string    `json:"share_token,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SavedViewStore struct {
	mu     sync.RWMutex
	nextID int64
	views  map[string]*SavedView
}

func NewSavedViewStore() *SavedViewStore {
	return &SavedViewStore{views: map[string]*SavedView{}}
}

func (s *SavedViewStore) Create(in SavedView) (SavedView, error) {
	if strings.TrimSpace(in.Name) == "" {
		return SavedView{}, errors.New("view name is required")
	}
	if strings.TrimSpace(in.Entity) == "" {
		return SavedView{}, errors.New("view entity is required")
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "human"
	}
	if mode != "human" && mode != "ast" {
		return SavedView{}, errors.New("view mode must be human or ast")
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := time.Now().UTC()
	in.ID = "view-" + itoa(s.nextID)
	in.Mode = mode
	in.CreatedAt = now
	in.UpdatedAt = now
	in.ShareToken = generateShareToken(in.ID, now)
	cp := in
	s.views[in.ID] = &cp
	return cp, nil
}

func (s *SavedViewStore) List() []SavedView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SavedView, 0, len(s.views))
	for _, v := range s.views {
		out = append(out, *cloneSavedView(v))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *SavedViewStore) Get(id string) (SavedView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.views[strings.TrimSpace(id)]
	if !ok {
		return SavedView{}, errors.New("saved view not found")
	}
	return *cloneSavedView(v), nil
}

func (s *SavedViewStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if _, ok := s.views[id]; !ok {
		return errors.New("saved view not found")
	}
	delete(s.views, id)
	return nil
}

func (s *SavedViewStore) SetPinned(id string, pinned bool) (SavedView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.views[strings.TrimSpace(id)]
	if !ok {
		return SavedView{}, errors.New("saved view not found")
	}
	v.Pinned = pinned
	v.UpdatedAt = time.Now().UTC()
	return *cloneSavedView(v), nil
}

func (s *SavedViewStore) RegenerateShareToken(id string) (SavedView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.views[strings.TrimSpace(id)]
	if !ok {
		return SavedView{}, errors.New("saved view not found")
	}
	now := time.Now().UTC()
	v.ShareToken = generateShareToken(v.ID, now)
	v.UpdatedAt = now
	return *cloneSavedView(v), nil
}

func cloneSavedView(in *SavedView) *SavedView {
	if in == nil {
		return nil
	}
	cp := *in
	return &cp
}

func generateShareToken(id string, at time.Time) string {
	ts := at.UTC().UnixNano()
	return id + "-" + itoa(ts)
}
