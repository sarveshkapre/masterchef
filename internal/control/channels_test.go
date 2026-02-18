package control

import "testing"

func TestChannelManagerAndCompatibility(t *testing.T) {
	mgr := NewChannelManager()
	item, err := mgr.SetChannel("control-plane", "stable")
	if err != nil {
		t.Fatalf("unexpected set channel error: %v", err)
	}
	if item.Channel != "stable" {
		t.Fatalf("unexpected channel assignment: %+v", item)
	}
	if _, err := mgr.SetChannel("worker", "bad"); err == nil {
		t.Fatalf("expected invalid channel validation error")
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("expected one assignment")
	}

	ok := CheckNMinusOneCompatibility(5, 4)
	if !ok.Compatible {
		t.Fatalf("expected protocol compatibility for n-1")
	}
	bad := CheckNMinusOneCompatibility(5, 3)
	if bad.Compatible {
		t.Fatalf("expected protocol incompatibility beyond n-1")
	}
}
