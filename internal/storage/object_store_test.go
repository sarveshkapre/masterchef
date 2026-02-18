package storage

import (
	"strings"
	"testing"
)

func TestLocalFSStorePutGetList(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFSStore(root)
	if err != nil {
		t.Fatalf("unexpected store init error: %v", err)
	}
	obj, err := store.Put("runs/run-1.json", []byte(`{"ok":true}`), "application/json")
	if err != nil {
		t.Fatalf("unexpected put error: %v", err)
	}
	if obj.Key != "runs/run-1.json" {
		t.Fatalf("unexpected key: %s", obj.Key)
	}
	data, info, err := store.Get("runs/run-1.json")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Fatalf("unexpected object body")
	}
	if info.SizeBytes <= 0 {
		t.Fatalf("expected non-zero object size")
	}
	items, err := store.List("runs/", 10)
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one listed object, got %d", len(items))
	}
}
