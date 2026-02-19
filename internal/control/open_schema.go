package control

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type OpenSchemaDocument struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Format    string    `json:"format"` // yaml|cue|json_schema
	Content   string    `json:"content"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenSchemaInput struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

type OpenSchemaValidationInput struct {
	SchemaID string `json:"schema_id"`
	Document string `json:"document"`
}

type OpenSchemaValidationResult struct {
	SchemaID string   `json:"schema_id"`
	Format   string   `json:"format"`
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
}

type OpenSchemaStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]*OpenSchemaDocument
}

func NewOpenSchemaStore() *OpenSchemaStore {
	return &OpenSchemaStore{items: map[string]*OpenSchemaDocument{}}
}

func (s *OpenSchemaStore) Upsert(in OpenSchemaInput) (OpenSchemaDocument, error) {
	name := strings.TrimSpace(in.Name)
	format := normalizeOpenSchemaFormat(in.Format)
	content := strings.TrimSpace(in.Content)
	if name == "" || format == "" || content == "" {
		return OpenSchemaDocument{}, errors.New("name, format, and content are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if strings.EqualFold(item.Name, name) {
			item.Format = format
			item.Content = content
			item.Enabled = in.Enabled
			item.UpdatedAt = now
			return cloneOpenSchemaDocument(*item), nil
		}
	}
	s.nextID++
	item := &OpenSchemaDocument{
		ID:        "schema-doc-" + itoa(s.nextID),
		Name:      name,
		Format:    format,
		Content:   content,
		Enabled:   in.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.items[item.ID] = item
	return cloneOpenSchemaDocument(*item), nil
}

func (s *OpenSchemaStore) List() []OpenSchemaDocument {
	s.mu.RLock()
	out := make([]OpenSchemaDocument, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneOpenSchemaDocument(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *OpenSchemaStore) Get(id string) (OpenSchemaDocument, bool) {
	s.mu.RLock()
	item, ok := s.items[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return OpenSchemaDocument{}, false
	}
	return cloneOpenSchemaDocument(*item), true
}

func (s *OpenSchemaStore) Validate(in OpenSchemaValidationInput) OpenSchemaValidationResult {
	result := OpenSchemaValidationResult{SchemaID: strings.TrimSpace(in.SchemaID)}
	doc, ok := s.Get(in.SchemaID)
	if !ok {
		result.Errors = []string{"schema not found"}
		return result
	}
	result.Format = doc.Format
	if !doc.Enabled {
		result.Errors = []string{"schema is disabled"}
		return result
	}
	payload := map[string]any{}
	if err := yaml.Unmarshal([]byte(in.Document), &payload); err != nil {
		result.Errors = []string{"document parse failed: " + err.Error()}
		return result
	}
	if payload == nil {
		payload = map[string]any{}
	}
	errs := validateAgainstSchema(doc, payload)
	result.Valid = len(errs) == 0
	result.Errors = errs
	return result
}

func validateAgainstSchema(doc OpenSchemaDocument, payload map[string]any) []string {
	switch doc.Format {
	case "yaml":
		var schemaDoc map[string]any
		if err := yaml.Unmarshal([]byte(doc.Content), &schemaDoc); err != nil {
			return []string{"yaml schema parse failed: " + err.Error()}
		}
		return nil
	case "json_schema":
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal([]byte(doc.Content), &schema); err != nil {
			return []string{"json schema parse failed: " + err.Error()}
		}
		missing := make([]string, 0)
		for _, field := range schema.Required {
			if _, ok := payload[field]; !ok {
				missing = append(missing, field)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		sort.Strings(missing)
		errs := make([]string, 0, len(missing))
		for _, field := range missing {
			errs = append(errs, "missing required field: "+field)
		}
		return errs
	case "cue":
		required := parseCueRequiredFields(doc.Content)
		errs := make([]string, 0)
		for _, field := range required {
			if _, ok := payload[field]; !ok {
				errs = append(errs, "missing required field: "+field)
			}
		}
		return errs
	default:
		return []string{"unsupported schema format"}
	}
}

func parseCueRequiredFields(content string) []string {
	lines := strings.Split(content, "\n")
	fields := make([]string, 0)
	seen := map[string]struct{}{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		field := strings.TrimSpace(line[:idx])
		if field == "" || strings.ContainsAny(field, " {}[]()") {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func normalizeOpenSchemaFormat(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "yaml":
		return "yaml"
	case "cue":
		return "cue"
	case "json_schema", "json-schema", "jsonschema":
		return "json_schema"
	default:
		return ""
	}
}

func cloneOpenSchemaDocument(in OpenSchemaDocument) OpenSchemaDocument {
	return in
}
