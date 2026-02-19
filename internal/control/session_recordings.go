package control

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionRecording struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Host       string    `json:"host"`
	Transport  string    `json:"transport"`
	ResourceID string    `json:"resource_id"`
	Command    string    `json:"command,omitempty"`
	Become     bool      `json:"become"`
	BecomeUser string    `json:"become_user,omitempty"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	Path       string    `json:"path"`
}

type SessionRecordingStore struct {
	baseDir string
}

func NewSessionRecordingStore(baseDir string) *SessionRecordingStore {
	return &SessionRecordingStore{baseDir: strings.TrimSpace(baseDir)}
}

func (s *SessionRecordingStore) sessionsDir() string {
	return filepath.Join(s.baseDir, ".masterchef", "sessions")
}

func (s *SessionRecordingStore) List(limit int, host, transport string) []SessionRecording {
	if limit <= 0 {
		limit = 100
	}
	host = strings.ToLower(strings.TrimSpace(host))
	transport = strings.ToLower(strings.TrimSpace(transport))
	dir := s.sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]SessionRecording, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		item, err := s.readByID(id)
		if err != nil {
			continue
		}
		if host != "" && strings.ToLower(strings.TrimSpace(item.Host)) != host {
			continue
		}
		if transport != "" && strings.ToLower(strings.TrimSpace(item.Transport)) != transport {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *SessionRecordingStore) Get(id string) (SessionRecording, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionRecording{}, errors.New("session id is required")
	}
	item, err := s.readByID(id)
	if err != nil {
		return SessionRecording{}, err
	}
	return item, nil
}

func (s *SessionRecordingStore) readByID(id string) (SessionRecording, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return SessionRecording{}, errors.New("invalid session id")
	}
	path := filepath.Join(s.sessionsDir(), id+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		return SessionRecording{}, err
	}
	var payload struct {
		Timestamp  time.Time `json:"timestamp"`
		Host       string    `json:"host"`
		Transport  string    `json:"transport"`
		Resource   string    `json:"resource_id"`
		Command    string    `json:"command,omitempty"`
		Become     bool      `json:"become"`
		BecomeUser string    `json:"become_user,omitempty"`
		Output     string    `json:"output,omitempty"`
		Error      string    `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return SessionRecording{}, err
	}
	return SessionRecording{
		ID:         id,
		Timestamp:  payload.Timestamp,
		Host:       payload.Host,
		Transport:  payload.Transport,
		ResourceID: payload.Resource,
		Command:    payload.Command,
		Become:     payload.Become,
		BecomeUser: payload.BecomeUser,
		Output:     payload.Output,
		Error:      payload.Error,
		Path:       path,
	}, nil
}
