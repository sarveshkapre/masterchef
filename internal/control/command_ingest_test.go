package control

import "testing"

func TestComputeCommandChecksumDeterministic(t *testing.T) {
	a := ComputeCommandChecksum("apply", "cfg.yaml", "HIGH", "k1")
	b := ComputeCommandChecksum("apply", "cfg.yaml", "high", "k1")
	if a != b {
		t.Fatalf("expected deterministic checksum normalization")
	}
}

func TestCommandIngestStore_AcceptedAndDeadLetters(t *testing.T) {
	s := NewCommandIngestStore(2)

	first := s.RecordAccepted(CommandEnvelope{Action: "apply", IdempotencyKey: "id-1"})
	dup := s.RecordAccepted(CommandEnvelope{Action: "apply", IdempotencyKey: "id-1"})
	if first.ID != dup.ID {
		t.Fatalf("expected idempotent accepted command dedupe")
	}

	s.RecordDeadLetter(CommandEnvelope{Action: "apply"}, "bad checksum")
	s.RecordDeadLetter(CommandEnvelope{Action: "apply"}, "unsupported")
	s.RecordDeadLetter(CommandEnvelope{Action: "apply"}, "other")
	dead := s.DeadLetters()
	if len(dead) != 2 {
		t.Fatalf("expected dead letter limit to apply, got %d", len(dead))
	}
}
