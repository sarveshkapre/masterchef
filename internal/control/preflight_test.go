package control

import (
	"net"
	"testing"
)

func TestRunPreflightPassAndFail(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	pass := RunPreflight(PreflightRequest{
		TCP:                []string{ln.Addr().String()},
		StoragePaths:       []string{t.TempDir()},
		RequireObjectStore: true,
		RequireQueue:       true,
	}, NewQueue(8), true)
	if pass.Status != "pass" || pass.Failed != 0 {
		t.Fatalf("expected passing preflight, got %+v", pass)
	}

	fail := RunPreflight(PreflightRequest{
		DNS:          []string{"invalid.invalid.masterchef.local"},
		RequireQueue: true,
	}, nil, false)
	if fail.Status != "fail" || fail.Failed == 0 {
		t.Fatalf("expected failing preflight, got %+v", fail)
	}
}
