package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/state"
	"github.com/masterchef/masterchef/internal/storage"
)

type Server struct {
	httpServer  *http.Server
	baseDir     string
	queue       *control.Queue
	scheduler   *control.Scheduler
	templates   *control.TemplateStore
	workflows   *control.WorkflowStore
	assocs      *control.AssociationStore
	commands    *control.CommandIngestStore
	canaries    *control.CanaryStore
	rules       *control.RuleEngine
	webhooks    *control.WebhookDispatcher
	channels    *control.ChannelManager
	schemaMigs  *control.SchemaMigrationManager
	objectStore storage.ObjectStore
	events      *control.EventStore
	runCancel   context.CancelFunc
	metricsMu   sync.Mutex
	metrics     map[string]int64

	backlogThreshold  int
	backlogSamples    []backlogSample
	backlogWarnActive bool
	backlogSatActive  bool
}

type backlogSample struct {
	at      time.Time
	pending int
}

func New(addr, baseDir string) *Server {
	runner := control.NewRunner(baseDir)
	queue := control.NewQueue(512)
	runCtx, runCancel := context.WithCancel(context.Background())
	queue.StartWorker(runCtx, runner)
	scheduler := control.NewScheduler(queue)
	templates := control.NewTemplateStore()
	workflows := control.NewWorkflowStore(queue, templates)
	assocs := control.NewAssociationStore(scheduler)
	commands := control.NewCommandIngestStore(5000)
	canaries := control.NewCanaryStore(queue)
	rules := control.NewRuleEngine()
	webhooks := control.NewWebhookDispatcher(5000)
	channels := control.NewChannelManager()
	schemaMigs := control.NewSchemaMigrationManager(1)
	objectStore, err := storage.NewObjectStoreFromEnv(baseDir)
	if err != nil {
		// Fallback to local filesystem object store under workspace state.
		fallback, fallbackErr := storage.NewLocalFSStore(filepath.Join(baseDir, ".masterchef", "objectstore"))
		if fallbackErr == nil {
			objectStore = fallback
		}
	}
	events := control.NewEventStore(20_000)

	mux := http.NewServeMux()
	s := &Server{
		baseDir:     baseDir,
		queue:       queue,
		scheduler:   scheduler,
		templates:   templates,
		workflows:   workflows,
		assocs:      assocs,
		commands:    commands,
		canaries:    canaries,
		rules:       rules,
		webhooks:    webhooks,
		channels:    channels,
		schemaMigs:  schemaMigs,
		objectStore: objectStore,
		events:      events,
		metrics:     map[string]int64{},
		runCancel:   runCancel,
		backlogThreshold: readIntEnv(
			"MC_QUEUE_BACKLOG_SLO_THRESHOLD",
			100,
		),
	}
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.wrapHTTP(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	queue.Subscribe(func(job control.Job) {
		s.recordEvent(control.Event{
			Type:    "job." + string(job.Status),
			Message: "job state updated",
			Fields: map[string]any{
				"job_id":   job.ID,
				"status":   job.Status,
				"priority": job.Priority,
			},
		}, true)
		s.observeQueueBacklog()
	})
	s.observeQueueBacklog()

	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/features/summary", s.handleFeatureSummary(baseDir))
	mux.HandleFunc("/v1/release/readiness", s.handleReleaseReadiness)
	mux.HandleFunc("/v1/release/api-contract", s.handleAPIContract)
	mux.HandleFunc("/v1/release/upgrade-assistant", s.handleUpgradeAssistant)
	mux.HandleFunc("/v1/query", s.handleQuery(baseDir))
	mux.HandleFunc("/v1/activity", s.handleActivity)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/events/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/commands/ingest", s.handleCommandIngest(baseDir))
	mux.HandleFunc("/v1/commands/dead-letters", s.handleCommandDeadLetters)
	mux.HandleFunc("/v1/object-store/objects", s.handleObjectStoreObjects)
	mux.HandleFunc("/v1/control/backup", s.handleBackup(baseDir))
	mux.HandleFunc("/v1/control/backups", s.handleBackups)
	mux.HandleFunc("/v1/control/restore", s.handleRestore(baseDir))
	mux.HandleFunc("/v1/control/drill", s.handleDRDrill(baseDir))
	mux.HandleFunc("/v1/webhooks", s.handleWebhooks)
	mux.HandleFunc("/v1/webhooks/", s.handleWebhookAction)
	mux.HandleFunc("/v1/webhooks/deliveries", s.handleWebhookDeliveries)
	mux.HandleFunc("/v1/rules", s.handleRules)
	mux.HandleFunc("/v1/rules/", s.handleRuleAction)
	mux.HandleFunc("/v1/runs", s.handleRuns(baseDir))
	mux.HandleFunc("/v1/runs/", s.handleRunAction(baseDir))
	mux.HandleFunc("/v1/jobs", s.handleJobs(baseDir))
	mux.HandleFunc("/v1/jobs/", s.handleJobByID)
	mux.HandleFunc("/v1/control/emergency-stop", s.handleEmergencyStop)
	mux.HandleFunc("/v1/control/freeze", s.handleFreeze)
	mux.HandleFunc("/v1/control/maintenance", s.handleMaintenance)
	mux.HandleFunc("/v1/control/capacity", s.handleCapacity)
	mux.HandleFunc("/v1/control/canary-health", s.handleCanaryHealth)
	mux.HandleFunc("/v1/control/channels", s.handleChannels)
	mux.HandleFunc("/v1/control/schema-migrations", s.handleSchemaMigrations)
	mux.HandleFunc("/v1/control/preflight", s.handlePreflight)
	mux.HandleFunc("/v1/control/queue", s.handleQueueControl)
	mux.HandleFunc("/v1/control/recover-stuck", s.handleRecoverStuck)
	mux.HandleFunc("/v1/templates", s.handleTemplates(baseDir))
	mux.HandleFunc("/v1/templates/", s.handleTemplateAction)
	mux.HandleFunc("/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/v1/workflows/", s.handleWorkflowAction)
	mux.HandleFunc("/v1/workflow-runs", s.handleWorkflowRuns)
	mux.HandleFunc("/v1/workflow-runs/", s.handleWorkflowRunByID)
	mux.HandleFunc("/v1/canaries", s.handleCanaries(baseDir))
	mux.HandleFunc("/v1/canaries/", s.handleCanaryAction)
	mux.HandleFunc("/v1/associations", s.handleAssociations(baseDir))
	mux.HandleFunc("/v1/associations/", s.handleAssociationAction)
	mux.HandleFunc("/v1/schedules", s.handleSchedules(baseDir))
	mux.HandleFunc("/v1/schedules/", s.handleScheduleAction)
	return s
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.runCancel != nil {
		s.runCancel()
	}
	if s.scheduler != nil {
		s.scheduler.Shutdown()
	}
	if s.canaries != nil {
		s.canaries.Shutdown()
	}
	if s.queue != nil {
		s.queue.Wait()
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

func (s *Server) handleActivity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.events.List())
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	out := map[string]int64{}
	for k, v := range s.metrics {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleEventIngest(w http.ResponseWriter, r *http.Request) {
	type ingestReq struct {
		Type    string         `json:"type"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req ingestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}
	if req.Message == "" {
		req.Message = "external event"
	}
	s.recordEvent(control.Event{
		Type:    req.Type,
		Message: req.Message,
		Fields:  req.Fields,
	}, true)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "ingested"})
}

func (s *Server) handleCommandIngest(baseDir string) http.HandlerFunc {
	type reqBody struct {
		Action         string `json:"action"`
		ConfigPath     string `json:"config_path"`
		Priority       string `json:"priority"`
		IdempotencyKey string `json:"idempotency_key"`
		Checksum       string `json:"checksum"`
		Force          bool   `json:"force"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}

		env := control.CommandEnvelope{
			Action:         req.Action,
			ConfigPath:     req.ConfigPath,
			Priority:       req.Priority,
			IdempotencyKey: req.IdempotencyKey,
			Checksum:       req.Checksum,
		}

		if strings.TrimSpace(req.Checksum) == "" {
			dlq := s.commands.RecordDeadLetter(env, "checksum is required")
			writeJSON(w, http.StatusUnprocessableEntity, dlq)
			return
		}

		expected := control.ComputeCommandChecksum(req.Action, req.ConfigPath, req.Priority, req.IdempotencyKey)
		if !strings.EqualFold(strings.TrimSpace(req.Checksum), expected) {
			dlq := s.commands.RecordDeadLetter(env, "checksum mismatch")
			writeJSON(w, http.StatusUnprocessableEntity, dlq)
			return
		}
		if strings.ToLower(strings.TrimSpace(req.Action)) != "apply" {
			dlq := s.commands.RecordDeadLetter(env, "unsupported action")
			writeJSON(w, http.StatusBadRequest, dlq)
			return
		}
		if strings.TrimSpace(req.ConfigPath) == "" {
			dlq := s.commands.RecordDeadLetter(env, "config_path is required")
			writeJSON(w, http.StatusBadRequest, dlq)
			return
		}

		configPath := req.ConfigPath
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseDir, configPath)
		}
		if _, err := os.Stat(configPath); err != nil {
			dlq := s.commands.RecordDeadLetter(env, "config_path not found")
			writeJSON(w, http.StatusBadRequest, dlq)
			return
		}

		accepted := s.commands.RecordAccepted(env)
		force := req.Force || strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
		job, err := s.queue.Enqueue(configPath, req.IdempotencyKey, force, req.Priority)
		if err != nil {
			dlq := s.commands.RecordDeadLetter(env, err.Error())
			writeJSON(w, http.StatusConflict, dlq)
			return
		}
		s.events.Append(control.Event{
			Type:    "command.ingested",
			Message: "asynchronous command ingested",
			Fields: map[string]any{
				"command_id": accepted.ID,
				"action":     accepted.Action,
				"job_id":     job.ID,
			},
		})
		writeJSON(w, http.StatusAccepted, map[string]any{
			"command": accepted,
			"job":     job,
		})
	}
}

