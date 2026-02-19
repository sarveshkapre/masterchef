package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) handleModuleScaffoldTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	writeJSON(w, http.StatusOK, s.moduleScaffold.List(kind))
}

func (s *Server) handleModuleScaffoldGenerate(baseDir string) http.HandlerFunc {
	type reqBody struct {
		TemplateID string `json:"template_id"`
		OutputDir  string `json:"output_dir,omitempty"`
		Overwrite  bool   `json:"overwrite,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		templateID := strings.TrimSpace(req.TemplateID)
		if templateID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "template_id is required"})
			return
		}
		item, err := s.moduleScaffold.Get(templateID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		outputDir := strings.TrimSpace(req.OutputDir)
		if outputDir == "" {
			outputDir = filepath.Join("scaffold", item.ID)
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

		paths := make([]string, 0, len(item.ScaffoldFiles))
		for rel := range item.ScaffoldFiles {
			paths = append(paths, rel)
		}
		sort.Strings(paths)
		written := make([]string, 0, len(paths))
		for _, rel := range paths {
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
		writeJSON(w, http.StatusCreated, map[string]any{
			"template":       item,
			"output_dir":     outputDir,
			"files_written":  written,
			"best_practices": item.BestPractices,
		})
	}
}
