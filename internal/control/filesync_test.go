package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSyncStoreRun(t *testing.T) {
	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	live := filepath.Join(tmp, "live")
	if err := os.MkdirAll(filepath.Join(staging, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "sub", "b.txt"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewFileSyncStore()
	p, err := store.Create(FileSyncPipelineInput{
		Name:        "compiler-sync",
		StagingPath: staging,
		LivePath:    live,
		Workers:     8,
	})
	if err != nil {
		t.Fatalf("create pipeline failed: %v", err)
	}
	p, err = store.Run(p.ID)
	if err != nil {
		t.Fatalf("run pipeline failed: %v", err)
	}
	if p.FilesSynced != 2 || p.BytesSynced <= 0 {
		t.Fatalf("unexpected sync stats: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(live, "a.txt")); err != nil {
		t.Fatalf("expected live file a.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(live, "sub", "b.txt")); err != nil {
		t.Fatalf("expected live file sub/b.txt: %v", err)
	}
}
