package control

import "testing"

func TestEdgeRelayStoreSiteQueueAndDeliver(t *testing.T) {
	store := NewEdgeRelayStore()
	site, err := store.UpsertSite(EdgeRelaySiteInput{
		SiteID:            "edge-1",
		Region:            "ap-south-1",
		Mode:              "store_and_forward",
		MaxQueueDepth:     10,
		HeartbeatInterval: 30,
	})
	if err != nil {
		t.Fatalf("upsert site failed: %v", err)
	}
	if site.SiteID != "edge-1" || site.Mode != "store_and_forward" {
		t.Fatalf("unexpected site %+v", site)
	}
	if _, err := store.Heartbeat("edge-1"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	msg, err := store.QueueMessage(EdgeRelayMessageInput{
		SiteID:    "edge-1",
		Direction: "egress",
		Payload:   "payload",
	})
	if err != nil {
		t.Fatalf("queue message failed: %v", err)
	}
	if msg.ID == "" || msg.Status != "queued" {
		t.Fatalf("unexpected message %+v", msg)
	}
	result, err := store.Deliver("edge-1", 10)
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}
	if result.Delivered != 1 {
		t.Fatalf("expected one delivered message, got %+v", result)
	}
	messages := store.ListMessages("edge-1", 10)
	if len(messages) != 1 || messages[0].Status != "delivered" {
		t.Fatalf("unexpected relay messages %+v", messages)
	}
}
