package control

import "testing"

func TestSyndicStoreRoutes(t *testing.T) {
	store := NewSyndicStore()
	if _, err := store.Upsert(SyndicNodeInput{Name: "master-1", Role: "master", Region: "us-east-1"}); err != nil {
		t.Fatalf("upsert master failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "syndic-a", Role: "syndic", Parent: "master-1", Segment: "dmz"}); err != nil {
		t.Fatalf("upsert syndic failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "node-1", Role: "minion", Parent: "syndic-a"}); err != nil {
		t.Fatalf("upsert minion failed: %v", err)
	}
	route, err := store.ResolveRoute("node-1")
	if err != nil {
		t.Fatalf("resolve route failed: %v", err)
	}
	if route.Hops != 2 {
		t.Fatalf("expected 2 hops, got %d", route.Hops)
	}
	if len(route.Path) != 3 || route.Path[0] != "master-1" || route.Path[2] != "node-1" {
		t.Fatalf("unexpected route path %+v", route.Path)
	}
	if len(store.List()) != 3 {
		t.Fatalf("expected 3 nodes in topology")
	}
}

func TestSyndicStoreValidation(t *testing.T) {
	store := NewSyndicStore()
	if _, err := store.Upsert(SyndicNodeInput{Name: "master-1", Role: "master"}); err != nil {
		t.Fatalf("seed master failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "child", Role: "minion"}); err == nil {
		t.Fatalf("expected missing parent error")
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "master-2", Role: "master", Parent: "master-1"}); err == nil {
		t.Fatalf("expected parent forbidden for master")
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "child", Role: "syndic", Parent: "unknown"}); err == nil {
		t.Fatalf("expected parent not found error")
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "leaf", Role: "minion", Parent: "master-1"}); err != nil {
		t.Fatalf("seed minion failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "grandchild", Role: "minion", Parent: "leaf"}); err == nil {
		t.Fatalf("expected parent minion rejection")
	}
}

func TestSyndicStoreCycleDetection(t *testing.T) {
	store := NewSyndicStore()
	if _, err := store.Upsert(SyndicNodeInput{Name: "master-1", Role: "master"}); err != nil {
		t.Fatalf("seed master failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "syndic-a", Role: "syndic", Parent: "master-1"}); err != nil {
		t.Fatalf("seed syndic failed: %v", err)
	}
	if _, err := store.Upsert(SyndicNodeInput{Name: "master-1", Role: "syndic", Parent: "syndic-a"}); err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}
	if _, err := store.ResolveRoute("syndic-a"); err == nil {
		t.Fatalf("expected cyclic topology error")
	}
}
