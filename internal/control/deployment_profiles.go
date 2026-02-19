package control

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type DeploymentProfile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Defaults    []string `json:"defaults,omitempty"`
}

type DeploymentProfileEvaluationInput struct {
	Profile            string `json:"profile"`
	ObjectStoreBackend string `json:"object_store_backend,omitempty"`
	QueueMode          string `json:"queue_mode,omitempty"`
}

type DeploymentProfileEvaluation struct {
	Profile   string    `json:"profile"`
	Pass      bool      `json:"pass"`
	Warnings  []string  `json:"warnings,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func BuiltInDeploymentProfiles() []DeploymentProfile {
	out := []DeploymentProfile{
		{
			ID:          "minimal-footprint",
			Name:        "Minimal Footprint",
			Description: "Single-binary control plane with embedded queue and local object storage defaults.",
			Defaults: []string{
				"embedded queue",
				"local filesystem object store",
				"single-binary runtime",
			},
		},
		{
			ID:          "scalable-control-plane",
			Name:        "Scalable Control Plane",
			Description: "Distributed control plane with externalized queue/object storage for high-throughput fleets.",
			Defaults: []string{
				"external queue",
				"shared object storage",
				"multi-node control-plane",
			},
		},
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func EvaluateDeploymentProfile(in DeploymentProfileEvaluationInput) (DeploymentProfileEvaluation, error) {
	profile := strings.ToLower(strings.TrimSpace(in.Profile))
	if profile == "" {
		return DeploymentProfileEvaluation{}, errors.New("profile is required")
	}
	objectStore := strings.ToLower(strings.TrimSpace(in.ObjectStoreBackend))
	queueMode := strings.ToLower(strings.TrimSpace(in.QueueMode))
	if objectStore == "" {
		objectStore = "filesystem"
	}
	if queueMode == "" {
		queueMode = "embedded"
	}
	out := DeploymentProfileEvaluation{
		Profile:   profile,
		Pass:      true,
		Warnings:  []string{},
		CheckedAt: time.Now().UTC(),
	}

	switch profile {
	case "minimal-footprint":
		if objectStore != "filesystem" && objectStore != "local" && objectStore != "fs" {
			out.Pass = false
			out.Warnings = append(out.Warnings, "minimal-footprint profile requires filesystem/local object store")
		}
		if queueMode != "embedded" {
			out.Pass = false
			out.Warnings = append(out.Warnings, "minimal-footprint profile requires embedded queue mode")
		}
	case "scalable-control-plane":
		if queueMode == "embedded" {
			out.Warnings = append(out.Warnings, "scalable-control-plane usually uses external queue")
		}
		if objectStore == "filesystem" || objectStore == "local" || objectStore == "fs" {
			out.Warnings = append(out.Warnings, "scalable-control-plane usually uses shared object storage backend")
		}
	default:
		return DeploymentProfileEvaluation{}, errors.New("unknown deployment profile")
	}
	if len(out.Warnings) == 0 {
		out.Warnings = nil
	}
	return out, nil
}
