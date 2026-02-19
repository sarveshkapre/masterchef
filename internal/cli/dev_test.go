package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDevDryRunPreparesLocalRuntime(t *testing.T) {
	prevBackend, hadBackend := os.LookupEnv("MC_OBJECT_STORE_BACKEND")
	prevPath, hadPath := os.LookupEnv("MC_OBJECT_STORE_PATH")
	t.Cleanup(func() {
		if hadBackend {
			_ = os.Setenv("MC_OBJECT_STORE_BACKEND", prevBackend)
		} else {
			_ = os.Unsetenv("MC_OBJECT_STORE_BACKEND")
		}
		if hadPath {
			_ = os.Setenv("MC_OBJECT_STORE_PATH", prevPath)
		} else {
			_ = os.Unsetenv("MC_OBJECT_STORE_PATH")
		}
	})

	root := filepath.Join(t.TempDir(), "dev-runtime")
	if err := runDev([]string{"-state-dir", root, "-addr", ":18080", "-grpc-addr", ":19090", "-dry-run"}); err != nil {
		t.Fatalf("runDev dry-run failed: %v", err)
	}

	if got := os.Getenv("MC_OBJECT_STORE_BACKEND"); got != "filesystem" {
		t.Fatalf("expected filesystem backend env, got %q", got)
	}
	objectStore := os.Getenv("MC_OBJECT_STORE_PATH")
	if !strings.HasSuffix(objectStore, filepath.Join("dev-runtime", "objectstore")) {
		t.Fatalf("unexpected object store path %q", objectStore)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("expected state dir to exist: err=%v", err)
	}
}

func TestRunDevRequiresStateDir(t *testing.T) {
	if err := runDev([]string{"-state-dir", "", "-dry-run"}); err == nil {
		t.Fatalf("expected missing state-dir error")
	}
}
