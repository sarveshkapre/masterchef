package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionRecordingStoreListAndGet(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".masterchef", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeSession := func(name, host, transport string, ts time.Time) {
		payload := map[string]any{
			"timestamp":   ts.UTC().Format(time.RFC3339Nano),
			"host":        host,
			"transport":   transport,
			"resource_id": "cmd-1",
			"command":     "id",
			"become":      true,
			"output":      "uid=0(root)",
		}
		body, _ := json.Marshal(payload)
		if err := os.WriteFile(filepath.Join(dir, name+".json"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSession("a", "host-a", "ssh", time.Now().Add(-2*time.Minute))
	writeSession("b", "host-b", "winrm", time.Now().Add(-1*time.Minute))

	store := NewSessionRecordingStore(tmp)
	all := store.List(10, "", "")
	if len(all) != 2 {
		t.Fatalf("expected 2 session recordings, got %d", len(all))
	}
	sshOnly := store.List(10, "", "ssh")
	if len(sshOnly) != 1 || sshOnly[0].Transport != "ssh" {
		t.Fatalf("expected ssh filtered sessions, got %+v", sshOnly)
	}
	item, err := store.Get("a")
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if item.ID != "a" || item.ResourceID != "cmd-1" {
		t.Fatalf("unexpected session payload %+v", item)
	}
}

func TestSessionRecordingStoreValidation(t *testing.T) {
	store := NewSessionRecordingStore(t.TempDir())
	if _, err := store.Get("../bad"); err == nil {
		t.Fatalf("expected path traversal validation error")
	}
}
