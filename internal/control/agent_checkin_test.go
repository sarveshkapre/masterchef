package control

import "testing"

func TestAgentCheckinDeterministicSplay(t *testing.T) {
	store := NewAgentCheckinStore()
	first, err := store.Checkin(AgentCheckinInput{
		AgentID:         "node-1",
		IntervalSeconds: 120,
		MaxSplaySeconds: 30,
	})
	if err != nil {
		t.Fatalf("first checkin failed: %v", err)
	}
	second, err := store.Checkin(AgentCheckinInput{
		AgentID:         "node-1",
		IntervalSeconds: 120,
		MaxSplaySeconds: 30,
	})
	if err != nil {
		t.Fatalf("second checkin failed: %v", err)
	}
	if first.AppliedSplaySec != second.AppliedSplaySec {
		t.Fatalf("expected deterministic splay, got %d and %d", first.AppliedSplaySec, second.AppliedSplaySec)
	}
	if first.AppliedSplaySec < 0 || first.AppliedSplaySec > 30 {
		t.Fatalf("splay out of range: %d", first.AppliedSplaySec)
	}
	list := store.List()
	if len(list) != 1 || list[0].AgentID != "node-1" {
		t.Fatalf("unexpected list output: %+v", list)
	}
}
