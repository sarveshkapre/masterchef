package control

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWebhookDispatcher_RegisterAndDispatch(t *testing.T) {
	d := NewWebhookDispatcher(100)
	var calls int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("X-Masterchef-Event-Type") == "" {
			t.Fatalf("missing event type header")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	wh, err := d.Register(WebhookSubscription{
		Name:        "alerts",
		URL:         receiver.URL,
		EventPrefix: "external.",
		Secret:      "secret",
	})
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	if wh.ID == "" {
		t.Fatalf("expected webhook id")
	}

	deliveries := d.Dispatch(Event{Type: "external.alert", Fields: map[string]any{"sev": "high"}})
	if len(deliveries) != 1 || deliveries[0].Status != "delivered" {
		t.Fatalf("expected one successful delivery, got %#v", deliveries)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one receiver call")
	}

	if _, err := d.SetEnabled(wh.ID, false); err != nil {
		t.Fatalf("unexpected set enabled error: %v", err)
	}
	deliveries = d.Dispatch(Event{Type: "external.alert"})
	if len(deliveries) != 0 {
		t.Fatalf("expected no delivery while disabled")
	}
}

func TestWebhookDispatcher_FailureDeliveryRecorded(t *testing.T) {
	d := NewWebhookDispatcher(100)
	_, err := d.Register(WebhookSubscription{
		Name:        "broken",
		URL:         "http://127.0.0.1:1",
		EventPrefix: "external.",
	})
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	deliveries := d.Dispatch(Event{Type: "external.alert"})
	if len(deliveries) != 1 || deliveries[0].Status != "failed" {
		t.Fatalf("expected one failed delivery, got %#v", deliveries)
	}
	if len(d.Deliveries(10)) == 0 {
		t.Fatalf("expected persisted delivery history")
	}
}
