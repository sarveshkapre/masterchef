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
	"sort"
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
	httpServer         *http.Server
	baseDir            string
	queue              *control.Queue
	scheduler          *control.Scheduler
	templates          *control.TemplateStore
	workflows          *control.WorkflowStore
	runbooks           *control.RunbookStore
	assocs             *control.AssociationStore
	commands           *control.CommandIngestStore
	canaries           *control.CanaryStore
	rules              *control.RuleEngine
	webhooks           *control.WebhookDispatcher
	alerts             *control.AlertInbox
	notifications      *control.NotificationRouter
	changeRecords      *control.ChangeRecordStore
	checklists         *control.ChecklistStore
	views              *control.SavedViewStore
	bulk               *control.BulkManager
	actionDocs         *control.ActionDocCatalog
	migrations         *control.MigrationStore
	solutionPacks      *control.SolutionPackCatalog
	useCaseTemplates   *control.UseCaseTemplateCatalog
	workspaceTemplates *control.WorkspaceTemplateCatalog
	channels           *control.ChannelManager
	schemaMigs         *control.SchemaMigrationManager
	dataBags           *control.DataBagStore
	roleEnv            *control.RoleEnvironmentStore
	encryptedVars      *control.EncryptedVariableStore
	facts              *control.FactCache
	varSources         *control.VariableSourceRegistry
	plugins            *control.PluginExtensionStore
	eventBus           *control.EventBus
	nodes              *control.NodeLifecycleStore
	gitopsPreviews     *control.GitOpsPreviewStore
	gitopsPromotions   *control.GitOpsPromotionStore
	objectStore        storage.ObjectStore
	events             *control.EventStore
	runCancel          context.CancelFunc
	metricsMu          sync.Mutex
	metrics            map[string]int64

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
	runbooks := control.NewRunbookStore()
	assocs := control.NewAssociationStore(scheduler)
	commands := control.NewCommandIngestStore(5000)
	canaries := control.NewCanaryStore(queue)
	rules := control.NewRuleEngine()
	webhooks := control.NewWebhookDispatcher(5000)
	alerts := control.NewAlertInbox()
	notifications := control.NewNotificationRouter(5000)
	changeRecords := control.NewChangeRecordStore()
	checklists := control.NewChecklistStore()
	views := control.NewSavedViewStore()
	bulk := control.NewBulkManager(15 * time.Minute)
	actionDocs := control.NewActionDocCatalog()
	migrations := control.NewMigrationStore()
	solutionPacks := control.NewSolutionPackCatalog()
	useCaseTemplates := control.NewUseCaseTemplateCatalog()
	workspaceTemplates := control.NewWorkspaceTemplateCatalog()
	channels := control.NewChannelManager()
	schemaMigs := control.NewSchemaMigrationManager(1)
	dataBags := control.NewDataBagStore()
	roleEnv := control.NewRoleEnvironmentStore(baseDir)
	encryptedVars := control.NewEncryptedVariableStore(baseDir)
	facts := control.NewFactCache(5 * time.Minute)
	varSources := control.NewVariableSourceRegistry(baseDir)
	plugins := control.NewPluginExtensionStore()
	eventBus := control.NewEventBus()
	nodes := control.NewNodeLifecycleStore()
	gitopsPreviews := control.NewGitOpsPreviewStore()
	gitopsPromotions := control.NewGitOpsPromotionStore()
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
		baseDir:            baseDir,
		queue:              queue,
		scheduler:          scheduler,
		templates:          templates,
		workflows:          workflows,
		runbooks:           runbooks,
		assocs:             assocs,
		commands:           commands,
		canaries:           canaries,
		rules:              rules,
		webhooks:           webhooks,
		alerts:             alerts,
		notifications:      notifications,
		changeRecords:      changeRecords,
		checklists:         checklists,
		views:              views,
		bulk:               bulk,
		actionDocs:         actionDocs,
		migrations:         migrations,
		solutionPacks:      solutionPacks,
		useCaseTemplates:   useCaseTemplates,
		workspaceTemplates: workspaceTemplates,
		channels:           channels,
		schemaMigs:         schemaMigs,
		dataBags:           dataBags,
		roleEnv:            roleEnv,
		encryptedVars:      encryptedVars,
		facts:              facts,
		varSources:         varSources,
		plugins:            plugins,
		eventBus:           eventBus,
		nodes:              nodes,
		gitopsPreviews:     gitopsPreviews,
		gitopsPromotions:   gitopsPromotions,
		objectStore:        objectStore,
		events:             events,
		metrics:            map[string]int64{},
		runCancel:          runCancel,
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
	mux.HandleFunc("/v1/docs/actions", s.handleActionDocs)
	mux.HandleFunc("/v1/docs/actions/", s.handleActionDocByID)
	mux.HandleFunc("/v1/release/readiness", s.handleReleaseReadiness)
	mux.HandleFunc("/v1/release/blocker-policy", s.handleReleaseBlockerPolicy)
	mux.HandleFunc("/v1/release/api-contract", s.handleAPIContract)
	mux.HandleFunc("/v1/release/upgrade-assistant", s.handleUpgradeAssistant)
	mux.HandleFunc("/v1/plans/explain", s.handlePlanExplain(baseDir))
	mux.HandleFunc("/v1/plans/risk-summary", s.handlePlanRiskSummary(baseDir))
	mux.HandleFunc("/v1/policy/simulate", s.handlePolicySimulation(baseDir))
	mux.HandleFunc("/v1/query", s.handleQuery(baseDir))
	mux.HandleFunc("/v1/search", s.handleSearch(baseDir))
	mux.HandleFunc("/v1/inventory/groups", s.handleInventoryGroups(baseDir))
	mux.HandleFunc("/v1/inventory/runtime-hosts", s.handleRuntimeHosts)
	mux.HandleFunc("/v1/inventory/runtime-hosts/", s.handleRuntimeHostAction)
	mux.HandleFunc("/v1/inventory/enroll", s.handleRuntimeEnrollAlias)
	mux.HandleFunc("/v1/gitops/previews", s.handleGitOpsPreviews(baseDir))
	mux.HandleFunc("/v1/gitops/previews/", s.handleGitOpsPreviewAction)
	mux.HandleFunc("/v1/gitops/promotions", s.handleGitOpsPromotions)
	mux.HandleFunc("/v1/gitops/promotions/", s.handleGitOpsPromotionAction)
	mux.HandleFunc("/v1/gitops/reconcile", s.handleGitOpsReconcile(baseDir))
	mux.HandleFunc("/v1/data-bags", s.handleDataBags)
	mux.HandleFunc("/v1/data-bags/search", s.handleDataBagSearch)
	mux.HandleFunc("/v1/data-bags/", s.handleDataBagItem)
	mux.HandleFunc("/v1/roles", s.handleRoles)
	mux.HandleFunc("/v1/roles/", s.handleRoleAction)
	mux.HandleFunc("/v1/environments", s.handleEnvironments)
	mux.HandleFunc("/v1/environments/", s.handleEnvironmentAction)
	mux.HandleFunc("/v1/vars/encrypted/keys", s.handleEncryptedVariableKeys)
	mux.HandleFunc("/v1/vars/encrypted/files", s.handleEncryptedVariableFiles)
	mux.HandleFunc("/v1/vars/encrypted/files/", s.handleEncryptedVariableFileAction)
	mux.HandleFunc("/v1/vars/resolve", s.handleVariableResolve)
	mux.HandleFunc("/v1/vars/explain", s.handleVariableExplain)
	mux.HandleFunc("/v1/vars/sources/resolve", s.handleVariableSourceResolve)
	mux.HandleFunc("/v1/plugins/extensions", s.handlePluginExtensions)
	mux.HandleFunc("/v1/plugins/extensions/", s.handlePluginExtensionAction)
	mux.HandleFunc("/v1/event-bus/targets", s.handleEventBusTargets)
	mux.HandleFunc("/v1/event-bus/targets/", s.handleEventBusTargetAction)
	mux.HandleFunc("/v1/event-bus/deliveries", s.handleEventBusDeliveries)
	mux.HandleFunc("/v1/event-bus/publish", s.handleEventBusPublish)
	mux.HandleFunc("/v1/pillar/resolve", s.handlePillarResolve)
	mux.HandleFunc("/v1/facts/cache", s.handleFactCache)
	mux.HandleFunc("/v1/facts/cache/", s.handleFactCacheNode)
	mux.HandleFunc("/v1/facts/mine/query", s.handleFactMineQuery)
	mux.HandleFunc("/v1/incidents/view", s.handleIncidentView(baseDir))
	mux.HandleFunc("/v1/fleet/nodes", s.handleFleetNodes(baseDir))
	mux.HandleFunc("/v1/drift/insights", s.handleDriftInsights(baseDir))
	mux.HandleFunc("/v1/activity", s.handleActivity)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/events/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/alerts/inbox", s.handleAlertInbox)
	mux.HandleFunc("/v1/notifications/targets", s.handleNotificationTargets)
	mux.HandleFunc("/v1/notifications/targets/", s.handleNotificationTargetAction)
	mux.HandleFunc("/v1/notifications/deliveries", s.handleNotificationDeliveries)
	mux.HandleFunc("/v1/change-records", s.handleChangeRecords)
	mux.HandleFunc("/v1/change-records/", s.handleChangeRecordAction)
	mux.HandleFunc("/v1/bulk/preview", s.handleBulkPreview)
	mux.HandleFunc("/v1/bulk/execute", s.handleBulkExecute)
	mux.HandleFunc("/v1/views", s.handleViews)
	mux.HandleFunc("/v1/views/", s.handleViewAction)
	mux.HandleFunc("/v1/views/home", s.handlePersonaHome(baseDir))
	mux.HandleFunc("/v1/views/workloads", s.handleWorkloadViews)
	mux.HandleFunc("/v1/migrations/assess", s.handleMigrationAssess)
	mux.HandleFunc("/v1/migrations/reports", s.handleMigrationReports)
	mux.HandleFunc("/v1/migrations/reports/", s.handleMigrationReportByID)
	mux.HandleFunc("/v1/use-case-templates", s.handleUseCaseTemplates(baseDir))
	mux.HandleFunc("/v1/use-case-templates/", s.handleUseCaseTemplateAction(baseDir))
	mux.HandleFunc("/v1/solution-packs", s.handleSolutionPacks(baseDir))
	mux.HandleFunc("/v1/solution-packs/", s.handleSolutionPackAction(baseDir))
	mux.HandleFunc("/v1/workspace-templates", s.handleWorkspaceTemplates(baseDir))
	mux.HandleFunc("/v1/workspace-templates/", s.handleWorkspaceTemplateAction(baseDir))
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
	mux.HandleFunc("/v1/compat/beacon-reactor/rules", s.handleBeaconReactorRules)
	mux.HandleFunc("/v1/compat/beacon-reactor/rules/", s.handleBeaconReactorRuleAction)
	mux.HandleFunc("/v1/compat/beacon-reactor/emit", s.handleBeaconReactorEmit)
	mux.HandleFunc("/v1/runs", s.handleRuns(baseDir))
	mux.HandleFunc("/v1/runs/digest", s.handleRunDigest(baseDir))
	mux.HandleFunc("/v1/runs/compare", s.handleRunCompare(baseDir))
	mux.HandleFunc("/v1/runs/", s.handleRunAction(baseDir))
	mux.HandleFunc("/v1/jobs", s.handleJobs(baseDir))
	mux.HandleFunc("/v1/jobs/", s.handleJobByID)
	mux.HandleFunc("/v1/control/emergency-stop", s.handleEmergencyStop)
	mux.HandleFunc("/v1/control/freeze", s.handleFreeze)
	mux.HandleFunc("/v1/control/maintenance", s.handleMaintenance)
	mux.HandleFunc("/v1/control/handoff", s.handleHandoff)
	mux.HandleFunc("/v1/control/checklists", s.handleChecklists)
	mux.HandleFunc("/v1/control/checklists/", s.handleChecklistAction)
	mux.HandleFunc("/v1/control/capacity", s.handleCapacity)
	mux.HandleFunc("/v1/control/canary-health", s.handleCanaryHealth)
	mux.HandleFunc("/v1/control/channels", s.handleChannels)
	mux.HandleFunc("/v1/control/schema-migrations", s.handleSchemaMigrations)
	mux.HandleFunc("/v1/control/preflight", s.handlePreflight)
	mux.HandleFunc("/v1/control/invariants/check", s.handleInvariantChecks)
	mux.HandleFunc("/v1/control/blast-radius-map", s.handleBlastRadiusMap(baseDir))
	mux.HandleFunc("/v1/control/queue", s.handleQueueControl)
	mux.HandleFunc("/v1/control/recover-stuck", s.handleRecoverStuck)
	mux.HandleFunc("/v1/templates", s.handleTemplates(baseDir))
	mux.HandleFunc("/v1/templates/", s.handleTemplateAction)
	mux.HandleFunc("/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/v1/runbooks", s.handleRunbooks(baseDir))
	mux.HandleFunc("/v1/runbooks/", s.handleRunbookAction(baseDir))
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

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
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
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			since = parsed
		}
	}
	var until time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("until")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			until = parsed
		}
	}
	desc := true
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("order")), "asc") {
		desc = false
	}
	items := s.events.Query(control.EventQuery{
		Since:      since,
		Until:      until,
		TypePrefix: r.URL.Query().Get("type_prefix"),
		Contains:   r.URL.Query().Get("contains"),
		Limit:      limit,
		Desc:       desc,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"count":  len(items),
		"limit":  limit,
		"order":  map[bool]string{true: "desc", false: "asc"}[desc],
		"since":  since,
		"until":  until,
		"filter": map[string]any{"type_prefix": r.URL.Query().Get("type_prefix"), "contains": r.URL.Query().Get("contains")},
	})
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

