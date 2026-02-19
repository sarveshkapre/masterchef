package release

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckToolchainPinnedMatch(t *testing.T) {
	tmp := t.TempDir()
	runtimeVer := normalizeGoVersion(runtime.Version())
	mod := "module example.com/test\n\ngo " + majorMinor(runtimeVer) + "\ntoolchain go" + runtimeVer + "\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := CheckToolchain(tmp)
	if err != nil {
		t.Fatalf("check toolchain failed: %v", err)
	}
	if !report.Pinned || !report.Match {
		t.Fatalf("expected pinned toolchain match, got %+v", report)
	}
}

func TestCheckToolchainMismatch(t *testing.T) {
	tmp := t.TempDir()
	mod := "module example.com/test\n\ngo 1.22\ntoolchain go0.0.1\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := CheckToolchain(tmp)
	if err != nil {
		t.Fatalf("check toolchain failed: %v", err)
	}
	if report.Match {
		t.Fatalf("expected mismatch for impossible toolchain, got %+v", report)
	}
}
