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
