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

	lts, err := mgr.SetChannel("control-plane", "lts")
	if err != nil {
		t.Fatalf("expected lts channel to be accepted: %v", err)
	}
	if lts.Channel != "lts" {
		t.Fatalf("unexpected lts assignment: %+v", lts)
	}
}

func TestBuildSupportMatrixAndParser(t *testing.T) {
	m := BuildSupportMatrix(5)
	if m.ControlPlaneProtocol != 5 {
		t.Fatalf("expected control plane protocol 5")
	}
	if len(m.Rows) < 4 {
		t.Fatalf("expected support matrix rows")
	}
	foundLTS := false
	for _, row := range m.Rows {
		if row.Channel == "lts" {
			foundLTS = true
			if row.MinAgentProtocol != 4 || row.MaxAgentProtocol != 5 {
				t.Fatalf("unexpected lts protocol bounds: %+v", row)
			}
		}
	}
	if !foundLTS {
		t.Fatalf("expected lts row in support matrix")
	}

	if got := ParseControlPlaneProtocol("7"); got != 7 {
		t.Fatalf("expected parsed protocol 7, got %d", got)
	}
	if got := ParseControlPlaneProtocol("invalid"); got != 1 {
		t.Fatalf("expected invalid protocol to default to 1, got %d", got)
	}
}
