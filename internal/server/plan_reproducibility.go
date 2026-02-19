package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/planner"
)

type planReproducibilityRunnerInput struct {
	Runner   string `json:"runner"`
	PlanPath string `json:"plan_path"`
}

type planReproducibilityRunnerResult struct {
	Runner   string                   `json:"runner"`
	PlanPath string                   `json:"plan_path"`
	Match    bool                     `json:"match"`
	Diff     planner.PlanSnapshotDiff `json:"diff"`
}

func (s *Server) handlePlanReproducibility(baseDir string) http.HandlerFunc {
	type reqBody struct {
		BaselinePath string                           `json:"baseline_path"`
		RunnerPlans  []planReproducibilityRunnerInput `json:"runner_plans"`
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
		baselinePath := strings.TrimSpace(req.BaselinePath)
		if baselinePath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseline_path is required"})
			return
		}
		if len(req.RunnerPlans) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_plans must include at least one plan artifact"})
			return
		}
		baselinePlan, resolvedBaseline, err := loadPlanArtifact(baseDir, baselinePath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		results := make([]planReproducibilityRunnerResult, 0, len(req.RunnerPlans))
		reproducible := true
		for i, item := range req.RunnerPlans {
			runner := strings.TrimSpace(item.Runner)
			if runner == "" {
				runner = fmt.Sprintf("runner-%d", i+1)
			}
			currentPlan, resolvedPath, err := loadPlanArtifact(baseDir, item.PlanPath)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			diff := planner.DiffPlans(baselinePlan, currentPlan)
			results = append(results, planReproducibilityRunnerResult{
				Runner:   runner,
				PlanPath: resolvedPath,
				Match:    diff.Match,
				Diff:     diff,
			})
			if !diff.Match {
				reproducible = false
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"baseline_path": resolvedBaseline,
			"reproducible":  reproducible,
			"runner_count":  len(results),
			"results":       results,
		})
	}
}

func loadPlanArtifact(baseDir, path string) (*planner.Plan, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", fmt.Errorf("plan_path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read plan artifact %q: %w", path, err)
	}
	var snap planner.PlanSnapshot
	if err := json.Unmarshal(raw, &snap); err == nil && snap.Plan != nil {
		return snap.Plan, path, nil
	}
	var plan planner.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, "", fmt.Errorf("parse plan artifact %q: %w", path, err)
	}
	return &plan, path, nil
}
