package control

import "testing"

func TestFactRecordToGrains(t *testing.T) {
	cache := NewFactCache(0)
	item := cache.Upsert("node-1", map[string]any{"os": "linux", "meta": map[string]any{"zone": "use1a"}}, 0)
	grains := FactRecordToGrains(item)
	if grains.Node != "node-1" {
		t.Fatalf("unexpected grain node %q", grains.Node)
	}
	if grains.Grains["id"] != "node-1" {
		t.Fatalf("expected id grain to be populated")
	}
	if grains.Grains["os"] != "linux" {
		t.Fatalf("expected os grain to survive conversion")
	}
}

func TestGrainQueryToFactQuery(t *testing.T) {
	q := GrainQueryToFactQuery(GrainQueryInput{Grain: "meta.zone", Contains: "us"})
	if q.Field != "meta.zone" || q.Contains != "us" {
		t.Fatalf("unexpected grain query conversion %+v", q)
	}
	idQuery := GrainQueryToFactQuery(GrainQueryInput{Grain: "id", Equals: "node-1"})
	if idQuery.Field != "" {
		t.Fatalf("expected id grain to normalize to root field, got %q", idQuery.Field)
	}
}
