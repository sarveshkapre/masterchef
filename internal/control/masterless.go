package control

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type MasterlessModeInput struct {
	Enabled         bool   `json:"enabled"`
	StateRoot       string `json:"state_root,omitempty"`
	DefaultStrategy string `json:"default_strategy,omitempty"`
}

type MasterlessMode struct {
	Enabled         bool      `json:"enabled"`
	StateRoot       string    `json:"state_root,omitempty"`
	DefaultStrategy string    `json:"default_strategy"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type MasterlessRenderInput struct {
	StateTemplate string            `json:"state_template"`
	Strategy      string            `json:"strategy,omitempty"`
	Layers        []PillarLayer     `json:"layers,omitempty"`
	Lookups       []string          `json:"lookups,omitempty"`
	Vars          map[string]string `json:"vars,omitempty"`
}

type MasterlessRenderResult struct {
	RenderedState   string         `json:"rendered_state"`
	ResolvedPillar  map[string]any `json:"resolved_pillar"`
	Lookups         map[string]any `json:"lookups,omitempty"`
	MissingTokens   []string       `json:"missing_tokens,omitempty"`
	Deterministic   bool           `json:"deterministic"`
	EffectiveMode   MasterlessMode `json:"effective_mode"`
	EffectiveMethod string         `json:"effective_strategy"`
}

type MasterlessStore struct {
	mu   sync.RWMutex
	mode MasterlessMode
}

func NewMasterlessStore() *MasterlessStore {
	return &MasterlessStore{
		mode: MasterlessMode{
			Enabled:         false,
			DefaultStrategy: "merge-last",
			UpdatedAt:       time.Now().UTC(),
		},
	}
}

func (s *MasterlessStore) Mode() MasterlessMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

func (s *MasterlessStore) SetMode(in MasterlessModeInput) (MasterlessMode, error) {
	strategy := normalizePillarStrategy(in.DefaultStrategy)
	if strategy == "" {
		strategy = "merge-last"
	}
	stateRoot := strings.TrimSpace(in.StateRoot)
	if in.Enabled && stateRoot == "" {
		return MasterlessMode{}, errors.New("state_root is required when masterless mode is enabled")
	}
	mode := MasterlessMode{
		Enabled:         in.Enabled,
		StateRoot:       stateRoot,
		DefaultStrategy: strategy,
		UpdatedAt:       time.Now().UTC(),
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return mode, nil
}

func (s *MasterlessStore) Render(in MasterlessRenderInput) (MasterlessRenderResult, error) {
	mode := s.Mode()
	if !mode.Enabled {
		return MasterlessRenderResult{}, errors.New("masterless mode is disabled")
	}
	template := strings.TrimSpace(in.StateTemplate)
	if template == "" {
		return MasterlessRenderResult{}, errors.New("state_template is required")
	}
	strategy := normalizePillarStrategy(in.Strategy)
	if strategy == "" {
		strategy = mode.DefaultStrategy
	}
	resolved, err := ResolvePillar(PillarResolveRequest{
		Strategy: strategy,
		Layers:   append([]PillarLayer{}, in.Layers...),
	})
	if err != nil {
		return MasterlessRenderResult{}, err
	}
	lookups := map[string]any{}
	for _, lookup := range normalizeStringSlice(in.Lookups) {
		if value, ok := lookupMasterlessPath(resolved.Merged, lookup); ok {
			lookups[lookup] = value
		}
	}
	rendered, missing := renderMasterlessTemplate(template, resolved.Merged, in.Vars)
	sort.Strings(missing)
	return MasterlessRenderResult{
		RenderedState:   rendered,
		ResolvedPillar:  resolved.Merged,
		Lookups:         lookups,
		MissingTokens:   missing,
		Deterministic:   len(missing) == 0,
		EffectiveMode:   mode,
		EffectiveMethod: strategy,
	}, nil
}

var masterlessTokenPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func renderMasterlessTemplate(template string, pillar map[string]any, vars map[string]string) (string, []string) {
	var missing []string
	out := masterlessTokenPattern.ReplaceAllStringFunc(template, func(token string) string {
		match := masterlessTokenPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		key := strings.TrimSpace(match[1])
		if strings.HasPrefix(key, "pillar.") {
			path := strings.TrimPrefix(key, "pillar.")
			if value, ok := lookupMasterlessPath(pillar, path); ok {
				if text, ok := value.(string); ok {
					return text
				}
				return stringifyAny(value)
			}
			missing = append(missing, key)
			return token
		}
		if strings.HasPrefix(key, "var.") {
			name := strings.TrimPrefix(key, "var.")
			if value, ok := vars[name]; ok {
				return value
			}
			missing = append(missing, key)
			return token
		}
		missing = append(missing, key)
		return token
	})
	missing = normalizeStringSlice(missing)
	return out, missing
}

func lookupMasterlessPath(root map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = value
	}
	return cur, true
}

func stringifyAny(in any) string {
	return fmt.Sprint(in)
}