func (s *Server) handleCommandDeadLetters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.commands.DeadLetters())
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name            string                  `json:"name"`
		SourcePrefix    string                  `json:"source_prefix"`
		Enabled         bool                    `json:"enabled"`
		MatchMode       string                  `json:"match_mode"`
		Conditions      []control.RuleCondition `json:"conditions"`
		Actions         []control.RuleAction    `json:"actions"`
		CooldownSeconds int                     `json:"cooldown_seconds"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.rules.List())
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		rule, err := s.rules.Create(control.Rule{
			Name:            req.Name,
			SourcePrefix:    req.SourcePrefix,
			Enabled:         req.Enabled,
			MatchMode:       req.MatchMode,
			Conditions:      req.Conditions,
			Actions:         req.Actions,
			CooldownSeconds: req.CooldownSeconds,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "rule.created",
			Message: "event rule created",
			Fields: map[string]any{
				"rule_id":       rule.ID,
				"source_prefix": rule.SourcePrefix,
			},
		})
		writeJSON(w, http.StatusCreated, rule)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRuleAction(w http.ResponseWriter, r *http.Request) {
	// /v1/rules/{id} or /v1/rules/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rule path"})
		return
	}
	id := parts[2]
	if len(parts) == 3 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rule, err := s.rules.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rule)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[3]
	switch action {
	case "enable":
		rule, err := s.rules.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rule)
	case "disable":
		rule, err := s.rules.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rule)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown rule action"})
	}
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		EventPrefix string `json:"event_prefix"`
		Secret      string `json:"secret"`
		Enabled     bool   `json:"enabled"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.webhooks.List())
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		webhook, err := s.webhooks.Register(control.WebhookSubscription{
			Name:        req.Name,
			URL:         req.URL,
			EventPrefix: req.EventPrefix,
			Secret:      req.Secret,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, webhook)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWebhookAction(w http.ResponseWriter, r *http.Request) {
	// /v1/webhooks/{id} or /v1/webhooks/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook path"})
		return
	}
	id := parts[2]
	if len(parts) == 3 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		wh, err := s.webhooks.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, wh)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[3]
	switch action {
	case "enable":
		wh, err := s.webhooks.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, wh)
	case "disable":
		wh, err := s.webhooks.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, wh)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown webhook action"})
	}
}

