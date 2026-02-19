package control

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type ImageBakeHookInput struct {
	Stage          string `json:"stage"` // pre_bake|post_bake|pre_promote|post_promote
	Action         string `json:"action"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Required       bool   `json:"required,omitempty"`
}

type ImageBakePipelineInput struct {
	Environment      string               `json:"environment"`
	Name             string               `json:"name"`
	Builder          string               `json:"builder"`
	BaseImage        string               `json:"base_image"`
	TargetImage      string               `json:"target_image"`
	ArtifactFormat   string               `json:"artifact_format,omitempty"`
	PromoteAfterBake bool                 `json:"promote_after_bake"`
	Hooks            []ImageBakeHookInput `json:"hooks,omitempty"`
}

type ImageBakeHook struct {
	Stage          string `json:"stage"`
	Action         string `json:"action"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Required       bool   `json:"required"`
}

type ImageBakePipeline struct {
	ID               string          `json:"id"`
	Environment      string          `json:"environment"`
	Name             string          `json:"name"`
	Builder          string          `json:"builder"`
	BaseImage        string          `json:"base_image"`
	TargetImage      string          `json:"target_image"`
	ArtifactFormat   string          `json:"artifact_format"`
	PromoteAfterBake bool            `json:"promote_after_bake"`
	Hooks            []ImageBakeHook `json:"hooks,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type ImageBakePlanInput struct {
	PipelineID       string `json:"pipeline_id"`
	Region           string `json:"region,omitempty"`
	BuildRef         string `json:"build_ref,omitempty"`
	PromoteAfterBake *bool  `json:"promote_after_bake,omitempty"`
}

type ImageBakePlanStep struct {
	Index          int    `json:"index"`
	Kind           string `json:"kind"` // core|hook
	Stage          string `json:"stage"`
	Action         string `json:"action"`
	Required       bool   `json:"required"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Reason         string `json:"reason"`
}

type ImageBakePlan struct {
	Allowed          bool                `json:"allowed"`
	PipelineID       string              `json:"pipeline_id,omitempty"`
	Environment      string              `json:"environment,omitempty"`
	Name             string              `json:"name,omitempty"`
	Builder          string              `json:"builder,omitempty"`
	BaseImage        string              `json:"base_image,omitempty"`
	TargetImage      string              `json:"target_image,omitempty"`
	ArtifactFormat   string              `json:"artifact_format,omitempty"`
	Region           string              `json:"region,omitempty"`
	BuildRef         string              `json:"build_ref,omitempty"`
	PromoteAfterBake bool                `json:"promote_after_bake"`
	Steps            []ImageBakePlanStep `json:"steps,omitempty"`
	BlockedReason    string              `json:"blocked_reason,omitempty"`
}

type ImageBakeStore struct {
	mu        sync.RWMutex
	nextID    int64
	pipelines map[string]*ImageBakePipeline
}

func NewImageBakeStore() *ImageBakeStore {
	return &ImageBakeStore{pipelines: map[string]*ImageBakePipeline{}}
}

func (s *ImageBakeStore) Create(in ImageBakePipelineInput) (ImageBakePipeline, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	name := strings.TrimSpace(in.Name)
	builder := strings.ToLower(strings.TrimSpace(in.Builder))
	baseImage := strings.TrimSpace(in.BaseImage)
	targetImage := strings.TrimSpace(in.TargetImage)
	if environment == "" || name == "" || builder == "" || baseImage == "" || targetImage == "" {
		return ImageBakePipeline{}, errors.New("environment, name, builder, base_image, and target_image are required")
	}
	artifactFormat := strings.ToLower(strings.TrimSpace(in.ArtifactFormat))
	if artifactFormat == "" {
		artifactFormat = "raw"
	}
	hooks, err := normalizeImageBakeHooks(in.Hooks)
	if err != nil {
		return ImageBakePipeline{}, err
	}

	item := ImageBakePipeline{
		Environment:      environment,
		Name:             name,
		Builder:          builder,
		BaseImage:        baseImage,
		TargetImage:      targetImage,
		ArtifactFormat:   artifactFormat,
		PromoteAfterBake: in.PromoteAfterBake,
		Hooks:            hooks,
		CreatedAt:        time.Now().UTC(),
	}

	s.mu.Lock()
	s.nextID++
	item.ID = "image-bake-pipeline-" + itoa(s.nextID)
	s.pipelines[item.ID] = &item
	s.mu.Unlock()

	return item, nil
}

