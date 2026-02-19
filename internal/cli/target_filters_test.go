package cli

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

func TestFilterPlanBySelectors(t *testing.T) {
	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host: config.Host{
					Name:      "web-01",
					Transport: "local",
					Roles:     []string{"web", "frontend"},
					Labels: map[string]string{
						"team": "payments",
					},
					Topology: map[string]string{
						"zone": "us-east-1a",
					},
				},
				Resource: config.Resource{
					ID:   "res-web",
					Host: "web-01",
					Tags: []string{"app", "blue"},
				},
			},
			{
				Order: 2,
				Host: config.Host{
					Name:      "db-01",
					Transport: "local",
					Roles:     []string{"db"},
					Labels: map[string]string{
						"team": "core",
					},
					Topology: map[string]string{
						"zone": "us-east-1b",
					},
				},
				Resource: config.Resource{
					ID:   "res-db",
					Host: "db-01",
					Tags: []string{"db", "stateful"},
				},
			},
		},
	}

	out := filterPlanBySelectors(p, planSelectors{
		Hosts: parseCSVSet("web-01"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-web" {
		t.Fatalf("expected host filter to keep web step, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		IncludeTags: parseCSVSet("db"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-db" {
		t.Fatalf("expected tag include filter to keep db step, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		SkipTags: parseCSVSet("stateful,blue"),
	})
	if len(out.Steps) != 0 {
		t.Fatalf("expected skip-tags to remove both steps, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		Resources: parseCSVSet("res-db"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-db" {
		t.Fatalf("expected resource filter to keep res-db, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		Groups: parseCSVSet("role:web"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-web" {
		t.Fatalf("expected role group filter to keep web step, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		Groups: parseCSVSet("label:team=core"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-db" {
		t.Fatalf("expected label group filter to keep db step, got %#v", out.Steps)
	}

	out = filterPlanBySelectors(p, planSelectors{
		Groups: parseCSVSet("topology:zone=us-east-1a"),
	})
	if len(out.Steps) != 1 || out.Steps[0].Resource.ID != "res-web" {
		t.Fatalf("expected topology group filter to keep web step, got %#v", out.Steps)
	}
}
