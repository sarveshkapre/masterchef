package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Artifact struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type SBOM struct {
	Version     string     `json:"version"`
	GeneratedAt time.Time  `json:"generated_at"`
	Artifacts   []Artifact `json:"artifacts"`
}

func GenerateSBOM(root string, files []string) (SBOM, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	if len(files) == 0 {
		return SBOM{}, errors.New("at least one file is required")
	}
	artifacts := make([]Artifact, 0, len(files))
	seen := map[string]struct{}{}
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		st, err := os.Stat(path)
		if err != nil {
			return SBOM{}, err
		}
		if st.IsDir() {
			_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					name := d.Name()
					if name == ".git" || name == ".masterchef" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if _, ok := seen[rel]; ok {
					return nil
				}
				art, err := buildArtifact(p, rel)
				if err != nil {
					return err
				}
				seen[rel] = struct{}{}
				artifacts = append(artifacts, art)
				return nil
			})
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return SBOM{}, err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := seen[rel]; ok {
			continue
		}
		art, err := buildArtifact(path, rel)
		if err != nil {
			return SBOM{}, err
		}
		seen[rel] = struct{}{}
		artifacts = append(artifacts, art)
	}
	if len(artifacts) == 0 {
		return SBOM{}, errors.New("no artifacts found")
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Path < artifacts[j].Path
	})
	return SBOM{
		Version:     "v1",
		GeneratedAt: time.Now().UTC(),
		Artifacts:   artifacts,
	}, nil
}

func SaveSBOM(path string, sbom SBOM) error {
	if path == "" {
		return errors.New("sbom path is required")
	}
	b, err := json.MarshalIndent(sbom, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadSBOM(path string) (SBOM, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SBOM{}, err
	}
	var sbom SBOM
	if err := json.Unmarshal(b, &sbom); err != nil {
		return SBOM{}, err
	}
	return sbom, nil
}

func buildArtifact(path, rel string) (Artifact, error) {
	f, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Path:   filepath.ToSlash(rel),
		Size:   n,
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func CanonicalSBOMBytes(sbom SBOM) ([]byte, error) {
	canon := sbom
	sort.Slice(canon.Artifacts, func(i, j int) bool {
		return canon.Artifacts[i].Path < canon.Artifacts[j].Path
	})
	b, err := json.Marshal(canon)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical sbom: %w", err)
	}
	return b, nil
}
