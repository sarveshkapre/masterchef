package control

import (
	"context"
	"testing"
	"time"
)

func TestWorkflowStore_LaunchAndComplete(t *testing.T) {
	q := NewQueue(32)
	exec := &fakeExecutor{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.StartWorker(ctx, exec)

	tpls := NewTemplateStore()
	t1 := tpls.Create(Template{Name: "step1", ConfigPath: "one.yaml"})
	t2 := tpls.Create(Template{Name: "step2", ConfigPath: "two.yaml"})

	ws := NewWorkflowStore(q, tpls)
	wf, err := ws.Create(WorkflowTemplate{
		Name: "deploy",
		Steps: []WorkflowStep{
			{TemplateID: t1.ID, Priority: "high"},
			{TemplateID: t2.ID, Priority: "normal"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected workflow create error: %v", err)
	}

	run, err := ws.Launch(wf.ID, "normal", false)
	if err != nil {
		t.Fatalf("unexpected workflow launch error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cur, err := ws.GetRun(run.ID)
		if err != nil {
			t.Fatalf("unexpected get run error: %v", err)
		}
		if cur.Status == WorkflowSucceeded {
			if len(cur.StepJobIDs) != 2 || cur.StepJobIDs[0] == "" || cur.StepJobIDs[1] == "" {
				t.Fatalf("expected job IDs for all steps, got %#v", cur.StepJobIDs)
			}
			break
		}
		if cur.Status == WorkflowFailed {
			t.Fatalf("expected workflow success, got failed: %s", cur.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for workflow completion")
		}
		time.Sleep(10 * time.Millisecond)
	}

	exec.mu.Lock()
	calls := exec.calls
	exec.mu.Unlock()
	if calls < 2 {
		t.Fatalf("expected at least two executor calls, got %d", calls)
	}
}

func TestWorkflowStore_FailsOnStepError(t *testing.T) {
	q := NewQueue(32)
	exec := &fakeExecutor{failOn: "bad.yaml"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.StartWorker(ctx, exec)

	tpls := NewTemplateStore()
	t1 := tpls.Create(Template{Name: "ok", ConfigPath: "ok.yaml"})
	t2 := tpls.Create(Template{Name: "bad", ConfigPath: "bad.yaml"})

	ws := NewWorkflowStore(q, tpls)
	wf, err := ws.Create(WorkflowTemplate{
		Name: "deploy",
		Steps: []WorkflowStep{
			{TemplateID: t1.ID},
			{TemplateID: t2.ID},
		},
	})
	if err != nil {
		t.Fatalf("unexpected workflow create error: %v", err)
	}

	run, err := ws.Launch(wf.ID, "normal", false)
	if err != nil {
		t.Fatalf("unexpected workflow launch error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cur, err := ws.GetRun(run.ID)
		if err != nil {
			t.Fatalf("unexpected get run error: %v", err)
		}
		if cur.Status == WorkflowFailed {
			if cur.Error == "" {
				t.Fatalf("expected failure reason")
			}
			break
		}
		if cur.Status == WorkflowSucceeded {
			t.Fatalf("expected workflow failure")
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for workflow failure")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
