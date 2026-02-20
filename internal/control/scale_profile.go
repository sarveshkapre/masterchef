package control

import "time"

type ScaleProfile struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	MinNodes            int      `json:"min_nodes"`
	MaxNodes            int      `json:"max_nodes"`
	ControlPlaneMode    string   `json:"control_plane_mode"` // single-node|ha|federated
	SchedulerStrategy   string   `json:"scheduler_strategy"`
	QueueStrategy       string   `json:"queue_strategy"`
	ExecutionPlane      string   `json:"execution_plane"`
	RecommendationHints []string `json:"recommendation_hints,omitempty"`
}

type ScaleProfileEvaluateInput struct {
	NodeCount   int `json:"node_count"`
	TenantCount int `json:"tenant_count,omitempty"`
	RegionCount int `json:"region_count,omitempty"`
	QueueDepth  int `json:"queue_depth,omitempty"`
}

type ScaleProfileEvaluation struct {
	Profile            ScaleProfile `json:"profile"`
	RecommendedShards  int          `json:"recommended_shards"`
	RecommendedWorkers int          `json:"recommended_workers"`
	Risks              []string     `json:"risks,omitempty"`
	GeneratedAt        time.Time    `json:"generated_at"`
}

func BuiltinScaleProfiles() []ScaleProfile {
	return []ScaleProfile{
		{
			ID:                "scale-small",
			Name:              "Small Fleet",
			MinNodes:          10,
			MaxNodes:          200,
			ControlPlaneMode:  "single-node",
			SchedulerStrategy: "single-queue",
			QueueStrategy:     "embedded",
			ExecutionPlane:    "integrated",
			RecommendationHints: []string{
				"use single control-plane with local object store",
				"prefer conservative concurrency with targeted applies",
			},
		},
		{
			ID:                "scale-growth",
			Name:              "Growth Fleet",
			MinNodes:          201,
			MaxNodes:          2000,
			ControlPlaneMode:  "ha",
			SchedulerStrategy: "partitioned-by-environment",
			QueueStrategy:     "priority-queues",
			ExecutionPlane:    "separated-workers",
			RecommendationHints: []string{
				"enable scheduler partitions and queue backlog SLO policies",
				"scale workers independently from control-plane nodes",
			},
		},
		{
			ID:                "scale-large",
			Name:              "Large Fleet",
			MinNodes:          2001,
			MaxNodes:          100000,
			ControlPlaneMode:  "federated",
			SchedulerStrategy: "tenant-and-region-sharded",
			QueueStrategy:     "distributed-backends",
			ExecutionPlane:    "regional-workers",
			RecommendationHints: []string{
				"use federation + regional failover drills",
				"enforce tenancy-aware scheduler partitioning and relay topology",
			},
		},
	}
}

func EvaluateScaleProfile(in ScaleProfileEvaluateInput) ScaleProfileEvaluation {
	nodes := in.NodeCount
	if nodes <= 0 {
		nodes = 10
	}
	profile := BuiltinScaleProfiles()[0]
	for _, item := range BuiltinScaleProfiles() {
		if nodes >= item.MinNodes && nodes <= item.MaxNodes {
			profile = item
			break
		}
		if nodes > item.MaxNodes {
			profile = item
		}
	}

	recommendedShards := 1
	if in.TenantCount > 20 {
		recommendedShards++
	}
	if in.RegionCount > 1 {
		recommendedShards += in.RegionCount - 1
	}
	if nodes > 2000 {
		recommendedShards += nodes / 2000
	}

	recommendedWorkers := 1
	if nodes > 0 {
		recommendedWorkers = nodes / 150
		if recommendedWorkers < 1 {
			recommendedWorkers = 1
		}
		if recommendedWorkers > 500 {
			recommendedWorkers = 500
		}
	}
	if in.QueueDepth > 1000 {
		recommendedWorkers += in.QueueDepth / 1000
	}

	risks := make([]string, 0, 4)
	if profile.ControlPlaneMode == "single-node" && nodes > 150 {
		risks = append(risks, "single-node control plane risk increases with fleet size")
	}
	if in.QueueDepth > 2000 {
		risks = append(risks, "queue depth indicates potential worker saturation")
	}
	if in.RegionCount > 3 && profile.ControlPlaneMode != "federated" {
		risks = append(risks, "multi-region topology should use federated control-plane mode")
	}

	return ScaleProfileEvaluation{
		Profile:            profile,
		RecommendedShards:  recommendedShards,
		RecommendedWorkers: recommendedWorkers,
		Risks:              risks,
		GeneratedAt:        time.Now().UTC(),
	}
}
