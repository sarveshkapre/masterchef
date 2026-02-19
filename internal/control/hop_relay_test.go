package control

import "testing"

func TestHopRelayStoreEndpointsAndSessions(t *testing.T) {
	store := NewHopRelayStore()
	ep, err := store.UpsertEndpoint(HopRelayEndpointInput{
		Name:        "global-hop",
		Kind:        "hop",
		Region:      "us-east-1",
		URL:         "relay.example.com:443",
		MaxSessions: 2,
	})
	if err != nil {
		t.Fatalf("upsert endpoint failed: %v", err)
	}
	if ep.ID == "" || ep.Kind != "hop" {
		t.Fatalf("unexpected endpoint %+v", ep)
	}
	sess, err := store.OpenSession(HopRelaySessionInput{
		EndpointID: ep.ID,
		NodeID:     "node-1",
		TargetHost: "db.internal:5432",
	})
	if err != nil {
		t.Fatalf("open session failed: %v", err)
	}
	if sess.ID == "" || !sess.EgressOnly {
		t.Fatalf("unexpected session %+v", sess)
	}
	if len(store.ListSessions(10)) != 1 {
		t.Fatalf("expected one session")
	}
}
