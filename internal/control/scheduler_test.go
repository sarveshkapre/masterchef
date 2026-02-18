package control

import (
	"testing"
	"time"
)

func TestScheduler_CreateAndList(t *testing.T) {
	q := NewQueue(32)
	s := NewScheduler(q)
	sc := s.Create("x.yaml", 50*time.Millisecond, 0)
	if sc.ID == "" {
		t.Fatalf("expected schedule id")
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected one schedule, got %d", len(list))
	}
}

func TestScheduler_EnqueueOnInterval(t *testing.T) {
	q := NewQueue(32)
	s := NewScheduler(q)
	sc := s.CreateWithPriority("x.yaml", 30*time.Millisecond, 0, "high")

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		jobs := q.List()
		if len(jobs) > 0 {
			if jobs[0].Priority != "high" {
				t.Fatalf("expected scheduled job priority high, got %s", jobs[0].Priority)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected queued jobs from schedule %s", sc.ID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
