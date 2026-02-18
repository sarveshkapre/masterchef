package planner

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestAnalyzeBlastRadius(t *testing.T) {
	p := &Plan{
		Steps: []Step{
			{Order: 1, Resource: config.Resource{ID: "a", Host: "h1", Type: "file"}},
			{Order: 2, Resource: config.Resource{ID: "b", Host: "h2", Type: "command"}},
			{Order: 3, Resource: config.Resource{ID: "c", Host: "h2", Type: "file"}},
		},
	}
	br := AnalyzeBlastRadius(p)
	if br.TotalSteps != 3 {
		t.Fatalf("unexpected total steps: %d", br.TotalSteps)
	}
	if len(br.AffectedHosts) != 2 || len(br.AffectedTypes) != 2 {
		t.Fatalf("unexpected blast radius: %+v", br)
	}
	if br.MaxOrder != 3 {
		t.Fatalf("unexpected max order: %d", br.MaxOrder)
	}
}
