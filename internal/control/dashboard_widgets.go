package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DashboardWidget struct {
	ID              string    `json:"id"`
	ViewID          string    `json:"view_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description,omitempty"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	Column          int       `json:"column"`
	Row             int       `json:"row"`
	Pinned          bool      `json:"pinned"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastRefreshedAt time.Time `json:"last_refreshed_at,omitempty"`
}

type DashboardWidgetStore struct {
	mu      sync.RWMutex
	nextID  int64
	widgets map[string]*DashboardWidget
}

func NewDashboardWidgetStore() *DashboardWidgetStore {
	return &DashboardWidgetStore{
		widgets: map[string]*DashboardWidget{},
	}
}

func (s *DashboardWidgetStore) Create(in DashboardWidget) (DashboardWidget, error) {
	viewID := strings.TrimSpace(in.ViewID)
	if viewID == "" {
		return DashboardWidget{}, errors.New("view_id is required")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return DashboardWidget{}, errors.New("title is required")
	}
	if in.Width <= 0 {
		in.Width = 6
	}
	if in.Height <= 0 {
		in.Height = 4
	}
	if in.Width > 24 {
		return DashboardWidget{}, errors.New("width must be <= 24")
	}
	if in.Height > 24 {
		return DashboardWidget{}, errors.New("height must be <= 24")
	}
	if in.Column < 0 {
		in.Column = 0
	}
	if in.Row < 0 {
		in.Row = 0
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := DashboardWidget{
		ID:          "widget-" + itoa(s.nextID),
		ViewID:      viewID,
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Width:       in.Width,
		Height:      in.Height,
		Column:      in.Column,
		Row:         in.Row,
		Pinned:      in.Pinned,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.widgets[item.ID] = &item
	return item, nil
}

func (s *DashboardWidgetStore) List() []DashboardWidget {
	s.mu.RLock()
	out := make([]DashboardWidget, 0, len(s.widgets))
	for _, item := range s.widgets {
		out = append(out, *cloneDashboardWidget(item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		if out[i].Column != out[j].Column {
			return out[i].Column < out[j].Column
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *DashboardWidgetStore) Get(id string) (DashboardWidget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.widgets[strings.TrimSpace(id)]
	if !ok {
		return DashboardWidget{}, errors.New("widget not found")
	}
	return *cloneDashboardWidget(item), nil
}

func (s *DashboardWidgetStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if _, ok := s.widgets[id]; !ok {
		return errors.New("widget not found")
	}
	delete(s.widgets, id)
	return nil
}

func (s *DashboardWidgetStore) SetPinned(id string, pinned bool) (DashboardWidget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.widgets[strings.TrimSpace(id)]
	if !ok {
		return DashboardWidget{}, errors.New("widget not found")
	}
	item.Pinned = pinned
	item.UpdatedAt = time.Now().UTC()
	return *cloneDashboardWidget(item), nil
}

func (s *DashboardWidgetStore) Refresh(id string) (DashboardWidget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.widgets[strings.TrimSpace(id)]
	if !ok {
		return DashboardWidget{}, errors.New("widget not found")
	}
	item.LastRefreshedAt = time.Now().UTC()
	item.UpdatedAt = item.LastRefreshedAt
	return *cloneDashboardWidget(item), nil
}

func cloneDashboardWidget(in *DashboardWidget) *DashboardWidget {
	if in == nil {
		return nil
	}
	cp := *in
	return &cp
}
