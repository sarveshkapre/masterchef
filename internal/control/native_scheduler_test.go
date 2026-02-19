package control

import "testing"

func TestNativeSchedulerSelectLinux(t *testing.T) {
	catalog := NewNativeSchedulerCatalog()
	result, err := catalog.Select(NativeSchedulerSelectionRequest{OSFamily: "linux", IntervalSeconds: 45, JitterSeconds: 10})
	if err != nil {
		t.Fatalf("select scheduler: %v", err)
	}
	if !result.Supported {
		t.Fatalf("expected supported result: %+v", result)
	}
	if result.Backend.Name != "systemd_timer" {
		t.Fatalf("expected systemd_timer, got %+v", result.Backend)
	}
}

func TestNativeSchedulerSelectWindows(t *testing.T) {
	catalog := NewNativeSchedulerCatalog()
	result, err := catalog.Select(NativeSchedulerSelectionRequest{OSFamily: "windows", IntervalSeconds: 120})
	if err != nil {
		t.Fatalf("select windows scheduler: %v", err)
	}
	if !result.Supported || result.Backend.Name != "windows_task_scheduler" {
		t.Fatalf("unexpected windows result %+v", result)
	}
}

func TestNativeSchedulerSelectPreferredUnsupported(t *testing.T) {
	catalog := NewNativeSchedulerCatalog()
	result, err := catalog.Select(NativeSchedulerSelectionRequest{OSFamily: "linux", IntervalSeconds: 30, PreferredBackend: "cron", JitterSeconds: 5})
	if err != nil {
		t.Fatalf("select preferred scheduler: %v", err)
	}
	if result.Supported {
		t.Fatalf("expected unsupported selection for cron+jitter")
	}
}
