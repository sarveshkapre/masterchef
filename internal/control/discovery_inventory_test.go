package control

import "testing"

func TestDiscoveryInventoryStoreCreateAndPrepareSync(t *testing.T) {
	store := NewDiscoveryInventoryStore()
	source, err := store.CreateSource(DiscoverySourceInput{
		Name:          "consul-main",
		Kind:          InventoryDiscoveryConsul,
		Endpoint:      "http://consul.service:8500",
		Query:         "service=payments",
		DefaultLabels: map[string]string{"team": "platform"},
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create source failed: %v", err)
	}
	if source.ID == "" || source.Kind != InventoryDiscoveryConsul {
		t.Fatalf("unexpected source %+v", source)
	}
	_, enrolls, report, err := store.PrepareSync(DiscoverySyncInput{
		SourceID: source.ID,
		Hosts: []DiscoveredHost{
			{Name: "app-1", Address: "10.0.0.1", Transport: "ssh", Labels: map[string]string{"env": "prod"}, Roles: []string{"api"}},
			{Name: "   "},
		},
	})
	if err != nil {
		t.Fatalf("prepare sync failed: %v", err)
	}
	if len(enrolls) != 1 {
		t.Fatalf("expected one valid host, got %d", len(enrolls))
	}
	if enrolls[0].Source != "discovery:consul" {
		t.Fatalf("unexpected host source %+v", enrolls[0])
	}
	if report.ValidHosts != 1 || report.RequestedHosts != 2 {
		t.Fatalf("unexpected report %+v", report)
	}
}
