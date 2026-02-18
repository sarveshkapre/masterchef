package control

import (
	"errors"
	"sync"
	"time"
)

type Template struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ConfigPath  string            `json:"config_path"`
	Defaults    map[string]string `json:"defaults,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type TemplateStore struct {
	mu        sync.RWMutex
	nextID    int64
	templates map[string]*Template
}

func NewTemplateStore() *TemplateStore {
	return &TemplateStore{
		templates: map[string]*Template{},
	}
}

func (s *TemplateStore) Create(t Template) Template {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	t.ID = "tpl-" + itoa(s.nextID)
	t.CreatedAt = time.Now().UTC()
	if t.Defaults == nil {
		t.Defaults = map[string]string{}
	}
	cp := t
	s.templates[t.ID] = &cp
	return cp
}

func (s *TemplateStore) List() []Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Template, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, *cloneTemplate(t))
	}
	return out
}

func (s *TemplateStore) Get(id string) (Template, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	if !ok {
		return Template{}, false
	}
	return *cloneTemplate(t), true
}

func (s *TemplateStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.templates[id]; !ok {
		return errors.New("template not found")
	}
	delete(s.templates, id)
	return nil
}

func cloneTemplate(t *Template) *Template {
	if t == nil {
		return nil
	}
	cp := *t
	cp.Defaults = map[string]string{}
	for k, v := range t.Defaults {
		cp.Defaults[k] = v
	}
	return &cp
}
