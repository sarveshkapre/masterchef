package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleWorkspaceTemplates(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		items := s.workspaceTemplates.List()
		pattern := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("pattern")))
		if pattern == "" {
			writeJSON(w, http.StatusOK, items)
			return
		}
		filtered := make([]control.WorkspaceTemplate, 0, len(items))
		for _, item := range items {
			if strings.EqualFold(strings.TrimSpace(item.Pattern), pattern) {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
	}
}

func (s *Server) handleWorkspaceTemplateAction(baseDir string) http.HandlerFunc {
	type bootstrapReq struct {
		OutputDir      string `json:"output_dir"`
		Overwrite      bool   `json:"overwrite"`
		CreateTemplate bool   `json:"create_template"`
		CreateRunbook  bool   `json:"create_runbook"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// /v1/workspace-templates/{id}/bootstrap
		parts := splitPath(r.URL.Path)
		if len(parts) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workspace template action path"})
			return
		}
		id := parts[2]
		action := parts[3]
		if action != "bootstrap" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown workspace template action"})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req bootstrapReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}

		item, err := s.workspaceTemplates.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		outputDir := strings.TrimSpace(req.OutputDir)
		if outputDir == "" {
			outputDir = filepath.Join("workspace-templates", item.ID)
		}
		if !filepath.IsAbs(outputDir) {
			outputDir = filepath.Join(baseDir, outputDir)
		}

		if _, statErr := os.Stat(outputDir); statErr == nil && !req.Overwrite {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "output_dir already exists"})
			return
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		filePaths := make([]string, 0, len(item.ScaffoldFiles))
		for rel := range item.ScaffoldFiles {
			filePaths = append(filePaths, rel)
		}
		sort.Strings(filePaths)
		written := make([]string, 0, len(filePaths))
		for _, rel := range filePaths {
			target, err := resolveScaffoldPath(outputDir, rel)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.WriteFile(target, []byte(item.ScaffoldFiles[rel]), 0o644); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			written = append(written, target)
		}

		if !req.CreateTemplate && !req.CreateRunbook {
			req.CreateTemplate = true
		}
		configPath := filepath.Join(outputDir, "policy", "main.yaml")
		var createdTemplate *control.Template
		if req.CreateTemplate {
			tpl := s.templates.Create(control.Template{
				Name:        "workspace-template/" + item.ID,
				Description: item.Description,
				ConfigPath:  configPath,
				Defaults:    map[string]string{},
				Survey:      map[string]control.SurveyField{},
			})
			createdTemplate = &tpl
		}
		var createdRunbook *control.Runbook
		if req.CreateRunbook {
			rb, err := s.runbooks.Create(control.Runbook{
				Name:        "runbook/workspace-template/" + item.ID,
				Description: "Generated from workspace template catalog",
				TargetType:  control.RunbookTargetConfig,
				ConfigPath:  configPath,
				RiskLevel:   "medium",
				Owner:       "platform",
				Tags:        append([]string{}, item.RecommendedTags...),
			})
			if err == nil {
				approved, _ := s.runbooks.Approve(rb.ID)
				createdRunbook = &approved
			}
		}

		resp := map[string]any{
			"workspace_template": item,
			"output_dir":         outputDir,
			"files_written":      written,
		}
		if createdTemplate != nil {
			resp["template"] = *createdTemplate
		}
		if createdRunbook != nil {
			resp["runbook"] = *createdRunbook
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

func resolveScaffoldPath(outputDir, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", errors.New("scaffold file path is empty")
	}
	clean := filepath.Clean(relativePath)
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid scaffold file path")
	}
	target := filepath.Join(outputDir, clean)
	rel, err := filepath.Rel(outputDir, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("scaffold file path escapes output directory")
	}
	return target, nil
}
