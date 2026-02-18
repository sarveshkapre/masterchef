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
