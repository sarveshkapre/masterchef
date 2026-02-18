package control

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNotificationRouterDispatchByRoute(t *testing.T) {
	var hits atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	router := NewNotificationRouter(100)
	target, err := router.Register(NotificationTarget{
		Name:  "incident-webhook",
		Kind:  "incident",
		URL:   receiver.URL,
		Route: "pager",
	})
	if err != nil {
		t.Fatalf("register target failed: %v", err)
	}

	del := router.NotifyAlert(AlertItem{
		ID:    "alert-1",
		Route: "pager",
	})
	if len(del) != 1 || del[0].Status != "delivered" {
		t.Fatalf("expected one delivered notification, got %+v", del)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one receiver hit")
	}

	del = router.NotifyAlert(AlertItem{
		ID:    "alert-2",
		Route: "ticket",
	})
	if len(del) != 0 {
		t.Fatalf("expected no delivery for route mismatch, got %+v", del)
	}

	updated, err := router.SetEnabled(target.ID, false)
	if err != nil {
		t.Fatalf("disable target failed: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("expected target to be disabled")
	}
	del = router.NotifyAlert(AlertItem{
		ID:    "alert-3",
		Route: "pager",
	})
	if len(del) != 0 {
		t.Fatalf("expected no delivery for disabled target")
	}
}
