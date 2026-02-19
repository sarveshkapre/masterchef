package control

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type TaskParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Default     any    `json:"default,omitempty"`
}

type TaskDefinition struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Module      string              `json:"module"`
	Action      string              `json:"action"`
	Primitive   string              `json:"primitive"`
	Description string              `json:"description,omitempty"`
	Parameters  []TaskParameterSpec `json:"parameters,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type TaskDefinitionInput struct {
	Name        string              `json:"name"`
	Module      string              `json:"module"`
	Action      string              `json:"action"`
	Primitive   string              `json:"primitive,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  []TaskParameterSpec `json:"parameters,omitempty"`
}

type TaskPlanStep struct {
	Name            string         `json:"name"`
	TaskID          string         `json:"task_id"`
	Parameters      map[string]any `json:"parameters,omitempty"`
	ContinueOnError bool           `json:"continue_on_error,omitempty"`
}

type TaskPlan struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []TaskPlanStep `json:"steps"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type TaskPlanInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []TaskPlanStep `json:"steps"`
}

type TaskPlanPreviewInput struct {
	Overrides map[string]map[string]any `json:"overrides,omitempty"`
}

type TaskPlanPreview struct {
	PlanID string                `json:"plan_id"`
	Steps  []TaskPlanPreviewStep `json:"steps"`
}

type TaskPlanPreviewStep struct {
	Name            string         `json:"name"`
	TaskID          string         `json:"task_id"`
	Module          string         `json:"module"`
	Action          string         `json:"action"`
	Primitive       string         `json:"primitive"`
	ContinueOnError bool           `json:"continue_on_error,omitempty"`
	SensitiveFields []string       `json:"sensitive_fields,omitempty"`
	Parameters      map[string]any `json:"parameters,omitempty"`
}

type TaskFrameworkStore struct {
	mu         sync.RWMutex
	nextTaskID int64
	nextPlanID int64
	tasks      map[string]TaskDefinition
	plans      map[string]TaskPlan
}

func NewTaskFrameworkStore() *TaskFrameworkStore {
	return &TaskFrameworkStore{
		tasks: map[string]TaskDefinition{},
		plans: map[string]TaskPlan{},
	}
}

func (s *TaskFrameworkStore) RegisterTask(in TaskDefinitionInput) (TaskDefinition, error) {
	name := strings.TrimSpace(in.Name)
	module := strings.TrimSpace(in.Module)
	action := strings.TrimSpace(in.Action)
	primitive := strings.ToLower(strings.TrimSpace(in.Primitive))
	if name == "" {
		return TaskDefinition{}, errors.New("name is required")
	}
	if module == "" {
		return TaskDefinition{}, errors.New("module is required")
	}
	if action == "" {
		return TaskDefinition{}, errors.New("action is required")
	}
	if primitive == "" {
		primitive = "operation"
	}
	params, err := normalizeTaskParameterSpecs(in.Parameters)
	if err != nil {
		return TaskDefinition{}, err
	}
	now := time.Now().UTC()
	task := TaskDefinition{
		Name:        name,
		Module:      module,
		Action:      action,
		Primitive:   primitive,
		Description: strings.TrimSpace(in.Description),
		Parameters:  params,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextTaskID++
	task.ID = "task-" + itoa(s.nextTaskID)
	s.tasks[task.ID] = cloneTaskDefinition(task)
	return cloneTaskDefinition(task), nil
}

func (s *TaskFrameworkStore) ListTasks() []TaskDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaskDefinition, 0, len(s.tasks))
	for _, task := range s.tasks {
		out = append(out, cloneTaskDefinition(task))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *TaskFrameworkStore) GetTask(id string) (TaskDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[strings.TrimSpace(id)]
	if !ok {
		return TaskDefinition{}, false
	}
	return cloneTaskDefinition(task), true
}

func (s *TaskFrameworkStore) RegisterPlan(in TaskPlanInput) (TaskPlan, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return TaskPlan{}, errors.New("name is required")
	}
	if len(in.Steps) == 0 {
		return TaskPlan{}, errors.New("at least one step is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	steps := make([]TaskPlanStep, 0, len(in.Steps))
	stepNames := map[string]struct{}{}
	for i, step := range in.Steps {
		resolved, err := s.resolveStepLocked(step, i)
		if err != nil {
			return TaskPlan{}, err
		}
		if _, exists := stepNames[resolved.Name]; exists {
			return TaskPlan{}, fmt.Errorf("duplicate step name %q", resolved.Name)
		}
		stepNames[resolved.Name] = struct{}{}
		steps = append(steps, resolved)
	}

	now := time.Now().UTC()
	plan := TaskPlan{
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Steps:       steps,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.nextPlanID++
	plan.ID = "taskplan-" + itoa(s.nextPlanID)
	s.plans[plan.ID] = cloneTaskPlan(plan)
	return s.maskPlanLocked(plan), nil
}

func (s *TaskFrameworkStore) ListPlans() []TaskPlan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaskPlan, 0, len(s.plans))
	for _, plan := range s.plans {
		out = append(out, s.maskPlanLocked(plan))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *TaskFrameworkStore) GetPlan(id string) (TaskPlan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[strings.TrimSpace(id)]
	if !ok {
		return TaskPlan{}, false
	}
	return s.maskPlanLocked(plan), true
}

func (s *TaskFrameworkStore) PreviewPlan(planID string, in TaskPlanPreviewInput) (TaskPlanPreview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[strings.TrimSpace(planID)]
	if !ok {
		return TaskPlanPreview{}, errors.New("task plan not found")
	}
	overrides := map[string]map[string]any{}
	for k, v := range in.Overrides {
		overrides[strings.TrimSpace(k)] = cloneTaskAnyMap(v)
	}

	out := TaskPlanPreview{PlanID: plan.ID, Steps: make([]TaskPlanPreviewStep, 0, len(plan.Steps))}
	for _, step := range plan.Steps {
		task, ok := s.tasks[step.TaskID]
		if !ok {
			return TaskPlanPreview{}, fmt.Errorf("step %q references unknown task %q", step.Name, step.TaskID)
		}
		merged := cloneTaskAnyMap(step.Parameters)
		if ov, ok := overrides[step.Name]; ok {
			for k, v := range ov {
				merged[strings.TrimSpace(k)] = deepCopyAny(v)
			}
		}
		resolved, err := resolveTaskParameters(task, merged)
		if err != nil {
			return TaskPlanPreview{}, fmt.Errorf("step %q: %w", step.Name, err)
		}
		out.Steps = append(out.Steps, TaskPlanPreviewStep{
			Name:            step.Name,
			TaskID:          task.ID,
			Module:          task.Module,
			Action:          task.Action,
			Primitive:       task.Primitive,
			ContinueOnError: step.ContinueOnError,
			SensitiveFields: sensitiveParameterNames(task),
			Parameters:      maskTaskParameters(task, resolved),
		})
	}
	return out, nil
}

func (s *TaskFrameworkStore) resolveStepLocked(step TaskPlanStep, idx int) (TaskPlanStep, error) {
	name := strings.TrimSpace(step.Name)
	if name == "" {
		name = "step-" + itoa(int64(idx+1))
	}
	taskID := strings.TrimSpace(step.TaskID)
	if taskID == "" {
		return TaskPlanStep{}, errors.New("task_id is required")
	}
	task, ok := s.tasks[taskID]
	if !ok {
		return TaskPlanStep{}, fmt.Errorf("task %q not found", taskID)
	}
	resolvedParams, err := resolveTaskParameters(task, step.Parameters)
	if err != nil {
		return TaskPlanStep{}, fmt.Errorf("step %q: %w", name, err)
	}
	return TaskPlanStep{
		Name:            name,
		TaskID:          taskID,
		Parameters:      resolvedParams,
		ContinueOnError: step.ContinueOnError,
	}, nil
}

func (s *TaskFrameworkStore) maskPlanLocked(plan TaskPlan) TaskPlan {
	out := cloneTaskPlan(plan)
	for i := range out.Steps {
		task, ok := s.tasks[out.Steps[i].TaskID]
		if !ok {
			continue
		}
		out.Steps[i].Parameters = maskTaskParameters(task, out.Steps[i].Parameters)
	}
	return out
}

func normalizeTaskParameterSpecs(in []TaskParameterSpec) ([]TaskParameterSpec, error) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]TaskParameterSpec, 0, len(in))
	for i, raw := range in {
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			return nil, fmt.Errorf("parameters[%d].name is required", i)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate parameter name %q", name)
		}
		seen[name] = struct{}{}
		typ := strings.ToLower(strings.TrimSpace(raw.Type))
		if typ == "" {
			typ = "string"
		}
		switch typ {
		case "string", "number", "integer", "bool", "object", "array", "any":
		default:
			return nil, fmt.Errorf("parameter %q has unsupported type %q", name, typ)
		}
		spec := TaskParameterSpec{
			Name:        name,
			Type:        typ,
			Description: strings.TrimSpace(raw.Description),
			Required:    raw.Required,
			Sensitive:   raw.Sensitive,
			Default:     deepCopyAny(raw.Default),
		}
		if spec.Default != nil {
			v, err := validateAndNormalizeType(spec.Type, spec.Default)
			if err != nil {
				return nil, fmt.Errorf("parameter %q default: %w", name, err)
			}
			spec.Default = v
		}
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func resolveTaskParameters(task TaskDefinition, input map[string]any) (map[string]any, error) {
	input = cloneTaskAnyMap(input)
	allowed := map[string]TaskParameterSpec{}
	for _, spec := range task.Parameters {
		allowed[spec.Name] = spec
	}
	for k := range input {
		if _, ok := allowed[k]; !ok {
			return nil, fmt.Errorf("unknown parameter %q", k)
		}
	}

	resolved := map[string]any{}
	for _, spec := range task.Parameters {
		val, ok := input[spec.Name]
		if !ok {
			if spec.Default != nil {
				val = deepCopyAny(spec.Default)
				ok = true
			}
		}
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("missing required parameter %q", spec.Name)
			}
			continue
		}
		normalized, err := validateAndNormalizeType(spec.Type, val)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", spec.Name, err)
		}
		resolved[spec.Name] = normalized
	}
	return resolved, nil
}

func validateAndNormalizeType(typ string, val any) (any, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "string":
		if v, ok := val.(string); ok {
			return v, nil
		}
		return nil, errors.New("expected string")
	case "number":
		switch v := val.(type) {
		case float64:
			return v, nil
		case float32:
			return float64(v), nil
		case int:
			return float64(v), nil
		case int8:
			return float64(v), nil
		case int16:
			return float64(v), nil
		case int32:
			return float64(v), nil
		case int64:
			return float64(v), nil
		case uint:
			return float64(v), nil
		case uint8:
			return float64(v), nil
		case uint16:
			return float64(v), nil
		case uint32:
			return float64(v), nil
		case uint64:
			return float64(v), nil
		default:
			return nil, errors.New("expected number")
		}
	case "integer":
		switch v := val.(type) {
		case int:
			return int64(v), nil
		case int8:
			return int64(v), nil
		case int16:
			return int64(v), nil
		case int32:
			return int64(v), nil
		case int64:
			return v, nil
		case uint:
			return int64(v), nil
		case uint8:
			return int64(v), nil
		case uint16:
			return int64(v), nil
		case uint32:
			return int64(v), nil
		case uint64:
			if v > math.MaxInt64 {
				return nil, errors.New("integer overflow")
			}
			return int64(v), nil
		case float64:
			if math.Trunc(v) != v {
				return nil, errors.New("expected integer")
			}
			return int64(v), nil
		default:
			return nil, errors.New("expected integer")
		}
	case "bool":
		if v, ok := val.(bool); ok {
			return v, nil
		}
		return nil, errors.New("expected bool")
	case "object":
		if v, ok := val.(map[string]any); ok {
			return cloneTaskAnyMap(v), nil
		}
		return nil, errors.New("expected object")
	case "array":
		if v, ok := val.([]any); ok {
			return cloneTaskAnySlice(v), nil
		}
		return nil, errors.New("expected array")
	case "any":
		return deepCopyAny(val), nil
	default:
		return nil, errors.New("unsupported type")
	}
}

func maskTaskParameters(task TaskDefinition, params map[string]any) map[string]any {
	out := cloneTaskAnyMap(params)
	for _, spec := range task.Parameters {
		if !spec.Sensitive {
			continue
		}
		if _, ok := out[spec.Name]; ok {
			out[spec.Name] = "***REDACTED***"
		}
	}
	return out
}

func sensitiveParameterNames(task TaskDefinition) []string {
	out := make([]string, 0)
	for _, spec := range task.Parameters {
		if spec.Sensitive {
			out = append(out, spec.Name)
		}
	}
	sort.Strings(out)
	return out
}

func cloneTaskDefinition(in TaskDefinition) TaskDefinition {
	out := in
	out.Parameters = make([]TaskParameterSpec, 0, len(in.Parameters))
	for _, spec := range in.Parameters {
		item := spec
		item.Default = deepCopyAny(spec.Default)
		out.Parameters = append(out.Parameters, item)
	}
	return out
}

func cloneTaskPlan(in TaskPlan) TaskPlan {
	out := in
	out.Steps = make([]TaskPlanStep, 0, len(in.Steps))
	for _, step := range in.Steps {
		out.Steps = append(out.Steps, TaskPlanStep{
			Name:            step.Name,
			TaskID:          step.TaskID,
			ContinueOnError: step.ContinueOnError,
			Parameters:      cloneTaskAnyMap(step.Parameters),
		})
	}
	return out
}

func cloneTaskAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[strings.TrimSpace(k)] = deepCopyAny(v)
	}
	return out
}

func cloneTaskAnySlice(in []any) []any {
	if len(in) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, deepCopyAny(v))
	}
	return out
}

func deepCopyAny(v any) any {
	switch item := v.(type) {
	case map[string]any:
		return cloneTaskAnyMap(item)
	case []any:
		return cloneTaskAnySlice(item)
	default:
		return item
	}
}
