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

func TestScheduler_MaintenanceSkipsScheduledRuns(t *testing.T) {
	q := NewQueue(32)
	s := NewScheduler(q)
	if _, err := s.SetMaintenance("environment", "prod", true, "deploy freeze"); err != nil {
		t.Fatalf("unexpected set maintenance error: %v", err)
	}

	s.CreateWithOptions(ScheduleOptions{
		ConfigPath:  "x.yaml",
		Interval:    30 * time.Millisecond,
		Environment: "prod",
	})

	time.Sleep(120 * time.Millisecond)
	if got := len(q.List()); got != 0 {
		t.Fatalf("expected no jobs queued under maintenance, got %d", got)
	}

	if _, err := s.SetMaintenance("environment", "prod", false, ""); err != nil {
		t.Fatalf("unexpected clear maintenance error: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if len(q.List()) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected queued jobs after maintenance was disabled")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
