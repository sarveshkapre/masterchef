package control

import (
	"errors"
	"sync"
	"time"
)

type WorkflowStatus string

const (
	WorkflowPending   WorkflowStatus = "pending"
	WorkflowRunning   WorkflowStatus = "running"
	WorkflowSucceeded WorkflowStatus = "succeeded"
	WorkflowFailed    WorkflowStatus = "failed"
)

type WorkflowStep struct {
	TemplateID string `json:"template_id"`
	Priority   string `json:"priority,omitempty"`
}

type WorkflowTemplate struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []WorkflowStep `json:"steps"`
	CreatedAt   time.Time      `json:"created_at"`
}

type WorkflowRun struct {
	ID              string         `json:"id"`
	WorkflowID      string         `json:"workflow_id"`
	Status          WorkflowStatus `json:"status"`
	CurrentStep     int            `json:"current_step"`
	TotalSteps      int            `json:"total_steps"`
	StepJobIDs      []string       `json:"step_job_ids,omitempty"`
	DefaultPriority string         `json:"default_priority"`
	Force           bool           `json:"force"`
	Error           string         `json:"error,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       time.Time      `json:"started_at,omitempty"`
	EndedAt         time.Time      `json:"ended_at,omitempty"`
}

type workflowJobRef struct {
	runID string
	step  int
}

type WorkflowStore struct {
	mu             sync.RWMutex
	nextWorkflowID int64
	nextRunID      int64
	workflows      map[string]*WorkflowTemplate
	runs           map[string]*WorkflowRun
	jobRefs        map[string]workflowJobRef
	queue          *Queue
	templates      *TemplateStore
}

func NewWorkflowStore(queue *Queue, templates *TemplateStore) *WorkflowStore {
	ws := &WorkflowStore{
		workflows: map[string]*WorkflowTemplate{},
		runs:      map[string]*WorkflowRun{},
		jobRefs:   map[string]workflowJobRef{},
		queue:     queue,
		templates: templates,
	}
	if queue != nil {
		queue.Subscribe(ws.onJob)
	}
	return ws
}

func (w *WorkflowStore) Create(in WorkflowTemplate) (WorkflowTemplate, error) {
	if in.Name == "" {
		return WorkflowTemplate{}, errors.New("workflow name is required")
	}
	if len(in.Steps) == 0 {
		return WorkflowTemplate{}, errors.New("workflow must include at least one step")
	}
	for i, step := range in.Steps {
		if step.TemplateID == "" {
			return WorkflowTemplate{}, errors.New("workflow step template_id is required")
		}
		if _, ok := w.templates.Get(step.TemplateID); !ok {
			return WorkflowTemplate{}, errors.New("workflow step references unknown template: " + step.TemplateID)
		}
		in.Steps[i].Priority = normalizePriority(step.Priority)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextWorkflowID++
	in.ID = "wf-" + itoa(w.nextWorkflowID)
	in.CreatedAt = time.Now().UTC()
	cp := cloneWorkflowTemplate(in)
	w.workflows[in.ID] = &cp
	return cp, nil
}

func (w *WorkflowStore) List() []WorkflowTemplate {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]WorkflowTemplate, 0, len(w.workflows))
	for _, wf := range w.workflows {
		out = append(out, cloneWorkflowTemplate(*wf))
	}
	return out
}

func (w *WorkflowStore) Get(id string) (WorkflowTemplate, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	wf, ok := w.workflows[id]
	if !ok {
		return WorkflowTemplate{}, false
	}
	return cloneWorkflowTemplate(*wf), true
}

func (w *WorkflowStore) Launch(workflowID, priority string, force bool) (WorkflowRun, error) {
	w.mu.Lock()
	wf, ok := w.workflows[workflowID]
	if !ok {
		w.mu.Unlock()
		return WorkflowRun{}, errors.New("workflow not found")
	}
	w.nextRunID++
	run := &WorkflowRun{
		ID:              "wfrun-" + itoa(w.nextRunID),
		WorkflowID:      workflowID,
		Status:          WorkflowPending,
		CurrentStep:     0,
		TotalSteps:      len(wf.Steps),
		StepJobIDs:      make([]string, len(wf.Steps)),
		DefaultPriority: normalizePriority(priority),
		Force:           force,
		CreatedAt:       time.Now().UTC(),
	}
	w.runs[run.ID] = run
	w.mu.Unlock()

	if err := w.dispatchStep(run.ID, 0); err != nil {
		return WorkflowRun{}, err
	}
	return w.GetRun(run.ID)
}

func (w *WorkflowStore) ListRuns() []WorkflowRun {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]WorkflowRun, 0, len(w.runs))
	for _, run := range w.runs {
		out = append(out, cloneWorkflowRun(*run))
	}
	return out
}

func (w *WorkflowStore) GetRun(id string) (WorkflowRun, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	run, ok := w.runs[id]
	if !ok {
		return WorkflowRun{}, errors.New("workflow run not found")
	}
	return cloneWorkflowRun(*run), nil
}

func (w *WorkflowStore) dispatchStep(runID string, stepIndex int) error {
	w.mu.RLock()
	run, ok := w.runs[runID]
	if !ok {
		w.mu.RUnlock()
		return errors.New("workflow run not found")
	}
	if run.Status == WorkflowFailed || run.Status == WorkflowSucceeded {
		w.mu.RUnlock()
		return nil
	}
	wf, ok := w.workflows[run.WorkflowID]
	if !ok {
		w.mu.RUnlock()
		return errors.New("workflow definition not found")
	}
	if stepIndex < 0 || stepIndex >= len(wf.Steps) {
		w.mu.RUnlock()
		return errors.New("workflow step out of range")
	}
	step := wf.Steps[stepIndex]
	priority := step.Priority
	if priority == "" || priority == "normal" {
		priority = run.DefaultPriority
	}
	force := run.Force
	runStarted := run.StartedAt
	w.mu.RUnlock()

	tpl, ok := w.templates.Get(step.TemplateID)
	if !ok {
		w.failRun(runID, "workflow step references missing template: "+step.TemplateID)
		return errors.New("workflow step references missing template")
	}

	job, err := w.queue.Enqueue(tpl.ConfigPath, runID+"-step-"+itoa(int64(stepIndex)), force, priority)
	if err != nil {
		w.failRun(runID, err.Error())
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	run, ok = w.runs[runID]
	if !ok {
		return nil
	}
	if run.Status == WorkflowFailed || run.Status == WorkflowSucceeded {
		return nil
	}
	if runStarted.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	run.Status = WorkflowRunning
	run.CurrentStep = stepIndex
	if stepIndex >= 0 && stepIndex < len(run.StepJobIDs) {
		run.StepJobIDs[stepIndex] = job.ID
	}
	w.jobRefs[job.ID] = workflowJobRef{runID: runID, step: stepIndex}
	return nil
}

func (w *WorkflowStore) onJob(job Job) {
	if job.Status != JobSucceeded && job.Status != JobFailed && job.Status != JobCanceled {
		return
	}

	w.mu.RLock()
	ref, ok := w.jobRefs[job.ID]
	if !ok {
		w.mu.RUnlock()
		return
	}
	run, ok := w.runs[ref.runID]
	if !ok {
		w.mu.RUnlock()
		return
	}
	if run.Status != WorkflowRunning && run.Status != WorkflowPending {
		w.mu.RUnlock()
		return
	}
	wf, ok := w.workflows[run.WorkflowID]
	if !ok {
		w.mu.RUnlock()
		w.failRun(ref.runID, "workflow definition not found")
		return
	}
	nextStep := ref.step + 1
	isLast := nextStep >= len(wf.Steps)
	w.mu.RUnlock()

	if job.Status == JobSucceeded {
		if isLast {
			w.mu.Lock()
			if r, ok := w.runs[ref.runID]; ok && (r.Status == WorkflowRunning || r.Status == WorkflowPending) {
				r.Status = WorkflowSucceeded
				r.CurrentStep = r.TotalSteps
				r.EndedAt = time.Now().UTC()
			}
			w.mu.Unlock()
			return
		}
		_ = w.dispatchStep(ref.runID, nextStep)
		return
	}

	w.failRun(ref.runID, "workflow step job failed: "+job.ID)
}

func (w *WorkflowStore) failRun(runID, reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	run, ok := w.runs[runID]
	if !ok {
		return
	}
	if run.Status == WorkflowSucceeded || run.Status == WorkflowFailed {
		return
	}
	run.Status = WorkflowFailed
	run.Error = reason
	run.EndedAt = time.Now().UTC()
}

func cloneWorkflowTemplate(in WorkflowTemplate) WorkflowTemplate {
	out := in
	out.Steps = append([]WorkflowStep{}, in.Steps...)
	return out
}

func cloneWorkflowRun(in WorkflowRun) WorkflowRun {
	out := in
	out.StepJobIDs = append([]string{}, in.StepJobIDs...)
	return out
}
