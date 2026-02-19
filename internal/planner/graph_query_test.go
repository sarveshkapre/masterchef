package planner

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestQueryGraphUpstreamAndDownstream(t *testing.T) {
	plan := &Plan{Steps: []Step{
		{Resource: config.Resource{ID: "base"}},
		{Resource: config.Resource{ID: "app", DependsOn: []string{"base"}}},
		{Resource: config.Resource{ID: "notify", DependsOn: []string{"app"}}},
	}}

	up := QueryGraph(plan, GraphQueryRequest{ResourceID: "notify", Direction: "upstream"})
	if len(up.Upstream) != 2 || up.Upstream[0] != "app" || up.Upstream[1] != "base" {
		t.Fatalf("unexpected upstream query %+v", up)
	}
	down := QueryGraph(plan, GraphQueryRequest{ResourceID: "base", Direction: "downstream"})
	if len(down.Downstream) != 2 || down.Downstream[0] != "app" || down.Downstream[1] != "notify" {
		t.Fatalf("unexpected downstream query %+v", down)
	}
}

func TestQueryGraphDepth(t *testing.T) {
	plan := &Plan{Steps: []Step{
		{Resource: config.Resource{ID: "a"}},
		{Resource: config.Resource{ID: "b", DependsOn: []string{"a"}}},
		{Resource: config.Resource{ID: "c", DependsOn: []string{"b"}}},
	}}
	res := QueryGraph(plan, GraphQueryRequest{ResourceID: "a", Direction: "downstream", Depth: 1})
	if len(res.Downstream) != 1 || res.Downstream[0] != "b" {
		t.Fatalf("unexpected depth-limited query %+v", res)
	}
}
