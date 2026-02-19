package control

import "testing"

func TestNodeLifecycleStore_EnrollAndTransitions(t *testing.T) {
	store := NewNodeLifecycleStore()
	node, created, err := store.Enroll(NodeEnrollInput{
		Name:      "node-1",
		Address:   "10.0.0.1",
		Transport: "SSH",
		Roles:     []string{"App", "app"},
		Labels: map[string]string{
			"Team": "platform",
		},
		Topology: map[string]string{
			"Zone": "us-east-1a",
		},
		Source: "discovery",
	})
	if err != nil {
		t.Fatalf("enroll failed: %v", err)
	}
	if !created || node.Status != NodeStatusBootstrap {
		t.Fatalf("expected created bootstrap node, got created=%t node=%+v", created, node)
	}
	if node.Transport != "ssh" || len(node.Roles) != 1 || node.Roles[0] != "app" {
		t.Fatalf("expected normalized node metadata, got %+v", node)
	}

	node, err = store.SetStatus("node-1", NodeStatusQuarantined, "failed checks")
	if err != nil {
		t.Fatalf("set status failed: %v", err)
	}
	if node.Status != NodeStatusQuarantined {
		t.Fatalf("expected quarantined status, got %+v", node)
	}
	if _, err := store.Heartbeat("node-1"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	list := store.List(NodeStatusQuarantined)
	if len(list) != 1 || list[0].Name != "node-1" {
		t.Fatalf("expected quarantined node in list, got %+v", list)
	}
}
