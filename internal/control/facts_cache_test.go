package control

import (
	"testing"
	"time"
)

func TestFactCacheTTLAndInvalidate(t *testing.T) {
	cache := NewFactCache(50 * time.Millisecond)
	item := cache.Upsert("node-a", map[string]any{"os": "linux"}, 0)
	if item.Node != "node-a" {
		t.Fatalf("expected normalized node, got %+v", item)
	}
	if _, ok := cache.Get("node-a"); !ok {
		t.Fatalf("expected cache hit")
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := cache.Get("node-a"); ok {
		t.Fatalf("expected cache miss after ttl")
	}

	cache.Upsert("node-b", map[string]any{"os": "linux"}, time.Minute)
	if !cache.Delete("node-b") {
		t.Fatalf("expected delete to return true")
	}
	if cache.Delete("node-b") {
		t.Fatalf("expected second delete to return false")
	}
}

func TestFactCacheSaltMineQuery(t *testing.T) {
	cache := NewFactCache(time.Minute)
	cache.Upsert("node-a", map[string]any{"role": "web", "meta": map[string]any{"zone": "us-east-1a"}}, 0)
	cache.Upsert("node-b", map[string]any{"role": "db", "meta": map[string]any{"zone": "us-east-1b"}}, 0)
	cache.Upsert("node-c", map[string]any{"role": "web", "meta": map[string]any{"zone": "us-west-2a"}}, 0)

	matches := cache.Query(FactCacheQuery{
		Field:    "meta.zone",
		Contains: "us-east",
		Limit:    10,
	})
	if len(matches) != 2 {
		t.Fatalf("expected two zone matches, got %#v", matches)
	}

	matches = cache.Query(FactCacheQuery{
		Field:  "role",
		Equals: "web",
		Limit:  1,
	})
	if len(matches) != 1 {
		t.Fatalf("expected limit to apply, got %#v", matches)
	}
}
