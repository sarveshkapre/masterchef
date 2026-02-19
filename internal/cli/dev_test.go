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
	prevRegistryBackend, hadRegistryBackend := os.LookupEnv("MC_REGISTRY_BACKEND")
	prevRegistryPath, hadRegistryPath := os.LookupEnv("MC_REGISTRY_PATH")
	prevQueueBackend, hadQueueBackend := os.LookupEnv("MC_WORKER_QUEUE_BACKEND")
	prevQueuePath, hadQueuePath := os.LookupEnv("MC_WORKER_QUEUE_PATH")
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
		if hadRegistryBackend {
			_ = os.Setenv("MC_REGISTRY_BACKEND", prevRegistryBackend)
		} else {
			_ = os.Unsetenv("MC_REGISTRY_BACKEND")
		}
		if hadRegistryPath {
			_ = os.Setenv("MC_REGISTRY_PATH", prevRegistryPath)
		} else {
			_ = os.Unsetenv("MC_REGISTRY_PATH")
		}
		if hadQueueBackend {
			_ = os.Setenv("MC_WORKER_QUEUE_BACKEND", prevQueueBackend)
		} else {
			_ = os.Unsetenv("MC_WORKER_QUEUE_BACKEND")
		}
		if hadQueuePath {
			_ = os.Setenv("MC_WORKER_QUEUE_PATH", prevQueuePath)
		} else {
			_ = os.Unsetenv("MC_WORKER_QUEUE_PATH")
		}
	})

	root := filepath.Join(t.TempDir(), "dev-runtime")
	manifest := filepath.Join(root, "runtime-manifest.json")
	if err := runDev([]string{"-state-dir", root, "-addr", ":18080", "-grpc-addr", ":19090", "-manifest", manifest, "-dry-run"}); err != nil {
		t.Fatalf("runDev dry-run failed: %v", err)
	}

	if got := os.Getenv("MC_OBJECT_STORE_BACKEND"); got != "filesystem" {
		t.Fatalf("expected filesystem backend env, got %q", got)
	}
	objectStore := os.Getenv("MC_OBJECT_STORE_PATH")
	if !strings.HasSuffix(objectStore, filepath.Join("dev-runtime", "objectstore")) {
		t.Fatalf("unexpected object store path %q", objectStore)
	}
	if got := os.Getenv("MC_REGISTRY_BACKEND"); got != "filesystem" {
		t.Fatalf("expected filesystem registry backend env, got %q", got)
	}
	if got := os.Getenv("MC_WORKER_QUEUE_BACKEND"); got != "embedded" {
		t.Fatalf("expected embedded worker queue backend env, got %q", got)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("expected state dir to exist: err=%v", err)
	}
	if st, err := os.Stat(manifest); err != nil || st.IsDir() {
		t.Fatalf("expected manifest file to exist: err=%v", err)
	}
}

func TestRunDevRequiresStateDir(t *testing.T) {
	if err := runDev([]string{"-state-dir", "", "-dry-run"}); err == nil {
		t.Fatalf("expected missing state-dir error")
	}
}