func (s *ImageBakeStore) List() []ImageBakePipeline {
	s.mu.RLock()
	out := make([]ImageBakePipeline, 0, len(s.pipelines))
	for _, pipeline := range s.pipelines {
		out = append(out, *pipeline)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ImageBakeStore) Get(id string) (ImageBakePipeline, bool) {
	s.mu.RLock()
	item, ok := s.pipelines[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ImageBakePipeline{}, false
	}
	return *item, true
}

func (s *ImageBakeStore) Plan(in ImageBakePlanInput) (ImageBakePlan, error) {
	id := strings.TrimSpace(in.PipelineID)
	if id == "" {
		return ImageBakePlan{}, errors.New("pipeline_id is required")
	}
	pipeline, ok := s.Get(id)
	if !ok {
		return ImageBakePlan{}, errors.New("image bake pipeline not found")
	}
	promote := pipeline.PromoteAfterBake
	if in.PromoteAfterBake != nil {
		promote = *in.PromoteAfterBake
	}

	plan := ImageBakePlan{
		Allowed:          true,
		PipelineID:       pipeline.ID,
		Environment:      pipeline.Environment,
		Name:             pipeline.Name,
		Builder:          pipeline.Builder,
		BaseImage:        pipeline.BaseImage,
		TargetImage:      pipeline.TargetImage,
		ArtifactFormat:   pipeline.ArtifactFormat,
		Region:           strings.TrimSpace(in.Region),
		BuildRef:         strings.TrimSpace(in.BuildRef),
		PromoteAfterBake: promote,
	}

	stepIndex := 1
	stepIndex = appendImageBakeHookSteps(&plan.Steps, stepIndex, "pre_bake", pipeline.Hooks)
	plan.Steps = append(plan.Steps, ImageBakePlanStep{
		Index:    stepIndex,
		Kind:     "core",
		Stage:    "bake",
		Action:   "build-image",
		Required: true,
		Reason:   "produce immutable golden image from base image input",
	})
	stepIndex++
	stepIndex = appendImageBakeHookSteps(&plan.Steps, stepIndex, "post_bake", pipeline.Hooks)

	if promote {
		stepIndex = appendImageBakeHookSteps(&plan.Steps, stepIndex, "pre_promote", pipeline.Hooks)
		plan.Steps = append(plan.Steps, ImageBakePlanStep{
			Index:    stepIndex,
			Kind:     "core",
			Stage:    "promote",
			Action:   "promote-image",
			Required: true,
			Reason:   "promote baked image into consumption channels",
		})
		stepIndex++
		appendImageBakeHookSteps(&plan.Steps, stepIndex, "post_promote", pipeline.Hooks)
	}

	if len(plan.Steps) == 0 {
		plan.Allowed = false
		plan.BlockedReason = "no executable image bake steps were generated"
	}
	return plan, nil
}

func appendImageBakeHookSteps(out *[]ImageBakePlanStep, start int, stage string, hooks []ImageBakeHook) int {
	index := start
	for _, hook := range hooks {
		if hook.Stage != stage {
			continue
		}
		*out = append(*out, ImageBakePlanStep{
			Index:          index,
			Kind:           "hook",
			Stage:          stage,
			Action:         hook.Action,
			Required:       hook.Required,
			TimeoutSeconds: hook.TimeoutSeconds,
			Reason:         "pipeline hook executes custom validation or promotion logic",
		})
		index++
	}
	return index
}

func normalizeImageBakeHooks(in []ImageBakeHookInput) ([]ImageBakeHook, error) {
	if len(in) == 0 {
		return nil, nil
	}
	type item struct {
		hook ImageBakeHook
		pos  int
	}
	out := make([]item, 0, len(in))
	for i, raw := range in {
		stage, ok := normalizeImageBakeStage(raw.Stage)
		if !ok {
			return nil, fmt.Errorf("invalid hook stage %q", raw.Stage)
		}
		action := strings.TrimSpace(raw.Action)
		if action == "" {
			return nil, errors.New("hook action is required")
		}
		timeout := raw.TimeoutSeconds
		if timeout <= 0 {
			timeout = 300
		}
		out = append(out, item{
			hook: ImageBakeHook{
				Stage:          stage,
				Action:         action,
				TimeoutSeconds: timeout,
				Required:       raw.Required,
			},
			pos: i,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri := imageBakeStageRank(out[i].hook.Stage)
		rj := imageBakeStageRank(out[j].hook.Stage)
		if ri == rj {
			return out[i].pos < out[j].pos
		}
		return ri < rj
	})
	normalized := make([]ImageBakeHook, 0, len(out))
	for _, item := range out {
		normalized = append(normalized, item.hook)
	}
	return normalized, nil
}

func normalizeImageBakeStage(in string) (string, bool) {
	stage := strings.ToLower(strings.TrimSpace(in))
	stage = strings.ReplaceAll(stage, "-", "_")
	stage = strings.ReplaceAll(stage, " ", "_")
	switch stage {
	case "pre_bake", "post_bake", "pre_promote", "post_promote":
		return stage, true
	default:
		return "", false
	}
}

func imageBakeStageRank(stage string) int {
	switch stage {
	case "pre_bake":
		return 1
	case "post_bake":
		return 2
	case "pre_promote":
		return 3
	case "post_promote":
		return 4
	default:
		return 99
	}
}
