package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorkflowWizardStep struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	RequiredInputs []string `json:"required_inputs,omitempty"`
	ActionHint     string   `json:"action_hint,omitempty"`
}

type WorkflowWizard struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	UseCase     string               `json:"use_case"`
	Steps       []WorkflowWizardStep `json:"steps"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

type WorkflowWizardLaunchInput struct {
	WizardID string            `json:"wizard_id,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
	DryRun   bool              `json:"dry_run,omitempty"`
}

type WorkflowWizardLaunchResult struct {
	WizardID      string   `json:"wizard_id"`
	Ready         bool     `json:"ready"`
	MissingInputs []string `json:"missing_inputs,omitempty"`
	NextStepID    string   `json:"next_step_id,omitempty"`
	Preview       []string `json:"preview,omitempty"`
	DryRun        bool     `json:"dry_run"`
}

type WorkflowWizardCatalog struct {
	mu      sync.RWMutex
	wizards map[string]*WorkflowWizard
}

func NewWorkflowWizardCatalog() *WorkflowWizardCatalog {
	now := time.Now().UTC()
	items := []WorkflowWizard{
		{
			ID:          "bootstrap",
			Title:       "Bootstrap Control Plane",
			Description: "Guide first-time environment bootstrap with dependency checks and template setup.",
			UseCase:     "bootstrap",
			UpdatedAt:   now,
			Steps: []WorkflowWizardStep{
				{ID: "validate-topology", Title: "Validate Topology", Description: "Validate DNS/network/storage prerequisites for control-plane deployment.", RequiredInputs: []string{"environment", "region"}, ActionHint: "POST /v1/control/bootstrap/ha"},
				{ID: "select-template", Title: "Select Workspace Template", Description: "Choose opinionated workspace template for fleet type.", RequiredInputs: []string{"workspace_template"}, ActionHint: "GET /v1/workspace-templates"},
				{ID: "generate-runbook", Title: "Generate Bootstrap Runbook", Description: "Generate runbook/checklist for handoff.", ActionHint: "POST /v1/runbooks"},
			},
		},
		{
			ID:          "rollout",
			Title:       "Service Rollout",
			Description: "Guide safe rollout with risk checks, canary gates, and staged promotion.",
			UseCase:     "rollout",
			UpdatedAt:   now,
			Steps: []WorkflowWizardStep{
				{ID: "preflight", Title: "Preflight Validation", Description: "Run policy simulation and preflight checks for candidate change.", RequiredInputs: []string{"config_path"}, ActionHint: "POST /v1/policy/simulate"},
				{ID: "start-deployment", Title: "Start Deployment", Description: "Create deployment rollout with strategy and concurrency guards.", RequiredInputs: []string{"strategy", "target_environment"}, ActionHint: "POST /v1/deployments"},
				{ID: "gate-promotion", Title: "Gate Promotion", Description: "Evaluate health probes and disruption budgets before promotion.", ActionHint: "POST /v1/control/health-probes/evaluate"},
			},
		},
		{
			ID:          "rollback",
			Title:       "Failure Rollback",
			Description: "Guide failure recovery with triage artifacts, safe rollback, and verification checks.",
			UseCase:     "rollback",
			UpdatedAt:   now,
			Steps: []WorkflowWizardStep{
				{ID: "capture-triage", Title: "Capture Triage Bundle", Description: "Export failure context and dependency impacts.", RequiredInputs: []string{"run_id"}, ActionHint: "POST /v1/runs/{id}/triage-bundle"},
				{ID: "execute-rollback", Title: "Execute Rollback", Description: "Run scoped rollback and enforce checkpoint resume safety.", RequiredInputs: []string{"rollback_config_path"}, ActionHint: "POST /v1/runs/{id}/rollback"},
				{ID: "post-verify", Title: "Post-Change Verification", Description: "Validate invariants, health probes, and drift state.", ActionHint: "POST /v1/invariants/evaluate"},
			},
		},
		{
			ID:          "incident-remediation",
			Title:       "Incident Remediation",
			Description: "Guide active incident handling with command guardrails, approvals, and handoff generation.",
			UseCase:     "incident-remediation",
			UpdatedAt:   now,
			Steps: []WorkflowWizardStep{
				{ID: "collect-signals", Title: "Collect Incident Signals", Description: "Load incident view and correlate runs, alerts, and health.", RequiredInputs: []string{"workload"}, ActionHint: "GET /v1/incidents/view"},
				{ID: "approved-action", Title: "Execute Approved Action", Description: "Run approved runbook/task with checklist and access controls.", RequiredInputs: []string{"runbook_id"}, ActionHint: "POST /v1/runbooks/{id}/launch"},
				{ID: "handoff", Title: "Generate Handoff Package", Description: "Create on-call handoff package with active risks and blockers.", ActionHint: "GET /v1/control/handoff"},
			},
		},
	}

	index := map[string]*WorkflowWizard{}
	for _, item := range items {
		copied := cloneWorkflowWizard(item)
		index[copied.ID] = &copied
	}
	return &WorkflowWizardCatalog{wizards: index}
}

func (c *WorkflowWizardCatalog) List() []WorkflowWizard {
	c.mu.RLock()
	out := make([]WorkflowWizard, 0, len(c.wizards))
	for _, item := range c.wizards {
		out = append(out, cloneWorkflowWizard(*item))
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *WorkflowWizardCatalog) Get(id string) (WorkflowWizard, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WorkflowWizard{}, errors.New("wizard id is required")
	}
	c.mu.RLock()
	item, ok := c.wizards[id]
	c.mu.RUnlock()
	if !ok {
		return WorkflowWizard{}, errors.New("wizard not found")
	}
	return cloneWorkflowWizard(*item), nil
}

func (c *WorkflowWizardCatalog) Launch(id string, in WorkflowWizardLaunchInput) (WorkflowWizardLaunchResult, error) {
	wizard, err := c.Get(id)
	if err != nil {
		return WorkflowWizardLaunchResult{}, err
	}
	inputs := map[string]string{}
	for key, value := range in.Inputs {
		k := strings.TrimSpace(strings.ToLower(key))
		if k == "" {
			continue
		}
		inputs[k] = strings.TrimSpace(value)
	}
	missing := map[string]struct{}{}
	next := ""
	for _, step := range wizard.Steps {
		for _, required := range step.RequiredInputs {
			k := strings.TrimSpace(strings.ToLower(required))
			if k == "" {
				continue
			}
			if strings.TrimSpace(inputs[k]) == "" {
				missing[k] = struct{}{}
				if next == "" {
					next = step.ID
				}
			}
		}
	}
	missingList := make([]string, 0, len(missing))
	for key := range missing {
		missingList = append(missingList, key)
	}
	sort.Strings(missingList)

	preview := make([]string, 0, len(wizard.Steps))
	for _, step := range wizard.Steps {
		preview = append(preview, step.ID+": "+step.Title)
	}

	return WorkflowWizardLaunchResult{
		WizardID:      wizard.ID,
		Ready:         len(missingList) == 0,
		MissingInputs: missingList,
		NextStepID:    next,
		Preview:       preview,
		DryRun:        in.DryRun,
	}, nil
}

func cloneWorkflowWizard(in WorkflowWizard) WorkflowWizard {
	out := in
	out.Steps = make([]WorkflowWizardStep, 0, len(in.Steps))
	for _, step := range in.Steps {
		copied := step
		copied.RequiredInputs = append([]string{}, step.RequiredInputs...)
		out.Steps = append(out.Steps, copied)
	}
	return out
}
