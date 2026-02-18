package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	baseDir string
}

type RunStatus string

const (
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

type ResourceRun struct {
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Changed    bool   `json:"changed"`
	Skipped    bool   `json:"skipped"`
	Message    string `json:"message"`
}

type RunRecord struct {
	ID        string        `json:"id"`
	StartedAt time.Time     `json:"started_at"`
	EndedAt   time.Time     `json:"ended_at"`
	Status    RunStatus     `json:"status"`
	Results   []ResourceRun `json:"results"`
}

func New(baseDir string) *Store {
	return &Store{baseDir: filepath.Join(baseDir, ".masterchef")}
}

func (s *Store) SaveRun(r RunRecord) error {
	if err := os.MkdirAll(filepath.Join(s.baseDir, "runs"), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}
	path := filepath.Join(s.baseDir, "runs", r.ID+".json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write run record: %w", err)
	}
	return nil
}
