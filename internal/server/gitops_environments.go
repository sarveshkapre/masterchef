package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsEnvironmentMaterializeAlias(baseDir string) http.HandlerFunc {
	h := s.handleGitOpsEnvironments(baseDir)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleGitOpsEnvironments(baseDir string) http.HandlerFunc {
	type materializeReq struct {
		Name             string `json:"name"`
		Branch           string `json:"branch"`
		SourceConfigPath string `json:"source_config_path"`
		OutputPath       string `json:"output_path,omitempty"`
		AutoEnqueue      bool   `json:"auto_enqueue,omitempty"`
		Priority         string `json:"priority,omitempty"`
		Force            bool   `json:"force,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.gitopsEnvironments.List())
		case http.MethodPost:
			var req materializeReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			name := strings.ToLower(strings.TrimSpace(req.Name))
			branch := strings.TrimSpace(req.Branch)
			source := strings.TrimSpace(req.SourceConfigPath)
			if name == "" || branch == "" || source == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, branch, and source_config_path are required"})
				return
			}
			sourceResolved := source
			if !filepath.IsAbs(sourceResolved) {
				sourceResolved = filepath.Join(baseDir, sourceResolved)
			}
			content, err := os.ReadFile(sourceResolved)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			outputPath := strings.TrimSpace(req.OutputPath)
			if outputPath == "" {
				outputPath = filepath.ToSlash(filepath.Join(".masterchef", "materialized", name+".yaml"))
			}
			outputResolved := outputPath
			if !filepath.IsAbs(outputResolved) {
				outputResolved = filepath.Join(baseDir, outputResolved)
			}
			header := "# masterchef materialized environment: " + name + "\n" +
				"# source branch: " + branch + "\n"
			materialized := header + string(content)
			if err := os.MkdirAll(filepath.Dir(outputResolved), 0o755); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.WriteFile(outputResolved, []byte(materialized), 0o644); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			sum := sha256.Sum256([]byte(materialized))
			sha := hex.EncodeToString(sum[:])

			lastJobID := ""
			if req.AutoEnqueue {
				job, err := s.queue.Enqueue(outputResolved, "gitops-env:"+name+":"+branch, req.Force, req.Priority)
				if err != nil {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error": err.Error(),
						"path":  outputPath,
					})
					return
				}
				lastJobID = job.ID
			}
			item, created, err := s.gitopsEnvironments.Upsert(control.GitOpsEnvironmentUpsert{
				Name:             name,
				Branch:           branch,
				SourceConfigPath: source,
				OutputPath:       outputPath,
				ContentSHA256:    sha,
				LastJobID:        lastJobID,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.recordEvent(control.Event{
				Type:    "gitops.environment.materialized",
				Message: "branch-per-environment config materialized",
				Fields: map[string]any{
					"environment": item.Name,
					"branch":      item.Branch,
					"output_path": item.OutputPath,
					"content_sha": item.ContentSHA256,
					"last_job_id": item.LastJobID,
					"was_created": created,
				},
			}, true)
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			writeJSON(w, status, item)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleGitOpsEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/environments/{name}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "environments" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := strings.ToLower(strings.TrimSpace(parts[3]))
	item, ok := s.gitopsEnvironments.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "gitops environment not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