func (s *Server) handleWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.webhooks.Deliveries(limit))
}

func (s *Server) handleRuns(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		st := state.New(baseDir)
		runs, err := st.ListRuns(200)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func (s *Server) handleRunAction(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// /v1/runs/{id}/export
		parts := splitPath(r.URL.Path)
		if len(parts) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run action path"})
			return
		}
		runID := parts[2]
		action := parts[3]
		switch action {
		case "export":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if s.objectStore == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
				return
			}
			run, err := state.New(baseDir).GetRun(runID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			payload, err := json.MarshalIndent(run, "", "  ")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			key := storage.TimestampedJSONKey("runs/"+runID, "run")
			obj, err := s.objectStore.Put(key, payload, "application/json")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id": runID,
				"object": obj,
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown run action"})
		}
	}
}

func (s *Server) handleObjectStoreObjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.objectStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
		return
	}
	prefix := r.URL.Query().Get("prefix")
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := s.objectStore.List(prefix, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleFeatureSummary(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		path := filepath.Join(baseDir, "features.md")
		doc, err := features.Parse(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		report := features.Verify(doc)
		writeJSON(w, http.StatusOK, report)
	}
}

func (s *Server) handleReleaseReadiness(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Signals    control.ReadinessSignals    `json:"signals"`
		Thresholds control.ReadinessThresholds `json:"thresholds"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, control.DefaultReadinessThresholds())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		report := control.EvaluateReadiness(req.Signals, req.Thresholds)
		if !report.Pass {
			writeJSON(w, http.StatusConflict, report)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIContract(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Baseline control.APISpec `json:"baseline"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, currentAPISpec())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		cur := currentAPISpec()
		report := control.DiffAPISpec(req.Baseline, cur)
		if !report.DeprecationLifecyclePass {
			writeJSON(w, http.StatusConflict, report)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUpgradeAssistant(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Baseline control.APISpec `json:"baseline"`
	}
	cur := currentAPISpec()
	switch r.Method {
	case http.MethodGet:
		report := control.DiffAPISpec(control.APISpec{
			Version:   cur.Version,
			Endpoints: cur.Endpoints,
		}, cur)
		writeJSON(w, http.StatusOK, map[string]any{
			"current_spec": cur,
			"report":       report,
			"advice":       control.GenerateUpgradeAdvice(report),
		})
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		report := control.DiffAPISpec(req.Baseline, cur)
		resp := map[string]any{
			"report": report,
			"advice": control.GenerateUpgradeAdvice(report),
		}
		if !report.DeprecationLifecyclePass {
			writeJSON(w, http.StatusConflict, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func currentAPISpec() control.APISpec {
	return control.APISpec{
		Version: "v1",
		Endpoints: []string{
			"GET /healthz",
			"GET /v1/activity",
			"GET /v1/metrics",
			"GET /v1/features/summary",
			"POST /v1/release/readiness",
			"GET /v1/release/readiness",
			"GET /v1/release/api-contract",
			"POST /v1/release/api-contract",
			"GET /v1/release/upgrade-assistant",
			"POST /v1/release/upgrade-assistant",
			"POST /v1/query",
			"POST /v1/events/ingest",
			"POST /v1/commands/ingest",
			"GET /v1/commands/dead-letters",
			"GET /v1/object-store/objects",
			"POST /v1/control/backup",
			"GET /v1/control/backups",
			"POST /v1/control/restore",
			"POST /v1/control/drill",
			"POST /v1/control/emergency-stop",
			"GET /v1/control/emergency-stop",
			"POST /v1/control/freeze",
			"GET /v1/control/freeze",
			"POST /v1/control/maintenance",
			"GET /v1/control/maintenance",
			"POST /v1/control/capacity",
			"GET /v1/control/capacity",
			"GET /v1/control/canary-health",
			"POST /v1/control/channels",
			"GET /v1/control/channels",
			"POST /v1/control/schema-migrations",
			"GET /v1/control/schema-migrations",
			"POST /v1/control/preflight",
			"POST /v1/control/queue",
			"GET /v1/control/queue",
			"POST /v1/control/recover-stuck",
			"GET /v1/runs",
			"POST /v1/runs/{id}/export",
			"GET /v1/jobs",
			"POST /v1/jobs",
			"GET /v1/jobs/{id}",
			"DELETE /v1/jobs/{id}",
			"GET /v1/templates",
			"POST /v1/templates",
			"POST /v1/templates/{id}/launch",
			"DELETE /v1/templates/{id}/delete",
			"GET /v1/workflows",
			"POST /v1/workflows",
			"POST /v1/workflows/{id}/launch",
			"GET /v1/workflow-runs",
			"GET /v1/workflow-runs/{id}",
			"GET /v1/canaries",
			"POST /v1/canaries",
			"GET /v1/canaries/{id}",
			"POST /v1/canaries/{id}/enable",
			"POST /v1/canaries/{id}/disable",
			"GET /v1/associations",
			"POST /v1/associations",
			"GET /v1/associations/{id}/revisions",
			"POST /v1/associations/{id}/enable",
			"POST /v1/associations/{id}/disable",
			"POST /v1/associations/{id}/replay",
			"POST /v1/associations/{id}/export",
			"GET /v1/schedules",
			"POST /v1/schedules",
			"POST /v1/schedules/{id}/enable",
			"POST /v1/schedules/{id}/disable",
			"GET /v1/rules",
			"POST /v1/rules",
			"GET /v1/rules/{id}",
			"POST /v1/rules/{id}/enable",
			"POST /v1/rules/{id}/disable",
			"GET /v1/webhooks",
			"POST /v1/webhooks",
			"GET /v1/webhooks/{id}",
			"POST /v1/webhooks/{id}/enable",
			"POST /v1/webhooks/{id}/disable",
			"GET /v1/webhooks/deliveries",
		},
		Deprecations: []control.APIDeprecation{
			{
				Endpoint:           "DELETE /v1/templates/{id}/delete",
				AnnouncedVersion:   "v1",
				RemoveAfterVersion: "v3",
				Replacement:        "DELETE /v1/templates/{id}",
			},
		},
	}
}

func (s *Server) handleJobs(baseDir string) http.HandlerFunc {
	type createReq struct {
		ConfigPath string `json:"config_path"`
		Priority   string `json:"priority"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.queue.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			key := r.Header.Get("Idempotency-Key")
			force := strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
			priority := req.Priority
			if priority == "" {
				priority = r.Header.Get("X-Queue-Priority")
			}
			job, err := s.queue.Enqueue(req.ConfigPath, key, force, priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusAccepted, job)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "jobs" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing job id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		j, ok := s.queue.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, j)
	case http.MethodDelete:
		if err := s.queue.Cancel(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSchedules(baseDir string) http.HandlerFunc {
	type createReq struct {
		ConfigPath      string `json:"config_path"`
		IntervalSeconds int    `json:"interval_seconds"`
		JitterSeconds   int    `json:"jitter_seconds"`
		Priority        string `json:"priority"`
		ExecutionCost   int    `json:"execution_cost"`
		Host            string `json:"host"`
		Cluster         string `json:"cluster"`
		Environment     string `json:"environment"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.scheduler.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if req.IntervalSeconds <= 0 {
				req.IntervalSeconds = 60
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			sc := s.scheduler.CreateWithOptions(control.ScheduleOptions{
				ConfigPath:    req.ConfigPath,
				Priority:      req.Priority,
				ExecutionCost: req.ExecutionCost,
				Host:          req.Host,
				Cluster:       req.Cluster,
				Environment:   req.Environment,
				Interval:      time.Duration(req.IntervalSeconds) * time.Second,
				Jitter:        time.Duration(req.JitterSeconds) * time.Second,
			})
			writeJSON(w, http.StatusCreated, sc)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleTemplates(baseDir string) http.HandlerFunc {
	type createReq struct {
		Name        string                         `json:"name"`
		Description string                         `json:"description"`
		ConfigPath  string                         `json:"config_path"`
		Defaults    map[string]string              `json:"defaults"`
		Survey      map[string]control.SurveyField `json:"survey"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.templates.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.Name == "" || req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and config_path are required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			t := s.templates.Create(control.Template{
				Name:        req.Name,
				Description: req.Description,
				ConfigPath:  req.ConfigPath,
				Defaults:    req.Defaults,
				Survey:      req.Survey,
			})
			s.events.Append(control.Event{
				Type:    "template.created",
				Message: "template created",
				Fields: map[string]any{
					"template_id": t.ID,
					"name":        t.Name,
				},
			})
			writeJSON(w, http.StatusCreated, t)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleTemplateAction(w http.ResponseWriter, r *http.Request) {
	// /v1/templates/{id}/launch
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid template action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	switch action {
	case "launch":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type launchReq struct {
			Priority string            `json:"priority"`
			Answers  map[string]string `json:"answers"`
		}
		var launch launchReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&launch); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}
		t, ok := s.templates.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
			return
		}
		if err := control.ValidateSurveyAnswers(t.Survey, launch.Answers); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		key := r.Header.Get("Idempotency-Key")
		force := strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
		priority := launch.Priority
		if priority == "" {
			priority = r.Header.Get("X-Queue-Priority")
		}
		job, err := s.queue.Enqueue(t.ConfigPath, key, force, priority)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "template.launched",
			Message: "template launch enqueued",
			Fields: map[string]any{
				"template_id": t.ID,
				"job_id":      job.ID,
			},
		})
		writeJSON(w, http.StatusAccepted, map[string]any{
			"template": t,
			"job":      job,
			"answers":  launch.Answers,
		})
	case "delete":
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := s.templates.Delete(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown template action"})
	}
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Steps       []control.WorkflowStep `json:"steps"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.workflows.List())
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		wf, err := s.workflows.Create(control.WorkflowTemplate{
			Name:        req.Name,
			Description: req.Description,
			Steps:       req.Steps,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "workflow.created",
			Message: "workflow created",
			Fields: map[string]any{
				"workflow_id": wf.ID,
				"name":        wf.Name,
			},
		})
		writeJSON(w, http.StatusCreated, wf)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkflowAction(w http.ResponseWriter, r *http.Request) {
	// /v1/workflows/{id}/launch
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workflow action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	if action != "launch" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown workflow action"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	type launchReq struct {
		Priority string `json:"priority"`
		Force    bool   `json:"force"`
	}
	var req launchReq
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
	}
	force := req.Force || strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
	run, err := s.workflows.Launch(id, req.Priority, force)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.events.Append(control.Event{
		Type:    "workflow.launched",
		Message: "workflow launch started",
		Fields: map[string]any{
			"workflow_id": id,
			"run_id":      run.ID,
		},
	})
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.workflows.ListRuns())
}

func (s *Server) handleWorkflowRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "workflow-runs" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing workflow run id"})
		return
	}
	run, err := s.workflows.GetRun(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleAssociations(baseDir string) http.HandlerFunc {
	type createReq struct {
		ConfigPath      string `json:"config_path"`
		TargetKind      string `json:"target_kind"`
		TargetName      string `json:"target_name"`
		Priority        string `json:"priority"`
		IntervalSeconds int    `json:"interval_seconds"`
		JitterSeconds   int    `json:"jitter_seconds"`
		Enabled         bool   `json:"enabled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.assocs.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if strings.TrimSpace(req.ConfigPath) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			if req.IntervalSeconds <= 0 {
				req.IntervalSeconds = 60
			}

			assoc, err := s.assocs.Create(control.AssociationCreate{
				ConfigPath: req.ConfigPath,
				TargetKind: req.TargetKind,
				TargetName: req.TargetName,
				Priority:   req.Priority,
				Interval:   time.Duration(req.IntervalSeconds) * time.Second,
				Jitter:     time.Duration(req.JitterSeconds) * time.Second,
				Enabled:    req.Enabled,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.events.Append(control.Event{
				Type:    "association.created",
				Message: "policy association created",
				Fields: map[string]any{
					"association_id": assoc.ID,
					"target_kind":    assoc.TargetKind,
					"target_name":    assoc.TargetName,
				},
			})
			writeJSON(w, http.StatusCreated, assoc)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleAssociationAction(w http.ResponseWriter, r *http.Request) {
	// /v1/associations/{id}/revisions|enable|disable|replay
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid association action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	switch action {
	case "revisions":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rev, err := s.assocs.Revisions(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rev)
	case "enable", "disable":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		assoc, err := s.assocs.SetEnabled(id, action == "enable")
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, assoc)
	case "replay":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type replayReq struct {
			Revision int `json:"revision"`
		}
		var req replayReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		assoc, err := s.assocs.Replay(id, req.Revision)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, assoc)
	case "export":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.objectStore == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
			return
		}
		revisions, err := s.assocs.Revisions(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		payload, err := json.MarshalIndent(map[string]any{
			"association_id": id,
			"revisions":      revisions,
		}, "", "  ")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		key := storage.TimestampedJSONKey("associations/"+id, "revisions")
		obj, err := s.objectStore.Put(key, payload, "application/json")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"association_id": id,
			"object":         obj,
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown association action"})
	}
}

func (s *Server) handleEmergencyStop(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.EmergencyStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		st := s.queue.SetEmergencyStop(req.Enabled, req.Reason)
		s.events.Append(control.Event{
			Type:    "control.emergency_stop",
			Message: "emergency stop toggled",
			Fields: map[string]any{
				"active": st.Active,
				"reason": st.Reason,
			},
		})
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Enabled         bool   `json:"enabled"`
		Until           string `json:"until"`
		DurationSeconds int    `json:"duration_seconds"`
		Reason          string `json:"reason"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.FreezeStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if !req.Enabled {
			st := s.queue.ClearFreeze()
			s.events.Append(control.Event{
				Type:    "control.freeze",
				Message: "change freeze cleared",
				Fields: map[string]any{
					"active": st.Active,
				},
			})
			writeJSON(w, http.StatusOK, st)
			return
		}

		var until time.Time
		switch {
		case strings.TrimSpace(req.Until) != "":
			parsed, err := time.Parse(time.RFC3339, req.Until)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "until must be RFC3339"})
				return
			}
			until = parsed
		default:
			if req.DurationSeconds <= 0 {
				req.DurationSeconds = 3600
			}
			until = time.Now().UTC().Add(time.Duration(req.DurationSeconds) * time.Second)
		}

		st := s.queue.SetFreezeUntil(until, req.Reason)
		s.events.Append(control.Event{
			Type:    "control.freeze",
			Message: "change freeze updated",
			Fields: map[string]any{
				"active": st.Active,
				"until":  st.Until,
				"reason": st.Reason,
			},
		})
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scheduler.MaintenanceStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		st, err := s.scheduler.SetMaintenance(req.Kind, req.Name, req.Enabled, req.Reason)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "control.maintenance",
			Message: "maintenance mode updated",
			Fields: map[string]any{
				"kind":    st.Kind,
				"name":    st.Name,
				"enabled": st.Enabled,
				"reason":  st.Reason,
			},
		})
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action           string `json:"action"` // set_capacity|set_host_health
		MaxBacklog       int    `json:"max_backlog"`
		MaxExecutionCost int    `json:"max_execution_cost"`
		Host             string `json:"host"`
		Healthy          bool   `json:"healthy"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scheduler.CapacityStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		var st control.SchedulerCapacityStatus
		switch req.Action {
		case "set_capacity":
			st = s.scheduler.SetCapacity(req.MaxBacklog, req.MaxExecutionCost)
			s.events.Append(control.Event{
				Type:    "control.capacity",
				Message: "scheduler capacity updated",
				Fields: map[string]any{
					"max_backlog":        st.MaxBacklog,
					"max_execution_cost": st.MaxExecutionCost,
				},
			})
		case "set_host_health":
			if strings.TrimSpace(req.Host) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required for set_host_health"})
				return
			}
			st = s.scheduler.SetHostHealth(req.Host, req.Healthy)
			s.events.Append(control.Event{
				Type:    "control.capacity.host_health",
				Message: "host health override updated",
				Fields: map[string]any{
					"host":    strings.ToLower(strings.TrimSpace(req.Host)),
					"healthy": req.Healthy,
				},
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCanaries(baseDir string) http.HandlerFunc {
	type createReq struct {
		Name             string `json:"name"`
		ConfigPath       string `json:"config_path"`
		Priority         string `json:"priority"`
		IntervalSeconds  int    `json:"interval_seconds"`
		JitterSeconds    int    `json:"jitter_seconds"`
		FailureThreshold int    `json:"failure_threshold"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.canaries.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if strings.TrimSpace(req.ConfigPath) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			if req.IntervalSeconds <= 0 {
				req.IntervalSeconds = 60
			}
			canary, err := s.canaries.Create(control.CanaryCreate{
				Name:             req.Name,
				ConfigPath:       req.ConfigPath,
				Priority:         req.Priority,
				Interval:         time.Duration(req.IntervalSeconds) * time.Second,
				Jitter:           time.Duration(req.JitterSeconds) * time.Second,
				FailureThreshold: req.FailureThreshold,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.events.Append(control.Event{
				Type:    "canary.created",
				Message: "synthetic canary created",
				Fields: map[string]any{
					"canary_id": canary.ID,
					"name":      canary.Name,
				},
			})
			writeJSON(w, http.StatusCreated, canary)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleCanaryAction(w http.ResponseWriter, r *http.Request) {
	// /v1/canaries/{id} or /v1/canaries/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid canary action path"})
		return
	}
	id := parts[2]
	if len(parts) == 3 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		canary, err := s.canaries.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, canary)
		return
	}
	action := parts[3]
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "enable":
		canary, err := s.canaries.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, canary)
	case "disable":
		canary, err := s.canaries.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, canary)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown canary action"})
	}
}

