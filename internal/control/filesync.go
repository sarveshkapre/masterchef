package control

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FileSyncPipeline struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	StagingPath string    `json:"staging_path"`
	LivePath    string    `json:"live_path"`
	Workers     int       `json:"workers"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	FilesSynced int       `json:"files_synced"`
	BytesSynced int64     `json:"bytes_synced"`
}

type FileSyncPipelineInput struct {
	Name        string `json:"name"`
	StagingPath string `json:"staging_path"`
	LivePath    string `json:"live_path"`
	Workers     int    `json:"workers,omitempty"`
}

type FileSyncStore struct {
	mu        sync.RWMutex
	nextID    int64
	pipelines map[string]*FileSyncPipeline
}

func NewFileSyncStore() *FileSyncStore {
	return &FileSyncStore{
		pipelines: map[string]*FileSyncPipeline{},
	}
}

func (s *FileSyncStore) Create(in FileSyncPipelineInput) (FileSyncPipeline, error) {
	name := strings.TrimSpace(in.Name)
	staging := strings.TrimSpace(in.StagingPath)
	live := strings.TrimSpace(in.LivePath)
	if name == "" || staging == "" || live == "" {
		return FileSyncPipeline{}, errors.New("name, staging_path, and live_path are required")
	}
	if in.Workers <= 0 {
		in.Workers = 4
	}
	if in.Workers > 128 {
		in.Workers = 128
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &FileSyncPipeline{
		ID:          "filesync-" + itoa(s.nextID),
		Name:        name,
		StagingPath: staging,
		LivePath:    live,
		Workers:     in.Workers,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.pipelines[item.ID] = item
	return cloneFileSyncPipeline(*item), nil
}

func (s *FileSyncStore) Get(id string) (FileSyncPipeline, bool) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.pipelines[id]
	if !ok {
		return FileSyncPipeline{}, false
	}
	return cloneFileSyncPipeline(*item), true
}

func (s *FileSyncStore) List() []FileSyncPipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FileSyncPipeline, 0, len(s.pipelines))
	for _, item := range s.pipelines {
		out = append(out, cloneFileSyncPipeline(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *FileSyncStore) Run(id string) (FileSyncPipeline, error) {
	s.mu.Lock()
	item, ok := s.pipelines[strings.TrimSpace(id)]
	s.mu.Unlock()
	if !ok {
		return FileSyncPipeline{}, errors.New("file sync pipeline not found")
	}

	files, bytesSynced, err := syncTree(item.StagingPath, item.LivePath)
	if err != nil {
		return FileSyncPipeline{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	item.FilesSynced = files
	item.BytesSynced = bytesSynced
	item.LastRunAt = time.Now().UTC()
	item.UpdatedAt = item.LastRunAt
	return cloneFileSyncPipeline(*item), nil
}

func syncTree(stagingPath, livePath string) (int, int64, error) {
	stagingPath = strings.TrimSpace(stagingPath)
	livePath = strings.TrimSpace(livePath)
	if stagingPath == "" || livePath == "" {
		return 0, 0, errors.New("staging and live paths are required")
	}
	info, err := os.Stat(stagingPath)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, errors.New("staging_path must be a directory")
	}
	if err := os.MkdirAll(livePath, 0o755); err != nil {
		return 0, 0, err
	}

	files := 0
	var bytesSynced int64
	err = filepath.WalkDir(stagingPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(stagingPath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(livePath, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		fi, err := os.Stat(target)
		if err == nil {
			bytesSynced += fi.Size()
		}
		files++
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return files, bytesSynced, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func cloneFileSyncPipeline(in FileSyncPipeline) FileSyncPipeline {
	return in
}
