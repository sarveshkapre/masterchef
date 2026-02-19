package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SystemdUnitInput struct {
	Name         string            `json:"name"`
	Content      string            `json:"content"`
	DropIns      map[string]string `json:"drop_ins,omitempty"`
	Enabled      bool              `json:"enabled"`
	DesiredState string            `json:"desired_state,omitempty"` // running|stopped
}

type SystemdUnit struct {
	Name         string            `json:"name"`
	Content      string            `json:"content"`
	DropIns      map[string]string `json:"drop_ins,omitempty"`
	Enabled      bool              `json:"enabled"`
	DesiredState string            `json:"desired_state"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type SystemdRenderInput struct {
	Name string `json:"name"`
}

type SystemdRenderOutput struct {
	Name           string            `json:"name"`
	UnitPath       string            `json:"unit_path"`
	DropInPaths    map[string]string `json:"drop_in_paths,omitempty"`
	UnitContent    string            `json:"unit_content"`
	DropInContents map[string]string `json:"drop_in_contents,omitempty"`
}

type SystemdUnitStore struct {
	mu    sync.RWMutex
	units map[string]*SystemdUnit
}

func NewSystemdUnitStore() *SystemdUnitStore {
	return &SystemdUnitStore{units: map[string]*SystemdUnit{}}
}

func (s *SystemdUnitStore) Upsert(in SystemdUnitInput) (SystemdUnit, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if name == "" {
		return SystemdUnit{}, errors.New("name is required")
	}
	if !strings.HasSuffix(name, ".service") {
		return SystemdUnit{}, errors.New("systemd unit name must end with .service")
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return SystemdUnit{}, errors.New("content is required")
	}
	state := strings.ToLower(strings.TrimSpace(in.DesiredState))
	if state == "" {
		state = "running"
	}
	if state != "running" && state != "stopped" {
		return SystemdUnit{}, errors.New("desired_state must be running or stopped")
	}
	dropIns := normalizeSystemdDropIns(in.DropIns)
	item := SystemdUnit{
		Name:         name,
		Content:      content,
		DropIns:      dropIns,
		Enabled:      in.Enabled,
		DesiredState: state,
		UpdatedAt:    time.Now().UTC(),
	}
	s.mu.Lock()
	s.units[name] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *SystemdUnitStore) List() []SystemdUnit {
	s.mu.RLock()
	out := make([]SystemdUnit, 0, len(s.units))
	for _, item := range s.units {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *SystemdUnitStore) Get(name string) (SystemdUnit, bool) {
	s.mu.RLock()
	item, ok := s.units[strings.ToLower(strings.TrimSpace(name))]
	s.mu.RUnlock()
	if !ok {
		return SystemdUnit{}, false
	}
	return *item, true
}

func (s *SystemdUnitStore) Render(in SystemdRenderInput) (SystemdRenderOutput, error) {
	item, ok := s.Get(in.Name)
	if !ok {
		return SystemdRenderOutput{}, errors.New("systemd unit not found")
	}
	out := SystemdRenderOutput{
		Name:        item.Name,
		UnitPath:    "/etc/systemd/system/" + item.Name,
		UnitContent: item.Content,
	}
	if len(item.DropIns) > 0 {
		out.DropInPaths = map[string]string{}
		out.DropInContents = map[string]string{}
		for file, body := range item.DropIns {
			base := sanitizeSystemdDropIn(file)
			out.DropInPaths[base] = "/etc/systemd/system/" + item.Name + ".d/" + base
			out.DropInContents[base] = body
		}
	}
	return out, nil
}

func sanitizeSystemdDropIn(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "10-masterchef.conf"
	}
	if !strings.HasSuffix(name, ".conf") {
		name += ".conf"
	}
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func normalizeSystemdDropIns(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		val := strings.TrimSpace(v)
		if val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
