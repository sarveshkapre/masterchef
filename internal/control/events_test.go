package control

import "testing"

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
