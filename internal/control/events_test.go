package control

import (
	"testing"
	"time"
)

func TestEventStore_RespectsLimit(t *testing.T) {
	s := NewEventStore(3)
	s.Append(Event{Type: "a"})
	s.Append(Event{Type: "b"})
	s.Append(Event{Type: "c"})
	s.Append(Event{Type: "d"})
	out := s.List()
	if len(out) != 3 {
		t.Fatalf("expected 3 events, got %d", len(out))
	}
	if out[0].Type != "b" || out[2].Type != "d" {
		t.Fatalf("unexpected rollover result: %#v", out)
	}
}

func TestEventStore_Replace(t *testing.T) {
	s := NewEventStore(2)
	s.Append(Event{Type: "old1"})
	s.Append(Event{Type: "old2"})
	s.Replace([]Event{{Type: "new1"}, {Type: "new2"}, {Type: "new3"}})
	out := s.List()
	if len(out) != 2 {
		t.Fatalf("expected replace to honor limit, got %d", len(out))
	}
	if out[0].Type != "new2" || out[1].Type != "new3" {
		t.Fatalf("unexpected replaced events: %#v", out)
	}
}

func TestEventStore_QueryFiltersAndOrdering(t *testing.T) {
	s := NewEventStore(10)
	base := time.Now().UTC().Add(-10 * time.Minute)
	s.Append(Event{Time: base.Add(1 * time.Minute), Type: "control.audit", Message: "user updated freeze"})
	s.Append(Event{Time: base.Add(2 * time.Minute), Type: "external.alert", Message: "disk pressure"})
	s.Append(Event{Time: base.Add(3 * time.Minute), Type: "control.audit", Message: "user approved change"})

	out := s.Query(EventQuery{
		Since:      base.Add(90 * time.Second),
		TypePrefix: "control.",
		Contains:   "approved",
		Limit:      5,
		Desc:       true,
	})
	if len(out) != 1 {
		t.Fatalf("expected one filtered event, got %d", len(out))
	}
	if out[0].Message != "user approved change" {
		t.Fatalf("unexpected filtered event: %+v", out[0])
	}
}

func TestEventStore_SubscribeReceivesEvents(t *testing.T) {
	s := NewEventStore(10)
	subID, ch := s.Subscribe(2)
	t.Cleanup(func() { s.Unsubscribe(subID) })

	s.Append(Event{Type: "test.event", Message: "first"})
	select {
	case got := <-ch:
		if got.Type != "test.event" || got.Message != "first" {
			t.Fatalf("unexpected subscribed event: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for subscribed event")
	}
}

func TestEventStore_VerifyIntegrity(t *testing.T) {
	s := NewEventStore(5)
	s.Append(Event{Type: "a", Message: "one"})
	s.Append(Event{Type: "b", Message: "two"})
	report := s.VerifyIntegrity()
	if !report.Valid {
		t.Fatalf("expected valid integrity report, got %+v", report)
	}
	if report.Checked != 2 || report.LastHash == "" {
		t.Fatalf("unexpected integrity report details: %+v", report)
	}
}

func TestEventStore_VerifyIntegrityDetectsTampering(t *testing.T) {
	s := NewEventStore(5)
	s.Append(Event{Type: "a", Message: "one"})
	s.Append(Event{Type: "b", Message: "two"})
	items := s.List()
	if len(items) != 2 {
		t.Fatalf("expected two events")
	}
	items[1].Message = "tampered"
	s.Replace(items)
	// Force tampering after re-seal by modifying hash.
	items = s.List()
	items[1].Hash = "sha256:tampered"
	s.mu.Lock()
	s.events = items
	s.mu.Unlock()

	report := s.VerifyIntegrity()
	if report.Valid {
		t.Fatalf("expected integrity verification to fail for tampered chain")
	}
	if len(report.Violations) == 0 {
		t.Fatalf("expected integrity violations, got %+v", report)
	}
}
