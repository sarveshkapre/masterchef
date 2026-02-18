package planner

import (
	"strings"
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestToDOT(t *testing.T) {
	p := &Plan{
		Steps: []Step{
			{
				Order: 1,
				Resource: config.Resource{
					ID:   "a",
					Type: "file",
					Host: "h1",
				},
			},
			{
				Order: 2,
				Resource: config.Resource{
					ID:        "b",
					Type:      "command",
					Host:      "h1",
					DependsOn: []string{"a"},
				},
			},
		},
	}
	dot := ToDOT(p)
	if !strings.Contains(dot, "\"a\" -> \"b\";") {
		t.Fatalf("expected dependency edge in DOT:\n%s", dot)
	}
	if !strings.Contains(dot, "digraph masterchef_plan") {
		t.Fatalf("expected graph header")
	}
}