func (s *Server) handleCanaryHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.canaries.HealthSummary())
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action               string `json:"action"` // set_channel|check_compatibility|support_matrix
		Component            string `json:"component"`
		Channel              string `json:"channel"`
		ControlPlaneProtocol int    `json:"control_plane_protocol"`
		AgentProtocol        int    `json:"agent_protocol"`
	}
	switch r.Method {
	case http.MethodGet:
		controlPlaneProtocol := control.ParseControlPlaneProtocol(r.URL.Query().Get("control_plane_protocol"))
		writeJSON(w, http.StatusOK, map[string]any{
			"channels":       s.channels.List(),
			"policy":         "n-1",
			"support_matrix": control.BuildSupportMatrix(controlPlaneProtocol),
		})
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		switch req.Action {
		case "set_channel":
			item, err := s.channels.SetChannel(req.Component, req.Channel)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case "check_compatibility":
			result := control.CheckNMinusOneCompatibility(req.ControlPlaneProtocol, req.AgentProtocol)
			if !result.Compatible {
				writeJSON(w, http.StatusConflict, result)
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "support_matrix":
			writeJSON(w, http.StatusOK, control.BuildSupportMatrix(req.ControlPlaneProtocol))
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSchemaMigrations(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action      string `json:"action"` // check|apply
		FromVersion int    `json:"from_version"`
		ToVersion   int    `json:"to_version"`
		PlanRef     string `json:"plan_ref"`
		Notes       string `json:"notes"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.schemaMigs.Status())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		switch req.Action {
		case "check":
			writeJSON(w, http.StatusOK, s.schemaMigs.Check(req.FromVersion, req.ToVersion))
		case "apply":
			rec, err := s.schemaMigs.Apply(req.FromVersion, req.ToVersion, strings.TrimSpace(req.PlanRef), strings.TrimSpace(req.Notes))
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			s.events.Append(control.Event{
				Type:    "control.schema_migration.applied",
				Message: "schema migration applied",
				Fields: map[string]any{
					"migration_id": rec.ID,
					"from":         rec.FromVersion,
					"to":           rec.ToVersion,
					"plan_ref":     rec.PlanRef,
				},
			})
			writeJSON(w, http.StatusOK, rec)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	report := control.RunPreflight(req, s.queue, s.objectStore != nil)
	if report.Status != "pass" {
		writeJSON(w, http.StatusServiceUnavailable, report)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleQueueControl(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action         string `json:"action"` // pause|resume|drain
		TimeoutSeconds int    `json:"timeout_seconds"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.ControlStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		switch req.Action {
		case "pause":
			writeJSON(w, http.StatusOK, s.queue.Pause())
		case "resume":
			writeJSON(w, http.StatusOK, s.queue.Resume())
		case "drain":
			if req.TimeoutSeconds <= 0 {
				req.TimeoutSeconds = 30
			}
			st, err := s.queue.SafeDrain(time.Duration(req.TimeoutSeconds) * time.Second)
			if err != nil {
				writeJSON(w, http.StatusRequestTimeout, map[string]any{
					"error":  err.Error(),
					"status": st,
				})
				return
			}
			writeJSON(w, http.StatusOK, st)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRecoverStuck(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		MaxAgeSeconds int `json:"max_age_seconds"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.MaxAgeSeconds <= 0 {
		req.MaxAgeSeconds = 300
	}
	recovered := s.queue.RecoverStuckJobs(time.Duration(req.MaxAgeSeconds) * time.Second)
	writeJSON(w, http.StatusOK, map[string]any{
		"recovered_count": len(recovered),
		"jobs":            recovered,
	})
}

func (s *Server) handleScheduleAction(w http.ResponseWriter, r *http.Request) {
	// /v1/schedules/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	switch r.Method {
	case http.MethodPost:
		var ok bool
		switch action {
		case "enable":
			ok = s.scheduler.Enable(id)
		case "disable":
			ok = s.scheduler.Disable(id)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": action + "d"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func splitPath(path string) []string {
	trimmed := filepath.ToSlash(filepath.Clean(path))
	parts := make([]string, 0, 8)
	for _, p := range strings.Split(trimmed, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) recordEvent(e control.Event, evaluateRules bool) {
	s.events.Append(e)
	if s.webhooks != nil {
		_ = s.webhooks.Dispatch(e)
	}
	if !evaluateRules || s.rules == nil {
		return
	}
	matches, err := s.rules.Evaluate(e)
	if err != nil {
		s.events.Append(control.Event{
			Type:    "rule.evaluate.error",
			Message: "rule evaluation failed",
			Fields: map[string]any{
				"event_type": e.Type,
				"error":      err.Error(),
			},
		})
		return
	}
	for _, match := range matches {
		s.events.Append(control.Event{
			Type:    "rule.matched",
			Message: "event matched rule",
			Fields: map[string]any{
				"rule_id":    match.RuleID,
				"rule_name":  match.RuleName,
				"event_type": e.Type,
			},
		})
		for _, action := range match.Actions {
			if err := s.executeRuleAction(match, action); err != nil {
				s.events.Append(control.Event{
					Type:    "rule.action.error",
					Message: "rule action failed",
					Fields: map[string]any{
						"rule_id":     match.RuleID,
						"action_type": action.Type,
						"error":       err.Error(),
					},
				})
				continue
			}
			s.events.Append(control.Event{
				Type:    "rule.action.succeeded",
				Message: "rule action executed",
				Fields: map[string]any{
					"rule_id":     match.RuleID,
					"action_type": action.Type,
				},
			})
		}
	}
}

func (s *Server) executeRuleAction(match control.RuleMatch, action control.RuleAction) error {
	switch action.Type {
	case "enqueue_apply":
		configPath := action.ConfigPath
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(s.baseDir, configPath)
		}
		if _, err := os.Stat(configPath); err != nil {
			return err
		}
		_, err := s.queue.Enqueue(configPath, "", action.Force, action.Priority)
		return err
	case "launch_template":
		tpl, ok := s.templates.Get(action.TemplateID)
		if !ok {
			return errors.New("template not found: " + action.TemplateID)
		}
		_, err := s.queue.Enqueue(tpl.ConfigPath, "", action.Force, action.Priority)
		return err
	case "launch_workflow":
		_, err := s.workflows.Launch(action.WorkflowID, action.Priority, action.Force)
		return err
	default:
		return errors.New("unsupported rule action type: " + action.Type)
	}
}

func (s *Server) wrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		reqID := randomID()
		w.Header().Set("X-Request-ID", reqID)

		s.metricsMu.Lock()
		s.metrics["requests_total"]++
		s.metrics["requests."+r.Method]++
		s.metrics["requests."+r.URL.Path]++
		s.metricsMu.Unlock()

		s.events.Append(control.Event{
			Type:    "http.request",
			Message: "request received",
			Fields: map[string]any{
				"id":     reqID,
				"method": r.Method,
				"path":   r.URL.Path,
			},
		})

		next.ServeHTTP(w, r)

		s.events.Append(control.Event{
			Type:    "http.response",
			Message: "request completed",
			Fields: map[string]any{
				"id":         reqID,
				"method":     r.Method,
				"path":       r.URL.Path,
				"started_at": start,
				"ended_at":   time.Now().UTC(),
			},
		})
	})
}

func randomID() string {
	return fmt.Sprintf("req-%d-%d", time.Now().UTC().UnixNano(), rand.Int63())
}

func (s *Server) observeQueueBacklog() {
	st := s.queue.ControlStatus()
	now := time.Now().UTC()

	var emit *control.Event

	s.metricsMu.Lock()
	s.metrics["queue.pending.total"] = int64(st.Pending)
	s.metrics["queue.pending.high"] = int64(st.PendingHigh)
	s.metrics["queue.pending.normal"] = int64(st.PendingNormal)
	s.metrics["queue.pending.low"] = int64(st.PendingLow)
	s.metrics["queue.running"] = int64(st.Running)

	s.backlogSamples = append(s.backlogSamples, backlogSample{
		at:      now,
		pending: st.Pending,
	})
	cutoff := now.Add(-2 * time.Minute)
	first := 0
	for first < len(s.backlogSamples) && s.backlogSamples[first].at.Before(cutoff) {
		first++
	}
	if first > 0 {
		s.backlogSamples = append([]backlogSample{}, s.backlogSamples[first:]...)
	}

	threshold := s.backlogThreshold
	if threshold <= 0 {
		threshold = 100
	}

	growthMilli := int64(0)
	predictive := false
	if len(s.backlogSamples) >= 2 {
		oldest := s.backlogSamples[0]
		latest := s.backlogSamples[len(s.backlogSamples)-1]
		dt := latest.at.Sub(oldest.at).Seconds()
		if dt > 0 && latest.pending > oldest.pending {
			growth := float64(latest.pending-oldest.pending) / dt
			growthMilli = int64(growth * 1000.0)
			projectedFiveMinutes := float64(latest.pending) + (growth * 300.0)
			predictive = projectedFiveMinutes >= float64(threshold)
		}
	}
	s.metrics["queue.backlog_growth_per_sec_milli"] = growthMilli

	prevSat := s.backlogSatActive
	prevWarn := s.backlogWarnActive
	s.backlogSatActive = st.Pending >= threshold
	s.backlogWarnActive = !s.backlogSatActive && predictive && st.Pending >= int(float64(threshold)*0.70)
	recovered := st.Pending <= threshold/2 && (prevSat || prevWarn) && !s.backlogSatActive && !s.backlogWarnActive

	if s.backlogSatActive && !prevSat {
		emit = &control.Event{
			Type:    "queue.saturation",
			Message: "queue backlog exceeded saturation SLO threshold",
			Fields: map[string]any{
				"pending":        st.Pending,
				"threshold":      threshold,
				"pending_high":   st.PendingHigh,
				"pending_normal": st.PendingNormal,
				"pending_low":    st.PendingLow,
			},
		}
	} else if s.backlogWarnActive && !prevWarn {
		emit = &control.Event{
			Type:    "queue.saturation.predicted",
			Message: "predictive queue backlog alert",
			Fields: map[string]any{
				"pending":                      st.Pending,
				"threshold":                    threshold,
				"backlog_growth_per_sec_milli": growthMilli,
			},
		}
	} else if recovered {
		emit = &control.Event{
			Type:    "queue.saturation.recovered",
			Message: "queue backlog recovered below recovery threshold",
			Fields: map[string]any{
				"pending":         st.Pending,
				"recovery_target": threshold / 2,
			},
		}
	}

	s.metrics["queue.saturation.active"] = boolToInt64(s.backlogSatActive)
	s.metrics["queue.saturation.warning"] = boolToInt64(s.backlogWarnActive)
	s.metricsMu.Unlock()

	if emit != nil {
		s.events.Append(*emit)
	}
}

func readIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
