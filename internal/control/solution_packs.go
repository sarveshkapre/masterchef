package control

import (
	"errors"
	"sort"
	"strings"
)

type SolutionPack struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Description       string   `json:"description"`
	RecommendedTags   []string `json:"recommended_tags,omitempty"`
	StarterConfigYAML string   `json:"starter_config_yaml"`
}

type SolutionPackCatalog struct {
	packs map[string]SolutionPack
}

func NewSolutionPackCatalog() *SolutionPackCatalog {
	packs := []SolutionPack{
		{
			ID:          "stateless-vm-service",
			Name:        "Stateless Service (VM)",
			Category:    "workspace-template",
			Description: "Starter for stateless service rollouts on VM fleets with safe rolling updates.",
			RecommendedTags: []string{
				"stateless", "vm", "rolling-update",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: app-01
      transport: local
resources:
  - id: deploy-artifact
    type: file
    host: app-01
    path: ./tmp/app-release.txt
    content: "service release artifact\n"
`,
		},
		{
			ID:          "stateful-postgres-cluster",
			Name:        "Stateful PostgreSQL Cluster",
			Category:    "solution-pack",
			Description: "Starter workflow for controlled stateful database maintenance and patching.",
			RecommendedTags: []string{
				"stateful", "postgres", "maintenance-window",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: db-01
      transport: local
resources:
  - id: postgres-config
    type: file
    host: db-01
    path: ./tmp/postgres.conf
    content: "max_connections=300\n"
`,
		},
		{
			ID:          "edge-disconnected-fleet",
			Name:        "Edge / Disconnected Fleet",
			Category:    "solution-pack",
			Description: "Starter for edge fleets with intermittent connectivity and local-first execution.",
			RecommendedTags: []string{
				"edge", "disconnected", "local-exec",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: edge-01
      transport: local
resources:
  - id: edge-runtime
    type: file
    host: edge-01
    path: ./tmp/edge-runtime.conf
    content: "mode=disconnected-first\n"
`,
		},
		{
			ID:          "caching-tier-redis",
			Name:        "Caching Tier (Redis/Memcached)",
			Category:    "solution-pack",
			Description: "Starter for shard-aware cache tier rollouts and operational tuning.",
			RecommendedTags: []string{
				"cache", "redis", "memcached",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: cache-01
      transport: local
resources:
  - id: cache-config
    type: file
    host: cache-01
    path: ./tmp/cache.conf
    content: "maxmemory-policy=allkeys-lru\n"
`,
		},
		{
			ID:          "messaging-platform-kafka",
			Name:        "Messaging / Streaming Platform",
			Category:    "solution-pack",
			Description: "Starter for Kafka/RabbitMQ/NATS style cluster operations and upgrades.",
			RecommendedTags: []string{
				"messaging", "streaming", "kafka",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: mq-01
      transport: local
resources:
  - id: broker-config
    type: file
    host: mq-01
    path: ./tmp/broker.conf
    content: "log.retention.hours=168\n"
`,
		},
		{
			ID:          "search-analytics-cluster",
			Name:        "Search / Analytics Cluster",
			Category:    "solution-pack",
			Description: "Starter for OpenSearch/Elasticsearch/ClickHouse cluster lifecycle operations.",
			RecommendedTags: []string{
				"search", "analytics", "cluster",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: search-01
      transport: local
resources:
  - id: search-config
    type: file
    host: search-01
    path: ./tmp/search.yml
    content: "cluster.routing.allocation.enable=all\n"
`,
		},
		{
			ID:          "observability-stack",
			Name:        "Observability Stack",
			Category:    "solution-pack",
			Description: "Starter for Prometheus/OTel collector/log pipeline control workflows.",
			RecommendedTags: []string{
				"observability", "prometheus", "otel",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: obs-01
      transport: local
resources:
  - id: otel-collector-config
    type: file
    host: obs-01
    path: ./tmp/otel-collector.yaml
    content: "receivers:\n  otlp:\n"
`,
		},
		{
			ID:          "ci-worker-fleet",
			Name:        "CI/CD Worker Fleet",
			Category:    "solution-pack",
			Description: "Starter for ephemeral build worker provisioning and lifecycle orchestration.",
			RecommendedTags: []string{
				"ci", "workers", "ephemeral",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: ci-01
      transport: local
resources:
  - id: worker-config
    type: file
    host: ci-01
    path: ./tmp/worker.conf
    content: "max_parallel_jobs=4\n"
`,
		},
		{
			ID:          "gpu-serving-fleet",
			Name:        "GPU / Accelerator Fleet",
			Category:    "solution-pack",
			Description: "Starter for GPU runtime tuning and rollout controls.",
			RecommendedTags: []string{
				"gpu", "accelerator", "serving",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: gpu-01
      transport: local
resources:
  - id: gpu-runtime
    type: file
    host: gpu-01
    path: ./tmp/gpu-runtime.conf
    content: "enable_persistence_mode=true\n"
`,
		},
		{
			ID:          "ml-training-serving",
			Name:        "ML Training / Serving",
			Category:    "solution-pack",
			Description: "Starter for ML cluster orchestration across training and serving pools.",
			RecommendedTags: []string{
				"ml", "training", "serving",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: ml-01
      transport: local
resources:
  - id: ml-orchestration
    type: file
    host: ml-01
    path: ./tmp/ml-orchestration.conf
    content: "autoscale_policy=balanced\n"
`,
		},
		{
			ID:          "batch-data-pipeline",
			Name:        "Batch / Data Pipeline Workers",
			Category:    "solution-pack",
			Description: "Starter for distributed batch worker pools and pipeline windows.",
			RecommendedTags: []string{
				"batch", "pipeline", "workers",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: batch-01
      transport: local
resources:
  - id: batch-config
    type: file
    host: batch-01
    path: ./tmp/batch.conf
    content: "queue.max_inflight=200\n"
`,
		},
		{
			ID:          "network-security-appliance",
			Name:        "Network / Security Appliance",
			Category:    "solution-pack",
			Description: "Starter for appliance configuration orchestration at scale.",
			RecommendedTags: []string{
				"network", "security", "appliance",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: sec-01
      transport: local
resources:
  - id: appliance-policy
    type: file
    host: sec-01
    path: ./tmp/appliance-policy.conf
    content: "default_action=allow\n"
`,
		},
		{
			ID:          "hybrid-migration-wave",
			Name:        "Hybrid Datacenter + Cloud Migration",
			Category:    "solution-pack",
			Description: "Starter for phased migration waves between datacenter and cloud.",
			RecommendedTags: []string{
				"hybrid", "migration", "wave",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: hybrid-01
      transport: local
resources:
  - id: migration-wave
    type: file
    host: hybrid-01
    path: ./tmp/migration-wave.conf
    content: "wave=1\n"
`,
		},
		{
			ID:          "saas-multi-tenant",
			Name:        "SaaS Multi-Tenant Operations",
			Category:    "solution-pack",
			Description: "Starter for tenant-aware operations and rollout partitioning.",
			RecommendedTags: []string{
				"saas", "multi-tenant", "tenancy",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: saas-01
      transport: local
resources:
  - id: tenancy-config
    type: file
    host: saas-01
    path: ./tmp/tenancy.conf
    content: "tenant_isolation=strict\n"
`,
		},
	}
	out := map[string]SolutionPack{}
	for _, p := range packs {
		out[p.ID] = p
	}
	return &SolutionPackCatalog{packs: out}
}

func (c *SolutionPackCatalog) List() []SolutionPack {
	out := make([]SolutionPack, 0, len(c.packs))
	for _, p := range c.packs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *SolutionPackCatalog) Get(id string) (SolutionPack, error) {
	id = strings.TrimSpace(id)
	p, ok := c.packs[id]
	if !ok {
		return SolutionPack{}, errors.New("solution pack not found")
	}
	return p, nil
}
