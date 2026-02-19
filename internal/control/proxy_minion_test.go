package control

import "testing"

func TestProxyMinionStoreBindingAndDispatch(t *testing.T) {
	store := NewProxyMinionStore()
	binding, err := store.UpsertBinding(ProxyMinionBindingInput{
		ProxyID:   "proxy-1",
		DeviceID:  "switch-1",
		Transport: "netconf",
		Metadata:  map[string]string{"site": "dc1"},
	})
	if err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if binding.ID == "" || binding.DeviceID != "switch-1" {
		t.Fatalf("unexpected binding %+v", binding)
	}
	resolved, ok := store.ResolveDevice("switch-1")
	if !ok || resolved.ProxyID != "proxy-1" {
		t.Fatalf("resolve device failed: %+v", resolved)
	}
	rec := store.RecordDispatch(resolved, ProxyMinionDispatchRequest{
		DeviceID:   "switch-1",
		ConfigPath: "network.yaml",
	}, "queued", "job-1")
	if rec.ID == "" || rec.JobID != "job-1" {
		t.Fatalf("unexpected dispatch record %+v", rec)
	}
	if len(store.ListDispatches(10)) != 1 {
		t.Fatalf("expected one dispatch record")
	}
}
