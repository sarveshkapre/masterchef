package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleUseCaseTemplates(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		items := s.useCaseTemplates.List()
		scenario := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scenario")))
		if scenario == "" {
			writeJSON(w, http.StatusOK, items)
			return
		}
		filtered := make([]control.UseCaseTemplate, 0, len(items))
		for _, item := range items {
			if strings.EqualFold(strings.TrimSpace(item.Scenario), scenario) {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
	}
}

func (s *Server) handleUseCaseTemplateAction(baseDir string) http.HandlerFunc {
	type applyReq struct {
		OutputDir      string `json:"output_dir"`
		Overwrite      bool   `json:"overwrite"`
		CreateWorkflow bool   `json:"create_workflow"`
		CreateRunbook  bool   `json:"create_runbook"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// /v1/use-case-templates/{id}/apply
		parts := splitPath(r.URL.Path)
		if len(parts) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid use-case template action path"})
			return
		}
		id := parts[2]
		action := parts[3]
		if action != "apply" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown use-case template action"})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req applyReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}

		item, err := s.useCaseTemplates.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		outputDir := strings.TrimSpace(req.OutputDir)
		if outputDir == "" {
			outputDir = filepath.Join("use-case-templates", item.ID)
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

		scaffoldPaths := make([]string, 0, len(item.ScaffoldFiles))
		for rel := range item.ScaffoldFiles {
			scaffoldPaths = append(scaffoldPaths, rel)
		}
		sort.Strings(scaffoldPaths)
		written := make([]string, 0, len(scaffoldPaths))
		for _, rel := range scaffoldPaths {
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

		if !req.CreateWorkflow && !req.CreateRunbook {
			req.CreateWorkflow = true
		}

		templateIDs := make([]string, 0, len(item.WorkflowStepFiles))
		var createdWorkflow *control.WorkflowTemplate
		if req.CreateWorkflow {
			steps := make([]control.WorkflowStep, 0, len(item.WorkflowStepFiles))
			for idx, rel := range item.WorkflowStepFiles {
				configPath, err := resolveScaffoldPath(outputDir, rel)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				tpl := s.templates.Create(control.Template{
					Name:        fmt.Sprintf("use-case/%s/step-%02d", item.ID, idx+1),
					Description: item.Description,
					ConfigPath:  configPath,
					Defaults:    map[string]string{},
					Survey:      map[string]control.SurveyField{},
				})
				templateIDs = append(templateIDs, tpl.ID)
				steps = append(steps, control.WorkflowStep{TemplateID: tpl.ID, Priority: "normal"})
			}
			wf, err := s.workflows.Create(control.WorkflowTemplate{
				Name:        "workflow/use-case/" + item.ID,
				Description: "Generated from use-case template catalog",
				Steps:       steps,
			})
			if err == nil {
				createdWorkflow = &wf
			}
		}

		var createdRunbook *control.Runbook
		if req.CreateRunbook {
			configPath, err := resolveScaffoldPath(outputDir, item.RunbookConfigFile)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			rb, err := s.runbooks.Create(control.Runbook{
				Name:        "runbook/use-case/" + item.ID,
				Description: "Generated from use-case template catalog",
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
			"use_case_template": item,
			"output_dir":        outputDir,
			"files_written":     written,
		}
		if len(templateIDs) > 0 {
			resp["step_template_ids"] = templateIDs
		}
		if createdWorkflow != nil {
			resp["workflow"] = *createdWorkflow
		}
		if createdRunbook != nil {
			resp["runbook"] = *createdRunbook
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}
