package control

import (
	"errors"
	"strings"
	"time"
)

type HABootstrapRequest struct {
	ClusterName  string `json:"cluster_name"`
	Region       string `json:"region"`
	Replicas     int    `json:"replicas"`
	ObjectStore  string `json:"object_store,omitempty"`
	QueueBackend string `json:"queue_backend,omitempty"`
}

type HABootstrapPlan struct {
	GeneratedAt      time.Time       `json:"generated_at"`
	ClusterName      string          `json:"cluster_name"`
	Region           string          `json:"region"`
	Replicas         int             `json:"replicas"`
	ObjectStore      string          `json:"object_store"`
	QueueBackend     string          `json:"queue_backend"`
	Command          string          `json:"command"`
	Steps            []string        `json:"steps"`
	Manifest         map[string]any  `json:"manifest"`
	ValidationChecks map[string]bool `json:"validation_checks"`
}

func BuildHABootstrapPlan(in HABootstrapRequest) (HABootstrapPlan, error) {
	cluster := strings.ToLower(strings.TrimSpace(in.ClusterName))
	region := strings.ToLower(strings.TrimSpace(in.Region))
	if cluster == "" || region == "" {
		return HABootstrapPlan{}, errors.New("cluster_name and region are required")
	}
	replicas := in.Replicas
	if replicas <= 0 {
		replicas = 3
	}
	if replicas < 3 {
		replicas = 3
	}
	objectStore := strings.ToLower(strings.TrimSpace(in.ObjectStore))
	if objectStore == "" {
		objectStore = "s3"
	}
	queue := strings.ToLower(strings.TrimSpace(in.QueueBackend))
	if queue == "" {
		queue = "postgres"
	}
	command := "masterchef bootstrap ha --cluster " + cluster + " --region " + region + " --replicas " + itoa(int64(replicas)) + " --object-store " + objectStore + " --queue " + queue
	return HABootstrapPlan{
		GeneratedAt:  time.Now().UTC(),
		ClusterName:  cluster,
		Region:       region,
		Replicas:     replicas,
		ObjectStore:  objectStore,
		QueueBackend: queue,
		Command:      command,
		Steps: []string{
			"provision regional data plane dependencies",
			"deploy control-plane replicas behind load balancer",
			"configure shared queue/object-store backends",
			"run preflight validation and health checks",
			"enable canary channel for first upgrade wave",
		},
		Manifest: map[string]any{
			"cluster":       map[string]any{"name": cluster, "region": region},
			"control_plane": map[string]any{"replicas": replicas, "strategy": "active-active"},
			"storage":       map[string]any{"object_store": objectStore, "queue_backend": queue},
		},
		ValidationChecks: map[string]bool{
			"dns":     true,
			"network": true,
			"storage": true,
			"queue":   true,
		},
	}, nil
}
