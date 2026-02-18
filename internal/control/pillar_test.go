package control

import "testing"

func TestResolvePillarMergeLast(t *testing.T) {
	res, err := ResolvePillar(PillarResolveRequest{
		Strategy: "merge-last",
		Layers: []PillarLayer{
			{
				Name: "base",
				Data: map[string]any{
					"db": map[string]any{
						"host": "db-a",
						"port": 5432,
					},
					"region": "us-east-1",
				},
			},
			{
				Name: "prod",
				Data: map[string]any{
					"db": map[string]any{
						"host": "db-prod",
					},
				},
			},
		},
		Lookup: "db.host",
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !res.Found || res.Value != "db-prod" {
		t.Fatalf("expected merge-last override, got %+v", res)
	}
	db := res.Merged["db"].(map[string]any)
	if db["port"] != float64(5432) {
		t.Fatalf("expected deep merge to preserve port, got %#v", db)
	}
}

func TestResolvePillarMergeFirstAndRemove(t *testing.T) {
	first, err := ResolvePillar(PillarResolveRequest{
		Strategy: "merge-first",
		Layers: []PillarLayer{
			{Name: "one", Data: map[string]any{"token": "a", "nested": map[string]any{"x": 1}}},
			{Name: "two", Data: map[string]any{"token": "b", "nested": map[string]any{"y": 2}}},
		},
	})
	if err != nil {
		t.Fatalf("merge-first resolve failed: %v", err)
	}
	if first.Merged["token"] != "a" {
		t.Fatalf("expected first layer token to win, got %#v", first.Merged["token"])
	}

	removed, err := ResolvePillar(PillarResolveRequest{
		Strategy: "remove",
		Layers: []PillarLayer{
			{Name: "base", Data: map[string]any{"old": "keep?", "new": "value"}},
			{Name: "cleanup", Data: map[string]any{"old": nil}},
		},
		Lookup: "old",
	})
	if err != nil {
		t.Fatalf("remove resolve failed: %v", err)
	}
	if removed.Found {
		t.Fatalf("expected removed key to be absent, got %+v", removed)
	}
}
