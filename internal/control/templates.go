package control

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SurveyField struct {
	Type     string   `json:"type"` // string|int|bool
	Required bool     `json:"required,omitempty"`
	Enum     []string `json:"enum,omitempty"`
}

type Template struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	ConfigPath  string                 `json:"config_path"`
	Defaults    map[string]string      `json:"defaults,omitempty"`
	Survey      map[string]SurveyField `json:"survey,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
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
	if t.Survey == nil {
		t.Survey = map[string]SurveyField{}
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
	cp.Survey = map[string]SurveyField{}
	for k, v := range t.Survey {
		cp.Survey[k] = v
	}
	return &cp
}

func ValidateSurveyAnswers(schema map[string]SurveyField, answers map[string]string) error {
	if len(schema) == 0 {
		return nil
	}
	if answers == nil {
		answers = map[string]string{}
	}

	for key := range answers {
		if _, ok := schema[key]; !ok {
			return fmt.Errorf("unknown survey answer field: %s", key)
		}
	}

	for field, def := range schema {
		raw, ok := answers[field]
		raw = strings.TrimSpace(raw)
		if def.Required && (!ok || raw == "") {
			return fmt.Errorf("missing required survey field: %s", field)
		}
		if !ok || raw == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(def.Type)) {
		case "", "string":
		case "int", "integer":
			if _, err := strconv.Atoi(raw); err != nil {
				return fmt.Errorf("invalid integer value for %s", field)
			}
		case "bool", "boolean":
			if _, err := strconv.ParseBool(raw); err != nil {
				return fmt.Errorf("invalid boolean value for %s", field)
			}
		default:
			return fmt.Errorf("unsupported survey field type for %s: %s", field, def.Type)
		}
		if len(def.Enum) > 0 {
			match := false
			for _, allowed := range def.Enum {
				if raw == allowed {
					match = true
					break
				}
			}
			if !match {
				return fmt.Errorf("invalid value for %s: must be one of %v", field, def.Enum)
			}
		}
	}
	return nil
}
