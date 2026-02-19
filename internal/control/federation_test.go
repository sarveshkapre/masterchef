package control

import "testing"

func TestFederationStorePeersAndHealth(t *testing.T) {
	store := NewFederationStore()
	peer, err := store.UpsertPeer(FederationPeerInput{
		Region:   "us-east-1",
		Endpoint: "https://cp-us.example.com",
		Mode:     "active_active",
		Weight:   100,
	})
	if err != nil {
		t.Fatalf("upsert peer failed: %v", err)
	}
	if peer.ID == "" || peer.Mode != "active_active" {
		t.Fatalf("unexpected peer %+v", peer)
	}
	updated, err := store.SetPeerHealth(peer.ID, false, 180)
	if err != nil {
		t.Fatalf("set peer health failed: %v", err)
	}
	if updated.Healthy {
		t.Fatalf("expected unhealthy peer %+v", updated)
	}
	matrix := store.HealthMatrix()
	if matrix.Healthy {
		t.Fatalf("expected unhealthy federation matrix %+v", matrix)
	}
}
