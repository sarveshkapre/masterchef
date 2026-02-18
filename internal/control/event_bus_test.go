package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEventBusPublishAndDelivery(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	bus := NewEventBus()
	target, err := bus.Register(EventBusTarget{
		Name:    "webhook-target",
		Kind:    EventBusWebhook,
		URL:     srv.URL,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if target.ID == "" {
		t.Fatalf("expected target id")
	}

	deliveries := bus.Publish(Event{Type: "external.alert", Message: "hello"})
	if len(deliveries) != 1 || deliveries[0].Status != "delivered" {
		t.Fatalf("expected delivered status, got %#v", deliveries)
	}
	if received != 1 {
		t.Fatalf("expected webhook receive count 1, got %d", received)
	}

	_, err = bus.Register(EventBusTarget{
		Name:    "kafka-sim",
		Kind:    EventBusKafka,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("register kafka target failed: %v", err)
	}
	deliveries = bus.Publish(Event{Type: "control.audit", Message: "sim"})
	if len(deliveries) != 2 {
		t.Fatalf("expected deliveries for two targets, got %#v", deliveries)
	}
	foundQueued := false
	for _, d := range deliveries {
		if d.Kind == EventBusKafka && d.Status == "queued" {
			foundQueued = true
		}
	}
	if !foundQueued {
		t.Fatalf("expected queued kafka delivery, got %#v", deliveries)
	}
}
