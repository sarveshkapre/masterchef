package storage

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ObjectInfo struct {
	Key         string    `json:"key"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Path        string    `json:"path"`
}

type ObjectStore interface {
	Put(key string, data []byte, contentType string) (ObjectInfo, error)
	Get(key string) ([]byte, ObjectInfo, error)
	List(prefix string, limit int) ([]ObjectInfo, error)
}

type LocalFSStore struct {
	root string
}

func NewObjectStoreFromEnv(baseDir string) (ObjectStore, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("MC_OBJECT_STORE_BACKEND")))
	if backend == "" {
		backend = "filesystem"
	}
	switch backend {
	case "filesystem", "fs", "local":
		root := strings.TrimSpace(os.Getenv("MC_OBJECT_STORE_PATH"))
		if root == "" {
			root = filepath.Join(baseDir, ".masterchef", "objectstore")
		}
		return NewLocalFSStore(root)
	default:
		return nil, errors.New("unsupported object store backend: " + backend)
	}
}

func NewLocalFSStore(root string) (*LocalFSStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("object store root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalFSStore{root: root}, nil
}

func (s *LocalFSStore) Put(key string, data []byte, contentType string) (ObjectInfo, error) {
	safeKey, path, err := s.resolvePath(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ObjectInfo{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return ObjectInfo{}, err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = http.DetectContentType(data)
	}
	return ObjectInfo{
		Key:         safeKey,
		SizeBytes:   st.Size(),
		ContentType: contentType,
		CreatedAt:   st.ModTime().UTC(),
		Path:        path,
	}, nil
}

func (s *LocalFSStore) Get(key string) ([]byte, ObjectInfo, error) {
	safeKey, path, err := s.resolvePath(key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	return data, ObjectInfo{
		Key:         safeKey,
		SizeBytes:   st.Size(),
		ContentType: http.DetectContentType(data),
		CreatedAt:   st.ModTime().UTC(),
		Path:        path,
	}, nil
}

func (s *LocalFSStore) List(prefix string, limit int) ([]ObjectInfo, error) {
	prefix = sanitizeKey(prefix)
	if limit <= 0 {
		limit = 200
	}
	items := make([]ObjectInfo, 0, limit)
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := sanitizeKey(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		st, err := os.Stat(path)
		if err != nil {
			return err
		}
		items = append(items, ObjectInfo{
			Key:       key,
			SizeBytes: st.Size(),
			CreatedAt: st.ModTime().UTC(),
			Path:      path,
		})
		if len(items) >= limit {
			return errors.New("list limit reached")
		}
		return nil
	})
	if err != nil && err.Error() != "list limit reached" {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	return items, nil
}

func (s *LocalFSStore) resolvePath(key string) (string, string, error) {
	safeKey := sanitizeKey(key)
	if safeKey == "" {
		return "", "", errors.New("object key is required")
	}
	if strings.Contains(safeKey, "..") {
		return "", "", errors.New("invalid object key")
	}
	path := filepath.Join(s.root, filepath.FromSlash(safeKey))
	rootClean := filepath.Clean(s.root)
	pathClean := filepath.Clean(path)
	if pathClean != rootClean && !strings.HasPrefix(pathClean, rootClean+string(filepath.Separator)) {
		return "", "", errors.New("invalid object key path")
	}
	return safeKey, pathClean, nil
}

func sanitizeKey(key string) string {
	key = strings.ReplaceAll(strings.TrimSpace(key), "\\", "/")
	key = strings.TrimPrefix(key, "/")
	key = strings.TrimSpace(key)
	key = filepath.Clean(key)
	if key == "." {
		return ""
	}
	return key
}

func TimestampedJSONKey(prefix, id string) string {
	prefix = sanitizeKey(prefix)
	id = sanitizeKey(id)
	ts := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	if prefix == "." {
		prefix = ""
	}
	if prefix == "" {
		return id + "-" + ts + ".json"
	}
	return prefix + "/" + id + "-" + ts + ".json"
}
