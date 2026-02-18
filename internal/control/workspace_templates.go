package control

import (
	"errors"
	"sort"
	"strings"
)

type WorkspaceTemplate struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Pattern         string            `json:"pattern"`
	Description     string            `json:"description"`
	RecommendedTags []string          `json:"recommended_tags,omitempty"`
	ScaffoldFiles   map[string]string `json:"scaffold_files,omitempty"`
}

type WorkspaceTemplateCatalog struct {
	templates map[string]WorkspaceTemplate
}

func NewWorkspaceTemplateCatalog() *WorkspaceTemplateCatalog {
	templates := []WorkspaceTemplate{
		{
			ID:          "stateless-kubernetes-service",
			Name:        "Stateless Kubernetes Service",
			Pattern:     "stateless-services",
			Description: "Workspace scaffold for stateless microservices deployed with rolling or canary promotion gates.",
			RecommendedTags: []string{
				"stateless", "kubernetes", "canary",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# Stateless Kubernetes Service

This workspace template is tuned for stateless service release workflows.
`,
				"policy/main.yaml": `version: v0
inventory:
  hosts:
    - name: app-01
      transport: local
resources:
  - id: app-release-marker
    type: file
    host: app-01
    path: ./tmp/app-release.txt
    content: "release=1\n"
`,
				"runbooks/deploy.yaml": `name: deploy-stateless-service
description: Safe rollout for stateless workloads
steps:
  - kind: preflight
    check: health-gates
  - kind: apply
    config_path: policy/main.yaml
  - kind: verify
    check: service-readiness
`,
			},
		},
		{
			ID:          "stateful-database-cluster",
			Name:        "Stateful Database Cluster",
			Pattern:     "stateful-clusters",
			Description: "Workspace scaffold for controlled maintenance of stateful database clusters.",
			RecommendedTags: []string{
				"stateful", "database", "maintenance-window",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# Stateful Database Cluster

This workspace template is tuned for staged stateful maintenance runs.
`,
				"policy/main.yaml": `version: v0
inventory:
  hosts:
    - name: db-01
      transport: local
resources:
  - id: db-maintenance-flag
    type: file
    host: db-01
    path: ./tmp/db-maintenance.flag
    content: "enabled=true\n"
`,
				"runbooks/patch-window.yaml": `name: database-patch-window
description: Coordinated patch and reboot runbook
steps:
  - kind: preflight
    check: replication-health
  - kind: apply
    config_path: policy/main.yaml
  - kind: verify
    check: replication-caught-up
`,
			},
		},
		{
			ID:          "edge-disconnected-fleet",
			Name:        "Edge Disconnected Fleet",
			Pattern:     "edge-fleets",
			Description: "Workspace scaffold for branch/retail/edge fleets with intermittent connectivity.",
			RecommendedTags: []string{
				"edge", "disconnected", "local-first",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# Edge Disconnected Fleet

This workspace template is tuned for resilient local-first operations.
`,
				"policy/main.yaml": `version: v0
inventory:
  hosts:
    - name: edge-01
      transport: local
resources:
  - id: edge-runtime-mode
    type: file
    host: edge-01
    path: ./tmp/edge-runtime.conf
    content: "mode=disconnected-first\n"
`,
				"runbooks/reconcile.yaml": `name: edge-reconcile
description: Reconcile edge nodes after reconnect
steps:
  - kind: collect
    check: queued-drift
  - kind: apply
    config_path: policy/main.yaml
  - kind: verify
    check: sync-status
`,
			},
		},
		{
			ID:          "gpu-model-serving-fleet",
			Name:        "GPU Model Serving Fleet",
			Pattern:     "ml-serving",
			Description: "Workspace scaffold for GPU fleet provisioning and model-serving runtime tuning.",
			RecommendedTags: []string{
				"gpu", "ml-serving", "runtime-tuning",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# GPU Model Serving Fleet

This workspace template is tuned for accelerator fleet rollout and runtime policies.
`,
				"policy/main.yaml": `version: v0
inventory:
  hosts:
    - name: gpu-01
      transport: local
resources:
  - id: gpu-runtime-policy
    type: file
    host: gpu-01
    path: ./tmp/gpu-runtime.conf
    content: "max_batch_size=32\n"
`,
				"runbooks/rollout.yaml": `name: gpu-serving-rollout
description: Staged rollout for serving model updates
steps:
  - kind: preflight
    check: accelerator-health
  - kind: apply
    config_path: policy/main.yaml
  - kind: verify
    check: p99-latency
`,
			},
		},
	}

	out := map[string]WorkspaceTemplate{}
	for _, item := range templates {
		out[item.ID] = item
	}
	return &WorkspaceTemplateCatalog{templates: out}
}

func (c *WorkspaceTemplateCatalog) List() []WorkspaceTemplate {
	out := make([]WorkspaceTemplate, 0, len(c.templates))
	for _, item := range c.templates {
		out = append(out, cloneWorkspaceTemplate(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *WorkspaceTemplateCatalog) Get(id string) (WorkspaceTemplate, error) {
	id = strings.TrimSpace(id)
	item, ok := c.templates[id]
	if !ok {
		return WorkspaceTemplate{}, errors.New("workspace template not found")
	}
	return cloneWorkspaceTemplate(item), nil
}

func cloneWorkspaceTemplate(item WorkspaceTemplate) WorkspaceTemplate {
	out := item
	out.RecommendedTags = append([]string{}, item.RecommendedTags...)
	out.ScaffoldFiles = map[string]string{}
	for path, content := range item.ScaffoldFiles {
		out.ScaffoldFiles[path] = content
	}
	return out
}