func (s *Server) handleAlertInbox(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action          string `json:"action"` // acknowledge|resolve|suppress|unsuppress
		ID              string `json:"id"`
		Fingerprint     string `json:"fingerprint"`
		DurationSeconds int    `json:"duration_seconds"`
		Reason          string `json:"reason"`
	}
	switch r.Method {
	case http.MethodGet:
		limit := 200
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		writeJSON(w, http.StatusOK, map[string]any{
			"items":        s.alerts.List(status, limit),
			"summary":      s.alerts.Summary(),
			"suppressions": s.alerts.Suppressions(),
		})
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "acknowledge":
			item, err := s.alerts.Acknowledge(req.ID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case "resolve":
			item, err := s.alerts.Resolve(req.ID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case "suppress":
			fp := strings.TrimSpace(req.Fingerprint)
			if fp == "" && strings.TrimSpace(req.ID) != "" {
				item, err := s.alerts.Get(req.ID)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				fp = item.Fingerprint
			}
			if fp == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fingerprint (or id) is required"})
				return
			}
			if req.DurationSeconds <= 0 {
				req.DurationSeconds = 300
			}
			sup, err := s.alerts.Suppress(fp, time.Duration(req.DurationSeconds)*time.Second, req.Reason)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, sup)
		case "unsuppress":
			fp := strings.TrimSpace(req.Fingerprint)
			if fp == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fingerprint is required"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"fingerprint": fp,
				"cleared":     s.alerts.ClearSuppression(fp),
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotificationTargets(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		URL     string `json:"url"`
		Route   string `json:"route"`
		Enabled bool   `json:"enabled"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.notifications.List())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		target, err := s.notifications.Register(control.NotificationTarget{
			Name:    req.Name,
			Kind:    req.Kind,
			URL:     req.URL,
			Route:   req.Route,
			Enabled: true,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !req.Enabled {
			target, _ = s.notifications.SetEnabled(target.ID, false)
		}
		writeJSON(w, http.StatusCreated, target)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotificationTargetAction(w http.ResponseWriter, r *http.Request) {
	// /v1/notifications/targets/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid notification target action path"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := parts[3]
	action := parts[4]
	switch action {
	case "enable":
		target, err := s.notifications.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, target)
	case "disable":
		target, err := s.notifications.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, target)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown notification target action"})
	}
}

func (s *Server) handleNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, s.notifications.Deliveries(limit))
}

func (s *Server) handleChangeRecords(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Summary      string `json:"summary"`
		TicketSystem string `json:"ticket_system"`
		TicketID     string `json:"ticket_id"`
		TicketURL    string `json:"ticket_url"`
		ConfigPath   string `json:"config_path"`
		RequestedBy  string `json:"requested_by"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.changeRecords.List())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		rec, err := s.changeRecords.Create(control.ChangeRecord{
			Summary:      req.Summary,
			TicketSystem: req.TicketSystem,
			TicketID:     req.TicketID,
			TicketURL:    req.TicketURL,
			ConfigPath:   req.ConfigPath,
			RequestedBy:  req.RequestedBy,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChangeRecordAction(w http.ResponseWriter, r *http.Request) {
	// /v1/change-records/{id} or /v1/change-records/{id}/approve|reject|attach-job|complete|fail
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid change record action path"})
		return
	}
	id := parts[2]
	if len(parts) == 3 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rec, err := s.changeRecords.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[3]
	switch action {
	case "approve", "reject":
		var req struct {
			Actor   string `json:"actor"`
			Comment string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		var (
			rec control.ChangeRecord
			err error
		)
		if action == "approve" {
			rec, err = s.changeRecords.Approve(id, req.Actor, req.Comment)
		} else {
			rec, err = s.changeRecords.Reject(id, req.Actor, req.Comment)
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "attach-job":
		var req struct {
			JobID string `json:"job_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		rec, err := s.changeRecords.AttachJob(id, req.JobID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "complete":
		rec, err := s.changeRecords.MarkCompleted(id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "fail":
		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		rec, err := s.changeRecords.MarkFailed(id, req.Reason)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown change record action"})
	}
}

func (s *Server) handleViews(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name     string `json:"name"`
		Entity   string `json:"entity"`
		Mode     string `json:"mode"`
		Query    string `json:"query"`
		QueryAST string `json:"query_ast"`
		Limit    int    `json:"limit"`
		Pinned   bool   `json:"pinned"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.views.List())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		view, err := s.views.Create(control.SavedView{
			Name:     req.Name,
			Entity:   req.Entity,
			Mode:     req.Mode,
			Query:    req.Query,
			QueryAST: req.QueryAST,
			Limit:    req.Limit,
			Pinned:   req.Pinned,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, view)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleViewAction(w http.ResponseWriter, r *http.Request) {
	// /v1/views/{id} or /v1/views/{id}/pin|share
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid view action path"})
		return
	}
	id := parts[2]
	if len(parts) == 3 {
		switch r.Method {
		case http.MethodGet:
			view, err := s.views.Get(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, view)
		case http.MethodDelete:
			if err := s.views.Delete(id); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[3]
	switch action {
	case "pin":
		var req struct {
			Pinned bool `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		view, err := s.views.SetPinned(id, req.Pinned)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
	case "share":
		view, err := s.views.RegenerateShareToken(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown view action"})
	}
}

func (s *Server) handleSolutionPacks(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		packs := s.solutionPacks.List()
		category := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("category")))
		if category == "" {
			writeJSON(w, http.StatusOK, packs)
			return
		}
		filtered := make([]control.SolutionPack, 0, len(packs))
		for _, p := range packs {
			if strings.ToLower(strings.TrimSpace(p.Category)) == category {
				filtered = append(filtered, p)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
		_ = baseDir
	}
}

func (s *Server) handleSolutionPackAction(baseDir string) http.HandlerFunc {
	type applyReq struct {
		OutputPath     string `json:"output_path"`
		CreateTemplate bool   `json:"create_template"`
		CreateRunbook  bool   `json:"create_runbook"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// /v1/solution-packs/{id}/apply
		parts := splitPath(r.URL.Path)
		if len(parts) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid solution pack action path"})
			return
		}
		id := parts[2]
		action := parts[3]
		if action != "apply" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown solution pack action"})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req applyReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}
		pack, err := s.solutionPacks.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		out := strings.TrimSpace(req.OutputPath)
		if out == "" {
			out = filepath.Join("solution-packs", id+".yaml")
		}
		if !filepath.IsAbs(out) {
			out = filepath.Join(baseDir, out)
		}
		if _, statErr := os.Stat(out); statErr == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "output_path already exists"})
			return
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := os.WriteFile(out, []byte(pack.StarterConfigYAML), 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if !req.CreateTemplate && !req.CreateRunbook {
			req.CreateTemplate = true
		}
		var createdTemplate *control.Template
		if req.CreateTemplate {
			tpl := s.templates.Create(control.Template{
				Name:        "solution-pack/" + pack.ID,
				Description: pack.Description,
				ConfigPath:  out,
				Defaults:    map[string]string{},
				Survey:      map[string]control.SurveyField{},
			})
			createdTemplate = &tpl
		}
		var createdRunbook *control.Runbook
		if req.CreateRunbook {
			rbInput := control.Runbook{
				Name:        "runbook/" + pack.ID,
				Description: "Generated from solution pack catalog",
				TargetType:  control.RunbookTargetConfig,
				ConfigPath:  out,
				RiskLevel:   "medium",
				Owner:       "platform",
				Tags:        append([]string{}, pack.RecommendedTags...),
			}
			rb, err := s.runbooks.Create(rbInput)
			if err == nil {
				approved, _ := s.runbooks.Approve(rb.ID)
				createdRunbook = &approved
			}
		}

		resp := map[string]any{
			"solution_pack": pack,
			"output_path":   out,
		}
		if createdTemplate != nil {
			resp["template"] = *createdTemplate
		}
		if createdRunbook != nil {
			resp["runbook"] = *createdRunbook
		}
		writeJSON(w, http.StatusCreated, resp)
	}
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

func (s *Server) handleRunDigest(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		hours := 24
		if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				hours = n
			}
		}
		if hours > 24*30 {
			hours = 24 * 30
		}
		limit := 1000
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		now := time.Now().UTC()
		windowStart := now.Add(-time.Duration(hours) * time.Hour)

		runs, err := state.New(baseDir).ListRuns(limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		filtered := make([]state.RunRecord, 0, len(runs))
		for _, run := range runs {
			ref := run.StartedAt
			if ref.IsZero() {
				ref = run.EndedAt
			}
			if ref.IsZero() || ref.Before(windowStart) {
				continue
			}
			filtered = append(filtered, run)
		}
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].StartedAt.After(filtered[j].StartedAt)
		})

		total := len(filtered)
		succeeded := 0
		failed := 0
		changedResources := 0
		failedRunIDs := make([]string, 0)
		for _, run := range filtered {
			switch run.Status {
			case state.RunSucceeded:
				succeeded++
			case state.RunFailed:
				failed++
				failedRunIDs = append(failedRunIDs, run.ID)
			}
			for _, res := range run.Results {
				if res.Changed {
					changedResources++
				}
			}
		}
		failRate := 0.0
		if total > 0 {
			failRate = float64(failed) / float64(total)
		}

		queueStatus := s.queue.ControlStatus()
		emergency := s.queue.EmergencyStatus()
		canary := s.canaries.HealthSummary()
		riskScore := int(failRate * 70.0)
		if queueStatus.Pending > 0 {
			riskScore += 10
		}
		if queueStatus.Pending >= s.backlogThreshold {
			riskScore += 10
		}
		if emergency.Active {
			riskScore += 15
		}
		if status, _ := canary["status"].(string); status == "degraded" {
			riskScore += 15
		}
		if riskScore > 100 {
			riskScore = 100
		}
		riskLevel := "low"
		if riskScore >= 60 {
			riskLevel = "high"
		} else if riskScore >= 30 {
			riskLevel = "medium"
		}
		if len(failedRunIDs) > 5 {
			failedRunIDs = failedRunIDs[:5]
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"window_hours":        hours,
			"window_start":        windowStart,
			"window_end":          now,
			"total_runs":          total,
			"succeeded_runs":      succeeded,
			"failed_runs":         failed,
			"fail_rate":           failRate,
			"changed_resources":   changedResources,
			"recent_failures":     failedRunIDs,
			"latent_risk_score":   riskScore,
			"latent_risk_level":   riskLevel,
			"queue_status":        queueStatus,
			"canary_health":       canary,
			"emergency_stop":      emergency,
			"summary_generated":   now,
			"summary_explanation": "Risk score blends run failure rate with queue pressure, canary health, and emergency controls.",
		})
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
		case "timeline":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			run, err := state.New(baseDir).GetRun(runID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			beforeMinutes := 60
			if raw := strings.TrimSpace(r.URL.Query().Get("minutes_before")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
					beforeMinutes = n
				}
			}
			afterMinutes := 60
			if raw := strings.TrimSpace(r.URL.Query().Get("minutes_after")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
					afterMinutes = n
				}
			}
			limit := 1000
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			beforeWindow := time.Duration(beforeMinutes) * time.Minute
			afterWindow := time.Duration(afterMinutes) * time.Minute
			items := s.buildRunTimeline(run, beforeWindow, afterWindow, limit)
			windowStart, windowEnd := timelineWindow(run, beforeWindow, afterWindow)
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":          runID,
				"window_start":    windowStart,
				"window_end":      windowEnd,
				"items":           items,
				"phase_breakdown": timelineSummary(items),
				"count":           len(items),
			})
		case "correlations":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			run, err := state.New(baseDir).GetRun(runID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			items := buildRunCorrelations(run)
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":       runID,
				"count":        len(items),
				"correlations": items,
			})
		case "retry":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			run, err := state.New(baseDir).GetRun(runID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			type reqBody struct {
				ConfigPath     string `json:"config_path"`
				Priority       string `json:"priority"`
				Force          bool   `json:"force"`
				IdempotencyKey string `json:"idempotency_key"`
			}
			var req reqBody
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if run.Status == state.RunSucceeded && !req.Force {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "run succeeded; set force=true to retry anyway"})
				return
			}
			configPath := strings.TrimSpace(req.ConfigPath)
			if configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required for retry"})
				return
			}
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(baseDir, configPath)
			}
			if _, err := os.Stat(configPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			key := strings.TrimSpace(req.IdempotencyKey)
			if key == "" {
				key = "retry-" + runID + "-" + time.Now().UTC().Format("20060102T150405")
			}
			job, err := s.queue.Enqueue(configPath, key, req.Force, req.Priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"action":        "retry",
				"source_run_id": runID,
				"job":           job,
			})
		case "rollback":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			run, err := state.New(baseDir).GetRun(runID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			type reqBody struct {
				RollbackConfigPath string `json:"rollback_config_path"`
				Priority           string `json:"priority"`
				Force              bool   `json:"force"`
				Reason             string `json:"reason"`
				IdempotencyKey     string `json:"idempotency_key"`
			}
			var req reqBody
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if run.Status == state.RunSucceeded && !req.Force {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "run succeeded; set force=true to allow rollback"})
				return
			}
			configPath := strings.TrimSpace(req.RollbackConfigPath)
			if configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rollback_config_path is required"})
				return
			}
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(baseDir, configPath)
			}
			if _, err := os.Stat(configPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("rollback_config_path not found: %v", err)})
				return
			}
			key := strings.TrimSpace(req.IdempotencyKey)
			if key == "" {
				key = "rollback-" + runID + "-" + time.Now().UTC().Format("20060102T150405")
			}
			priority := req.Priority
			if strings.TrimSpace(priority) == "" {
				priority = "high"
			}
			job, err := s.queue.Enqueue(configPath, key, req.Force, priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			s.recordEvent(control.Event{
				Type:    "run.rollback.requested",
				Message: "rollback requested from run context",
				Fields: map[string]any{
					"run_id":      runID,
					"job_id":      job.ID,
					"reason":      strings.TrimSpace(req.Reason),
					"config_path": configPath,
				},
			}, true)
			writeJSON(w, http.StatusAccepted, map[string]any{
				"action":        "rollback",
				"source_run_id": runID,
				"job":           job,
			})
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
		case "triage-bundle":
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
			events := s.events.List()
			windowStart, windowEnd := timelineWindow(run, 5*time.Minute, 5*time.Minute)
			correlated := filterCorrelatedEvents(events, windowStart, windowEnd, 1000)
			hostSet := map[string]struct{}{}
			for _, result := range run.Results {
				if strings.TrimSpace(result.Host) == "" {
					continue
				}
				hostSet[result.Host] = struct{}{}
			}
			hosts := make([]string, 0, len(hostSet))
			for host := range hostSet {
				hosts = append(hosts, host)
			}
			sort.Strings(hosts)

			payload, err := json.MarshalIndent(map[string]any{
				"run":               run,
				"correlated_events": correlated,
				"host_metadata": map[string]any{
					"hosts":      hosts,
					"host_count": len(hosts),
				},
				"artifacts": map[string]any{
					"provider_output": "captured in run results",
					"diffs":           "captured in checker/check report surfaces",
					"facts":           "available through query API/entity integrations",
				},
				"generated_at": time.Now().UTC(),
			}, "", "  ")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			key := storage.TimestampedJSONKey("runs/"+runID, "triage-bundle")
			obj, err := s.objectStore.Put(key, payload, "application/json")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":            runID,
				"object":            obj,
				"correlated_events": len(correlated),
				"host_count":        len(hosts),
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
			"GET /v1/search",
			"GET /v1/inventory/groups",
			"GET /v1/inventory/runtime-hosts",
			"POST /v1/inventory/runtime-hosts",
			"POST /v1/inventory/enroll",
			"GET /v1/inventory/runtime-hosts/{name}",
			"POST /v1/inventory/runtime-hosts/{name}/heartbeat",
			"POST /v1/inventory/runtime-hosts/{name}/bootstrap",
			"POST /v1/inventory/runtime-hosts/{name}/activate",
			"POST /v1/inventory/runtime-hosts/{name}/quarantine",
			"POST /v1/inventory/runtime-hosts/{name}/decommission",
			"GET /v1/gitops/previews",
			"POST /v1/gitops/previews",
			"GET /v1/gitops/previews/{id}",
			"POST /v1/gitops/previews/{id}/promote",
			"POST /v1/gitops/previews/{id}/close",
			"GET /v1/gitops/promotions",
			"POST /v1/gitops/promotions",
			"GET /v1/gitops/promotions/{id}",
			"POST /v1/gitops/promotions/{id}/advance",
			"POST /v1/gitops/reconcile",
			"GET /v1/incidents/view",
			"GET /v1/fleet/nodes",
			"GET /v1/drift/insights",
			"GET /v1/metrics",
			"GET /v1/features/summary",
			"GET /v1/docs/actions",
			"GET /v1/docs/actions/{id}",
			"POST /v1/plans/explain",
			"POST /v1/plans/risk-summary",
			"POST /v1/policy/simulate",
			"GET /v1/alerts/inbox",
			"POST /v1/alerts/inbox",
			"GET /v1/notifications/targets",
			"POST /v1/notifications/targets",
			"POST /v1/notifications/targets/{id}/enable",
			"POST /v1/notifications/targets/{id}/disable",
			"GET /v1/notifications/deliveries",
			"GET /v1/change-records",
			"POST /v1/change-records",
			"GET /v1/change-records/{id}",
			"POST /v1/change-records/{id}/approve",
			"POST /v1/change-records/{id}/reject",
			"POST /v1/change-records/{id}/attach-job",
			"POST /v1/change-records/{id}/complete",
			"POST /v1/change-records/{id}/fail",
			"POST /v1/bulk/preview",
			"POST /v1/bulk/execute",
			"GET /v1/views",
			"POST /v1/views",
			"GET /v1/views/{id}",
			"DELETE /v1/views/{id}",
			"POST /v1/views/{id}/pin",
			"POST /v1/views/{id}/share",
			"GET /v1/views/home",
			"GET /v1/views/workloads",
			"POST /v1/migrations/assess",
			"GET /v1/migrations/reports",
			"GET /v1/migrations/reports/{id}",
			"GET /v1/use-case-templates",
			"POST /v1/use-case-templates/{id}/apply",
			"GET /v1/solution-packs",
			"POST /v1/solution-packs/{id}/apply",
			"GET /v1/workspace-templates",
			"POST /v1/workspace-templates/{id}/bootstrap",
			"POST /v1/release/readiness",
			"GET /v1/release/readiness",
			"POST /v1/release/blocker-policy",
			"GET /v1/release/blocker-policy",
			"GET /v1/release/api-contract",
			"POST /v1/release/api-contract",
			"GET /v1/release/upgrade-assistant",
			"POST /v1/release/upgrade-assistant",
			"POST /v1/query",
			"GET /v1/data-bags",
			"POST /v1/data-bags",
			"GET /v1/data-bags/{bag}/{item}",
			"PUT /v1/data-bags/{bag}/{item}",
			"DELETE /v1/data-bags/{bag}/{item}",
			"POST /v1/data-bags/search",
			"GET /v1/roles",
			"POST /v1/roles",
			"GET /v1/roles/{name}",
			"DELETE /v1/roles/{name}",
			"GET /v1/roles/{name}/resolve",
			"GET /v1/environments",
			"POST /v1/environments",
			"GET /v1/environments/{name}",
			"DELETE /v1/environments/{name}",
			"GET /v1/vars/encrypted/keys",
			"POST /v1/vars/encrypted/keys",
			"GET /v1/vars/encrypted/files",
			"POST /v1/vars/encrypted/files",
			"GET /v1/vars/encrypted/files/{name}",
			"DELETE /v1/vars/encrypted/files/{name}",
			"POST /v1/vars/resolve",
			"POST /v1/vars/explain",
			"POST /v1/vars/sources/resolve",
			"GET /v1/plugins/extensions",
			"POST /v1/plugins/extensions",
			"GET /v1/plugins/extensions/{id}",
			"DELETE /v1/plugins/extensions/{id}",
			"POST /v1/plugins/extensions/{id}/enable",
			"POST /v1/plugins/extensions/{id}/disable",
			"GET /v1/event-bus/targets",
			"POST /v1/event-bus/targets",
			"POST /v1/event-bus/targets/{id}/enable",
			"POST /v1/event-bus/targets/{id}/disable",
			"GET /v1/event-bus/deliveries",
			"POST /v1/event-bus/publish",
			"POST /v1/pillar/resolve",
			"GET /v1/facts/cache",
			"POST /v1/facts/cache",
			"GET /v1/facts/cache/{node}",
			"DELETE /v1/facts/cache/{node}",
			"POST /v1/facts/mine/query",
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
			"GET /v1/control/handoff",
			"GET /v1/control/checklists",
			"POST /v1/control/checklists",
			"GET /v1/control/checklists/{id}",
			"POST /v1/control/checklists/{id}/complete",
			"POST /v1/control/capacity",
			"GET /v1/control/capacity",
			"GET /v1/control/canary-health",
			"POST /v1/control/channels",
			"GET /v1/control/channels",
			"POST /v1/control/schema-migrations",
			"GET /v1/control/schema-migrations",
			"POST /v1/control/preflight",
			"POST /v1/control/invariants/check",
			"POST /v1/control/blast-radius-map",
			"POST /v1/control/queue",
			"GET /v1/control/queue",
			"POST /v1/control/recover-stuck",
			"GET /v1/runs",
			"GET /v1/runs/digest",
			"GET /v1/runs/compare",
			"GET /v1/runs/{id}/timeline",
			"GET /v1/runs/{id}/correlations",
			"POST /v1/runs/{id}/retry",
			"POST /v1/runs/{id}/rollback",
			"POST /v1/runs/{id}/export",
			"POST /v1/runs/{id}/triage-bundle",
			"GET /v1/jobs",
			"POST /v1/jobs",
			"GET /v1/jobs/{id}",
			"DELETE /v1/jobs/{id}",
			"GET /v1/templates",
			"POST /v1/templates",
			"POST /v1/templates/{id}/launch",
			"DELETE /v1/templates/{id}/delete",
			"GET /v1/runbooks",
			"POST /v1/runbooks",
			"GET /v1/runbooks/{id}",
			"POST /v1/runbooks/{id}/approve",
			"POST /v1/runbooks/{id}/deprecate",
			"POST /v1/runbooks/{id}/launch",
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
			"GET /v1/compat/beacon-reactor/rules",
			"POST /v1/compat/beacon-reactor/rules",
			"GET /v1/compat/beacon-reactor/rules/{id}",
			"POST /v1/compat/beacon-reactor/rules/{id}/enable",
			"POST /v1/compat/beacon-reactor/rules/{id}/disable",
			"POST /v1/compat/beacon-reactor/emit",
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

func (s *Server) handleRunbooks(baseDir string) http.HandlerFunc {
	type createReq struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		TargetType  string   `json:"target_type"` // template|workflow|config
		TargetID    string   `json:"target_id"`
		ConfigPath  string   `json:"config_path"`
		RiskLevel   string   `json:"risk_level"` // low|medium|high
		Owner       string   `json:"owner"`
		Tags        []string `json:"tags"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
			items := s.runbooks.List()
			if status == "" || status == "all" {
				writeJSON(w, http.StatusOK, items)
				return
			}
			filtered := make([]control.Runbook, 0, len(items))
			for _, item := range items {
				if string(item.Status) == status {
					filtered = append(filtered, item)
				}
			}
			writeJSON(w, http.StatusOK, filtered)
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
			if targetType == "config" {
				if !filepath.IsAbs(req.ConfigPath) {
					req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
				}
				if _, err := os.Stat(req.ConfigPath); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
					return
				}
			} else if targetType == "template" {
				if _, ok := s.templates.Get(req.TargetID); !ok {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target template not found"})
					return
				}
			} else if targetType == "workflow" {
				if _, ok := s.workflows.Get(req.TargetID); !ok {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target workflow not found"})
					return
				}
			}
			runbook, err := s.runbooks.Create(control.Runbook{
				Name:        req.Name,
				Description: req.Description,
				TargetType:  control.RunbookTargetType(targetType),
				TargetID:    req.TargetID,
				ConfigPath:  req.ConfigPath,
				RiskLevel:   req.RiskLevel,
				Owner:       req.Owner,
				Tags:        req.Tags,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.events.Append(control.Event{
				Type:    "runbook.created",
				Message: "runbook created",
				Fields: map[string]any{
					"runbook_id":    runbook.ID,
					"name":          runbook.Name,
					"target_type":   runbook.TargetType,
					"target_id":     runbook.TargetID,
					"config_path":   runbook.ConfigPath,
					"risk_level":    runbook.RiskLevel,
					"catalog_state": runbook.Status,
				},
			})
			writeJSON(w, http.StatusCreated, runbook)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleRunbookAction(baseDir string) http.HandlerFunc {
	type launchReq struct {
		Priority string            `json:"priority"`
		Answers  map[string]string `json:"answers"`
		Force    bool              `json:"force"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// /v1/runbooks/{id} or /v1/runbooks/{id}/approve|deprecate|launch
		parts := splitPath(r.URL.Path)
		if len(parts) < 3 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runbook action path"})
			return
		}
		id := parts[2]
		if len(parts) == 3 {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			runbook, err := s.runbooks.Get(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, runbook)
			return
		}
		action := parts[3]
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch action {
		case "approve":
			runbook, err := s.runbooks.Approve(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, runbook)
		case "deprecate":
			runbook, err := s.runbooks.Deprecate(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, runbook)
		case "launch":
			var req launchReq
			if r.ContentLength > 0 {
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
					return
				}
			}
			runbook, err := s.runbooks.Get(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			if runbook.Status != control.RunbookApproved {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "runbook must be approved before launch"})
				return
			}
			force := req.Force || strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
			priority := req.Priority
			if priority == "" {
				priority = r.Header.Get("X-Queue-Priority")
			}
			switch runbook.TargetType {
			case control.RunbookTargetTemplate:
				tpl, ok := s.templates.Get(runbook.TargetID)
				if !ok {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "template target not found"})
					return
				}
				if err := control.ValidateSurveyAnswers(tpl.Survey, req.Answers); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				key := r.Header.Get("Idempotency-Key")
				job, err := s.queue.Enqueue(tpl.ConfigPath, key, force, priority)
				if err != nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusAccepted, map[string]any{
					"runbook":  runbook,
					"template": tpl,
					"job":      job,
					"answers":  req.Answers,
				})
			case control.RunbookTargetWorkflow:
				run, err := s.workflows.Launch(runbook.TargetID, priority, force)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusAccepted, map[string]any{
					"runbook":      runbook,
					"workflow_run": run,
				})
			case control.RunbookTargetConfig:
				configPath := runbook.ConfigPath
				if !filepath.IsAbs(configPath) {
					configPath = filepath.Join(baseDir, configPath)
				}
				if _, err := os.Stat(configPath); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
					return
				}
				key := r.Header.Get("Idempotency-Key")
				job, err := s.queue.Enqueue(configPath, key, force, priority)
				if err != nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusAccepted, map[string]any{
					"runbook": runbook,
					"job":     job,
				})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported runbook target type"})
				return
			}
			s.events.Append(control.Event{
				Type:    "runbook.launched",
				Message: "runbook launch triggered",
				Fields: map[string]any{
					"runbook_id":  runbook.ID,
					"target_type": runbook.TargetType,
					"risk_level":  runbook.RiskLevel,
				},
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown runbook action"})
		}
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

func (s *Server) handleHandoff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	queueStatus := s.queue.ControlStatus()
	emergency := s.queue.EmergencyStatus()
	freeze := s.queue.FreezeStatus()
	maintenance := s.scheduler.MaintenanceStatus()
	canary := s.canaries.HealthSummary()
	capacity := s.scheduler.CapacityStatus()

	jobs := s.queue.List()
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	activeRollouts := make([]control.Job, 0, len(jobs))
	for _, job := range jobs {
		if job.Status != control.JobPending && job.Status != control.JobRunning {
			continue
		}
		activeRollouts = append(activeRollouts, job)
		if len(activeRollouts) >= 25 {
			break
		}
	}

	risks := make([]string, 0)
	blocked := make([]string, 0)
	if emergency.Active {
		risks = append(risks, "Emergency stop is active; all new applies are blocked.")
		blocked = append(blocked, "new applies blocked by emergency stop")
	}
	if freeze.Active {
		risks = append(risks, "Change freeze is active until "+freeze.Until.Format(time.RFC3339)+".")
		blocked = append(blocked, "new applies blocked by change freeze")
	}
	if queueStatus.Paused {
		risks = append(risks, "Queue is paused; dispatch will not progress until resumed.")
		blocked = append(blocked, "queue dispatch paused")
	}
	if queueStatus.Pending >= capacity.MaxBacklog {
		risks = append(risks, "Queue backlog at/above configured capacity threshold.")
	}
	if status, _ := canary["status"].(string); status == "degraded" {
		risks = append(risks, "Synthetic canary health is degraded.")
	}
	activeMaintenance := make([]control.MaintenanceTarget, 0)
	for _, mt := range maintenance {
		if mt.Enabled {
			activeMaintenance = append(activeMaintenance, mt)
		}
	}
	if len(activeMaintenance) > 0 {
		blocked = append(blocked, "scheduled dispatch suppressed by maintenance targets")
	}
	if len(risks) == 0 {
		risks = append(risks, "No critical control-plane risks detected at handoff time.")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":          time.Now().UTC(),
		"queue":                 queueStatus,
		"emergency_stop":        emergency,
		"freeze":                freeze,
		"maintenance":           maintenance,
		"canary_health":         canary,
		"active_rollouts":       activeRollouts,
		"blocked_actions":       blocked,
		"risks":                 risks,
		"handoff_checklist":     []string{"review blocked actions", "review active rollouts", "acknowledge degraded canaries", "confirm queue mode before handoff"},
		"next_operator_actions": []string{"clear stale freeze/emergency flags if no longer needed", "resume queue if paused intentionally", "triage unhealthy canaries before major rollout"},
	})
}

func (s *Server) handleChecklists(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action    string         `json:"action"` // create
		Name      string         `json:"name"`
		RiskLevel string         `json:"risk_level"`
		Context   map[string]any `json:"context"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.checklists.List())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if req.Action == "" {
			req.Action = "create"
		}
		if req.Action != "create" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = "operator checklist"
		}
		item, err := s.checklists.Create(req.Name, req.RiskLevel, req.Context)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChecklistAction(w http.ResponseWriter, r *http.Request) {
	// /v1/control/checklists/{id} or /v1/control/checklists/{id}/complete
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid checklist action path"})
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := s.checklists.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) < 5 || parts[4] != "complete" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ItemID string `json:"item_id"`
		Notes  string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := s.checklists.CompleteItem(id, req.ItemID, req.Notes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
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
	if s.eventBus != nil {
		_ = s.eventBus.Publish(e)
	}
	if s.alerts != nil {
		if res, ok := s.alerts.IngestEvent(e); ok && s.notifications != nil {
			_ = s.notifications.NotifyAlert(res.Item)
		}
	}
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
