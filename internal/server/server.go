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
	httpServer             *http.Server
	baseDir                string
	queue                  *control.Queue
	runLeases              *control.RunLeaseStore
	stepSnapshots          *control.StepSnapshotStore
	executionLocks         *control.ExecutionLockStore
	checkpoints            *control.ExecutionCheckpointStore
	scheduler              *control.Scheduler
	templates              *control.TemplateStore
	wizards                *control.WorkflowWizardCatalog
	tasks                  *control.TaskFrameworkStore
	workflows              *control.WorkflowStore
	runbooks               *control.RunbookStore
	assocs                 *control.AssociationStore
	commands               *control.CommandIngestStore
	convergeTriggers       *control.ConvergeTriggerStore
	exportedResources      *control.ExportedResourceStore
	canaries               *control.CanaryStore
	rules                  *control.RuleEngine
	webhooks               *control.WebhookDispatcher
	alerts                 *control.AlertInbox
	notifications          *control.NotificationRouter
	changeRecords          *control.ChangeRecordStore
	checklists             *control.ChecklistStore
	views                  *control.SavedViewStore
	accessibility          *control.AccessibilityStore
	progressiveDisclosure  *control.ProgressiveDisclosureStore
	shortcuts              *control.UIShortcutCatalog
	dashboardWidgets       *control.DashboardWidgetStore
	bulk                   *control.BulkManager
	actionDocs             *control.ActionDocCatalog
	objectModel            *control.ObjectModelRegistry
	moduleScaffold         *control.ModuleScaffoldCatalog
	migrations             *control.MigrationStore
	solutionPacks          *control.SolutionPackCatalog
	useCaseTemplates       *control.UseCaseTemplateCatalog
	workspaceTemplates     *control.WorkspaceTemplateCatalog
	channels               *control.ChannelManager
	dependencyUpdates      *control.DependencyUpdateStore
	flakes                 *control.FlakeQuarantineStore
	scenarioTests          *control.ScenarioTestStore
	providerConformance    *control.ProviderConformanceStore
	ephemeralTestEnv       *control.EphemeralEnvironmentStore
	chaosExperiments       *control.ChaosExperimentStore
	leakDetection          *control.LeakDetectionStore
	performanceGates       *control.PerformanceGateStore
	loadSoak               *control.LoadSoakStore
	readinessScorecards    *control.ReadinessScorecardStore
	mutationTests          *control.MutationStore
	propertyHarness        *control.PropertyHarnessStore
	modulePolicyHarness    *control.ModulePolicyHarnessStore
	styleAnalyzer          *control.StyleAnalyzer
	providerCatalog        *control.ProviderCatalog
	providerProtocols      *control.ProviderProtocolStore
	healthProbes           *control.HealthProbeStore
	canaryUpgrades         *control.CanaryUpgradeStore
	upgradeOrchestration   *control.UpgradeOrchestrationStore
	failoverDrills         *control.RegionalFailoverDrillStore
	performanceDiagnostics *control.PerformanceDiagnosticsStore
	topologyPlacement      *control.TopologyPlacementStore
	federation             *control.FederationStore
	schedulerPartitions    *control.SchedulerPartitionStore
	workerAutoscaling      *control.WorkerAutoscalingStore
	costScheduling         *control.CostSchedulingStore
	artifactDistribution   *control.ArtifactDistributionStore
	workspaceIsolation     *control.WorkspaceIsolationStore
	tenantCrypto           *control.TenantCryptoStore
	delegatedAdmin         *control.DelegatedAdminStore
	tenantLimits           *control.TenantLimitStore
	schemaMigs             *control.SchemaMigrationManager
	openSchemas            *control.OpenSchemaStore
	dataBags               *control.DataBagStore
	roleEnv                *control.RoleEnvironmentStore
	encryptedVars          *control.EncryptedVariableStore
	facts                  *control.FactCache
	varSources             *control.VariableSourceRegistry
	discoveryInventory     *control.DiscoveryInventoryStore
	inventoryDrift         *control.InventoryDriftStore
	policyModes            *control.PolicyEnforcementStore
	encProviders           *control.ENCProviderStore
	nodeClassification     *control.NodeClassificationStore
	plugins                *control.PluginExtensionStore
	eventBus               *control.EventBus
	nodes                  *control.NodeLifecycleStore
	gitopsPreviews         *control.GitOpsPreviewStore
	gitopsPromotions       *control.GitOpsPromotionStore
	gitopsEnvironments     *control.GitOpsEnvironmentStore
	deployments            *control.DeploymentStore
	rolloutControls        *control.RolloutControlStore
	fileSync               *control.FileSyncStore
	agentCheckins          *control.AgentCheckinStore
	agentDispatch          *control.AgentDispatchStore
	proxyMinions           *control.ProxyMinionStore
	networkTransports      *control.NetworkTransportCatalog
	portableRunners        *control.PortableRunnerCatalog
	nativeSchedulers       *control.NativeSchedulerCatalog
	adaptiveConcurrency    *control.AdaptiveConcurrencyStore
	disruptionBudgets      *control.DisruptionBudgetStore
	executionEnvs          *control.ExecutionEnvironmentStore
	executionCreds         *control.ExecutionCredentialStore
	packageManagers        *control.PackageManagerAbstractionStore
	systemdUnits           *control.SystemdUnitStore
	rebootOrchestration    *control.RebootOrchestrationStore
	patchManagement        *control.PatchManagementStore
	imageBaking            *control.ImageBakeStore
	artifactDeployments    *control.ArtifactDeploymentStore
	sessionRecordings      *control.SessionRecordingStore
	masterless             *control.MasterlessStore
	hopRelay               *control.HopRelayStore
	syndic                 *control.SyndicStore
	fipsMode               *control.FIPSModeStore
	hostSecurityProfiles   *control.HostSecurityProfileStore
	signatureAdmission     *control.SignatureAdmissionStore
	runtimeSecrets         *control.RuntimeSecretStore
	encryptedSecrets       *control.EncryptedSecretStore
	delegationTokens       *control.DelegationTokenStore
	accessApprovals        *control.AccessApprovalStore
	jitGrants              *control.JITAccessGrantStore
	compliance             *control.ComplianceStore
	rbac                   *control.RBACStore
	abac                   *control.ABACStore
	identity               *control.IdentityStore
	scim                   *control.SCIMStore
	oidcWorkload           *control.OIDCWorkloadStore
	mtls                   *control.MTLSStore
	secretIntegrations     *control.SecretsIntegrationStore
	packagePinning         *control.PackagePinStore
	packageRegistry        *control.PackageRegistryStore
	contentChannels        *control.ContentChannelStore
	agentPKI               *control.AgentPKIStore
	agentCatalogs          *control.AgentCatalogStore
	agentAttestation       *control.AgentAttestationStore
	driftPolicies          *control.DriftPolicyStore
	policyBundles          *control.PolicyBundleStore
	policyPull             *control.PolicyPullStore
	multiMaster            *control.MultiMasterStore
	edgeRelay              *control.EdgeRelayStore
	offline                *control.OfflineStore
	objectStore            storage.ObjectStore
	events                 *control.EventStore
	runCancel              context.CancelFunc
	metricsMu              sync.Mutex
	metrics                map[string]int64

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
	runLeases := control.NewRunLeaseStore()
	stepSnapshots := control.NewStepSnapshotStore(20_000)
	executionLocks := control.NewExecutionLockStore()
	checkpoints := control.NewExecutionCheckpointStore()
	runCtx, runCancel := context.WithCancel(context.Background())
	queue.StartWorker(runCtx, runner)
	scheduler := control.NewScheduler(queue)
	templates := control.NewTemplateStore()
	wizards := control.NewWorkflowWizardCatalog()
	tasks := control.NewTaskFrameworkStore()
	workflows := control.NewWorkflowStore(queue, templates)
	runbooks := control.NewRunbookStore()
	assocs := control.NewAssociationStore(scheduler)
	commands := control.NewCommandIngestStore(5000)
	convergeTriggers := control.NewConvergeTriggerStore(5000)
	exportedResources := control.NewExportedResourceStore(5000)
	canaries := control.NewCanaryStore(queue)
	rules := control.NewRuleEngine()
	webhooks := control.NewWebhookDispatcher(5000)
	alerts := control.NewAlertInbox()
	notifications := control.NewNotificationRouter(5000)
	changeRecords := control.NewChangeRecordStore()
	checklists := control.NewChecklistStore()
	views := control.NewSavedViewStore()
	accessibility := control.NewAccessibilityStore()
	progressiveDisclosure := control.NewProgressiveDisclosureStore()
	shortcuts := control.NewUIShortcutCatalog()
	dashboardWidgets := control.NewDashboardWidgetStore()
	bulk := control.NewBulkManager(15 * time.Minute)
	actionDocs := control.NewActionDocCatalog()
	objectModel := control.NewObjectModelRegistry()
	moduleScaffold := control.NewModuleScaffoldCatalog()
	migrations := control.NewMigrationStore()
	solutionPacks := control.NewSolutionPackCatalog()
	useCaseTemplates := control.NewUseCaseTemplateCatalog()
	workspaceTemplates := control.NewWorkspaceTemplateCatalog()
	channels := control.NewChannelManager()
	dependencyUpdates := control.NewDependencyUpdateStore()
	flakes := control.NewFlakeQuarantineStore()
	scenarioTests := control.NewScenarioTestStore()
	providerConformance := control.NewProviderConformanceStore()
	ephemeralTestEnv := control.NewEphemeralEnvironmentStore()
	chaosExperiments := control.NewChaosExperimentStore()
	leakDetection := control.NewLeakDetectionStore()
	performanceGates := control.NewPerformanceGateStore()
	loadSoak := control.NewLoadSoakStore()
	readinessScorecards := control.NewReadinessScorecardStore()
	mutationTests := control.NewMutationStore()
	propertyHarness := control.NewPropertyHarnessStore()
	modulePolicyHarness := control.NewModulePolicyHarnessStore()
	styleAnalyzer := control.NewStyleAnalyzer()
	providerCatalog := control.NewProviderCatalog()
	providerProtocols := control.NewProviderProtocolStore()
	healthProbes := control.NewHealthProbeStore()
	canaryUpgrades := control.NewCanaryUpgradeStore()
	upgradeOrchestration := control.NewUpgradeOrchestrationStore()
	failoverDrills := control.NewRegionalFailoverDrillStore()
	performanceDiagnostics := control.NewPerformanceDiagnosticsStore()
	topologyPlacement := control.NewTopologyPlacementStore()
	federation := control.NewFederationStore()
	schedulerPartitions := control.NewSchedulerPartitionStore()
	workerAutoscaling := control.NewWorkerAutoscalingStore()
	costScheduling := control.NewCostSchedulingStore()
	artifactDistribution := control.NewArtifactDistributionStore()
	workspaceIsolation := control.NewWorkspaceIsolationStore()
	tenantCrypto := control.NewTenantCryptoStore()
	delegatedAdmin := control.NewDelegatedAdminStore()
	tenantLimits := control.NewTenantLimitStore()
	schemaMigs := control.NewSchemaMigrationManager(1)
	openSchemas := control.NewOpenSchemaStore()
	dataBags := control.NewDataBagStore()
	roleEnv := control.NewRoleEnvironmentStore(baseDir)
	encryptedVars := control.NewEncryptedVariableStore(baseDir)
	facts := control.NewFactCache(5 * time.Minute)
	varSources := control.NewVariableSourceRegistry(baseDir)
	discoveryInventory := control.NewDiscoveryInventoryStore()
	inventoryDrift := control.NewInventoryDriftStore()
	policyModes := control.NewPolicyEnforcementStore()
	encProviders := control.NewENCProviderStore()
	nodeClassification := control.NewNodeClassificationStore()
	plugins := control.NewPluginExtensionStore()
	eventBus := control.NewEventBus()
	nodes := control.NewNodeLifecycleStore()
	gitopsPreviews := control.NewGitOpsPreviewStore()
	gitopsPromotions := control.NewGitOpsPromotionStore()
	gitopsEnvironments := control.NewGitOpsEnvironmentStore()
	deployments := control.NewDeploymentStore()
	rolloutControls := control.NewRolloutControlStore()
	fileSync := control.NewFileSyncStore()
	agentCheckins := control.NewAgentCheckinStore()
	agentDispatch := control.NewAgentDispatchStore()
	proxyMinions := control.NewProxyMinionStore()
	networkTransports := control.NewNetworkTransportCatalog()
	portableRunners := control.NewPortableRunnerCatalog()
	nativeSchedulers := control.NewNativeSchedulerCatalog()
	adaptiveConcurrency := control.NewAdaptiveConcurrencyStore()
	disruptionBudgets := control.NewDisruptionBudgetStore()
	executionEnvs := control.NewExecutionEnvironmentStore()
	executionCreds := control.NewExecutionCredentialStore()
	packageManagers := control.NewPackageManagerAbstractionStore()
	systemdUnits := control.NewSystemdUnitStore()
	rebootOrchestration := control.NewRebootOrchestrationStore()
	patchManagement := control.NewPatchManagementStore()
	imageBaking := control.NewImageBakeStore()
	artifactDeployments := control.NewArtifactDeploymentStore()
	sessionRecordings := control.NewSessionRecordingStore(baseDir)
	masterless := control.NewMasterlessStore()
	hopRelay := control.NewHopRelayStore()
	syndic := control.NewSyndicStore()
	fipsMode := control.NewFIPSModeStore()
	hostSecurityProfiles := control.NewHostSecurityProfileStore()
	signatureAdmission := control.NewSignatureAdmissionStore()
	runtimeSecrets := control.NewRuntimeSecretStore()
	encryptedSecrets := control.NewEncryptedSecretStore()
	delegationTokens := control.NewDelegationTokenStore()
	accessApprovals := control.NewAccessApprovalStore()
	jitGrants := control.NewJITAccessGrantStore()
	compliance := control.NewComplianceStore()
	rbac := control.NewRBACStore()
	abac := control.NewABACStore()
	identity := control.NewIdentityStore()
	scim := control.NewSCIMStore()
	oidcWorkload := control.NewOIDCWorkloadStore()
	mtls := control.NewMTLSStore()
	secretIntegrations := control.NewSecretsIntegrationStore()
	packagePinning := control.NewPackagePinStore()
	packageRegistry := control.NewPackageRegistryStore()
	contentChannels := control.NewContentChannelStore()
	agentPKI := control.NewAgentPKIStore()
	agentCatalogs := control.NewAgentCatalogStore()
	agentAttestation := control.NewAgentAttestationStore()
	driftPolicies := control.NewDriftPolicyStore()
	policyBundles := control.NewPolicyBundleStore()
	policyPull := control.NewPolicyPullStore()
	multiMaster := control.NewMultiMasterStore()
	edgeRelay := control.NewEdgeRelayStore()
	offline := control.NewOfflineStore()
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
		baseDir:                baseDir,
		queue:                  queue,
		runLeases:              runLeases,
		stepSnapshots:          stepSnapshots,
		executionLocks:         executionLocks,
		checkpoints:            checkpoints,
		scheduler:              scheduler,
		templates:              templates,
		wizards:                wizards,
		tasks:                  tasks,
		workflows:              workflows,
		runbooks:               runbooks,
		assocs:                 assocs,
		commands:               commands,
		convergeTriggers:       convergeTriggers,
		exportedResources:      exportedResources,
		canaries:               canaries,
		rules:                  rules,
		webhooks:               webhooks,
		alerts:                 alerts,
		notifications:          notifications,
		changeRecords:          changeRecords,
		checklists:             checklists,
		views:                  views,
		accessibility:          accessibility,
		progressiveDisclosure:  progressiveDisclosure,
		shortcuts:              shortcuts,
		dashboardWidgets:       dashboardWidgets,
		bulk:                   bulk,
		actionDocs:             actionDocs,
		objectModel:            objectModel,
		moduleScaffold:         moduleScaffold,
		migrations:             migrations,
		solutionPacks:          solutionPacks,
		useCaseTemplates:       useCaseTemplates,
		workspaceTemplates:     workspaceTemplates,
		channels:               channels,
		dependencyUpdates:      dependencyUpdates,
		flakes:                 flakes,
		scenarioTests:          scenarioTests,
		providerConformance:    providerConformance,
		ephemeralTestEnv:       ephemeralTestEnv,
		chaosExperiments:       chaosExperiments,
		leakDetection:          leakDetection,
		performanceGates:       performanceGates,
		loadSoak:               loadSoak,
		readinessScorecards:    readinessScorecards,
		mutationTests:          mutationTests,
		propertyHarness:        propertyHarness,
		modulePolicyHarness:    modulePolicyHarness,
		styleAnalyzer:          styleAnalyzer,
		providerCatalog:        providerCatalog,
		providerProtocols:      providerProtocols,
		healthProbes:           healthProbes,
		canaryUpgrades:         canaryUpgrades,
		upgradeOrchestration:   upgradeOrchestration,
		failoverDrills:         failoverDrills,
		performanceDiagnostics: performanceDiagnostics,
		topologyPlacement:      topologyPlacement,
		federation:             federation,
		schedulerPartitions:    schedulerPartitions,
		workerAutoscaling:      workerAutoscaling,
		costScheduling:         costScheduling,
		artifactDistribution:   artifactDistribution,
		workspaceIsolation:     workspaceIsolation,
		tenantCrypto:           tenantCrypto,
		delegatedAdmin:         delegatedAdmin,
		tenantLimits:           tenantLimits,
		schemaMigs:             schemaMigs,
		openSchemas:            openSchemas,
		dataBags:               dataBags,
		roleEnv:                roleEnv,
		encryptedVars:          encryptedVars,
		facts:                  facts,
		varSources:             varSources,
		discoveryInventory:     discoveryInventory,
		inventoryDrift:         inventoryDrift,
		policyModes:            policyModes,
		encProviders:           encProviders,
		nodeClassification:     nodeClassification,
		plugins:                plugins,
		eventBus:               eventBus,
		nodes:                  nodes,
		gitopsPreviews:         gitopsPreviews,
		gitopsPromotions:       gitopsPromotions,
		gitopsEnvironments:     gitopsEnvironments,
		deployments:            deployments,
		rolloutControls:        rolloutControls,
		fileSync:               fileSync,
		agentCheckins:          agentCheckins,
		agentDispatch:          agentDispatch,
		proxyMinions:           proxyMinions,
		networkTransports:      networkTransports,
		portableRunners:        portableRunners,
		nativeSchedulers:       nativeSchedulers,
		adaptiveConcurrency:    adaptiveConcurrency,
		disruptionBudgets:      disruptionBudgets,
		executionEnvs:          executionEnvs,
		executionCreds:         executionCreds,
		packageManagers:        packageManagers,
		systemdUnits:           systemdUnits,
		rebootOrchestration:    rebootOrchestration,
		patchManagement:        patchManagement,
		imageBaking:            imageBaking,
		artifactDeployments:    artifactDeployments,
		sessionRecordings:      sessionRecordings,
		masterless:             masterless,
		hopRelay:               hopRelay,
		syndic:                 syndic,
		fipsMode:               fipsMode,
		hostSecurityProfiles:   hostSecurityProfiles,
		signatureAdmission:     signatureAdmission,
		runtimeSecrets:         runtimeSecrets,
		encryptedSecrets:       encryptedSecrets,
		delegationTokens:       delegationTokens,
		accessApprovals:        accessApprovals,
		jitGrants:              jitGrants,
		compliance:             compliance,
		rbac:                   rbac,
		abac:                   abac,
		identity:               identity,
		scim:                   scim,
		oidcWorkload:           oidcWorkload,
		mtls:                   mtls,
		secretIntegrations:     secretIntegrations,
		packagePinning:         packagePinning,
		packageRegistry:        packageRegistry,
		contentChannels:        contentChannels,
		agentPKI:               agentPKI,
		agentCatalogs:          agentCatalogs,
		agentAttestation:       agentAttestation,
		driftPolicies:          driftPolicies,
		policyBundles:          policyBundles,
		policyPull:             policyPull,
		multiMaster:            multiMaster,
		edgeRelay:              edgeRelay,
		offline:                offline,
		objectStore:            objectStore,
		events:                 events,
		metrics:                map[string]int64{},
		runCancel:              runCancel,
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
		if job.Status == control.JobSucceeded || job.Status == control.JobFailed || job.Status == control.JobCanceled {
			if released, ok := s.executionLocks.Release(control.ExecutionLockReleaseInput{JobID: job.ID}); ok {
				s.recordEvent(control.Event{
					Type:    "execution.lock.released",
					Message: "execution lock released after job completion",
					Fields: map[string]any{
						"job_id":   job.ID,
						"lock_key": released.Key,
					},
				}, true)
			}
		}
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
	mux.HandleFunc("/v1/tasks/definitions", s.handleTaskDefinitions)
	mux.HandleFunc("/v1/tasks/definitions/", s.handleTaskDefinitionByID)
	mux.HandleFunc("/v1/wizards", s.handleWorkflowWizards)
	mux.HandleFunc("/v1/wizards/", s.handleWorkflowWizardAction)
	mux.HandleFunc("/v1/tasks/plans", s.handleTaskPlans)
	mux.HandleFunc("/v1/tasks/plans/", s.handleTaskPlanAction)
	mux.HandleFunc("/v1/tasks/executions", s.handleTaskExecutions)
	mux.HandleFunc("/v1/tasks/executions/", s.handleTaskExecutionAction)
	mux.HandleFunc("/v1/edge-relay/sites", s.handleEdgeRelaySites)
	mux.HandleFunc("/v1/edge-relay/sites/", s.handleEdgeRelaySiteAction)
	mux.HandleFunc("/v1/edge-relay/messages", s.handleEdgeRelayMessages)
	mux.HandleFunc("/v1/offline/mode", s.handleOfflineMode)
	mux.HandleFunc("/v1/offline/bundles", s.handleOfflineBundles(baseDir))
	mux.HandleFunc("/v1/offline/bundles/verify", s.handleOfflineBundleVerify(baseDir))
	mux.HandleFunc("/v1/offline/mirrors", s.handleOfflineMirrors)
	mux.HandleFunc("/v1/offline/mirrors/", s.handleOfflineMirrorAction)
	mux.HandleFunc("/v1/offline/mirrors/sync", s.handleOfflineMirrorSync)
	mux.HandleFunc("/v1/docs/actions", s.handleActionDocs)
	mux.HandleFunc("/v1/docs/actions/", s.handleActionDocByID)
	mux.HandleFunc("/v1/model/objects", s.handleObjectModel)
	mux.HandleFunc("/v1/model/objects/resolve", s.handleObjectModelResolve)
	mux.HandleFunc("/v1/docs/inline", s.handleInlineDocs)
	mux.HandleFunc("/v1/docs/generate", s.handleDocsGenerate)
	mux.HandleFunc("/v1/docs/examples/verify", s.handleDocsExampleVerify)
	mux.HandleFunc("/v1/docs/api/version-diff", s.handleDocsAPIVersionDiff)
	mux.HandleFunc("/v1/lint/style/rules", s.handleStyleAnalyzerRules)
	mux.HandleFunc("/v1/lint/style/analyze", s.handleStyleAnalyzerAnalyze)
	mux.HandleFunc("/v1/format/canonicalize", s.handleCanonicalize)
	mux.HandleFunc("/v1/release/readiness", s.handleReleaseReadiness)
	mux.HandleFunc("/v1/release/readiness/scorecards", s.handleReadinessScorecards)
	mux.HandleFunc("/v1/release/readiness/scorecards/", s.handleReadinessScorecardAction)
	mux.HandleFunc("/v1/release/blocker-policy", s.handleReleaseBlockerPolicy)
	mux.HandleFunc("/v1/release/api-contract", s.handleAPIContract)
	mux.HandleFunc("/v1/release/upgrade-assistant", s.handleUpgradeAssistant)
	mux.HandleFunc("/v1/release/dependency-bot/policy", s.handleDependencyUpdatePolicy)
	mux.HandleFunc("/v1/release/dependency-bot/updates", s.handleDependencyUpdates)
	mux.HandleFunc("/v1/release/dependency-bot/updates/", s.handleDependencyUpdateAction)
	mux.HandleFunc("/v1/release/performance-gates/policy", s.handlePerformanceGatePolicy)
	mux.HandleFunc("/v1/release/performance-gates/evaluate", s.handlePerformanceGateEvaluate)
	mux.HandleFunc("/v1/release/performance-gates/evaluations", s.handlePerformanceGateEvaluations)
	mux.HandleFunc("/v1/release/tests/flake-policy", s.handleFlakePolicy)
	mux.HandleFunc("/v1/release/tests/flake-observations", s.handleFlakeObservations)
	mux.HandleFunc("/v1/release/tests/flake-cases", s.handleFlakeCases)
	mux.HandleFunc("/v1/release/tests/flake-cases/", s.handleFlakeCaseAction)
	mux.HandleFunc("/v1/release/tests/impact-analysis", s.handleTestImpactAnalysis)
	mux.HandleFunc("/v1/release/tests/scenarios", s.handleTestScenarios)
	mux.HandleFunc("/v1/release/tests/scenario-runs", s.handleTestScenarioRuns)
	mux.HandleFunc("/v1/release/tests/scenario-runs/", s.handleTestScenarioRunAction)
	mux.HandleFunc("/v1/release/tests/scenario-baselines", s.handleTestScenarioBaselines)
	mux.HandleFunc("/v1/release/tests/scenario-baselines/", s.handleTestScenarioBaselineAction)
	mux.HandleFunc("/v1/release/tests/environments", s.handleTestEnvironments)
	mux.HandleFunc("/v1/release/tests/environments/", s.handleTestEnvironmentAction)
	mux.HandleFunc("/v1/release/tests/load-soak/suites", s.handleLoadSoakSuites)
	mux.HandleFunc("/v1/release/tests/load-soak/runs", s.handleLoadSoakRuns)
	mux.HandleFunc("/v1/release/tests/load-soak/runs/", s.handleLoadSoakRunAction)
	mux.HandleFunc("/v1/release/tests/mutation/policy", s.handleMutationPolicy)
	mux.HandleFunc("/v1/release/tests/mutation/suites", s.handleMutationSuites)
	mux.HandleFunc("/v1/release/tests/mutation/runs", s.handleMutationRuns)
	mux.HandleFunc("/v1/release/tests/mutation/runs/", s.handleMutationRunAction)
	mux.HandleFunc("/v1/release/tests/property-harness/cases", s.handlePropertyHarnessCases)
	mux.HandleFunc("/v1/release/tests/property-harness/runs", s.handlePropertyHarnessRuns)
	mux.HandleFunc("/v1/release/tests/property-harness/runs/", s.handlePropertyHarnessRunAction)
	mux.HandleFunc("/v1/release/tests/harness/cases", s.handleModulePolicyHarnessCases)
	mux.HandleFunc("/v1/release/tests/harness/runs", s.handleModulePolicyHarnessRuns)
	mux.HandleFunc("/v1/release/tests/harness/runs/", s.handleModulePolicyHarnessRunAction)
	mux.HandleFunc("/v1/providers/conformance/suites", s.handleProviderConformanceSuites)
	mux.HandleFunc("/v1/providers/conformance/runs", s.handleProviderConformanceRuns)
	mux.HandleFunc("/v1/providers/conformance/runs/", s.handleProviderConformanceRunAction)
	mux.HandleFunc("/v1/providers/catalog", s.handleProviderCatalog)
	mux.HandleFunc("/v1/providers/catalog/validate", s.handleProviderCatalogValidate)
	mux.HandleFunc("/v1/providers/protocol/descriptors", s.handleProviderProtocolDescriptors)
	mux.HandleFunc("/v1/providers/protocol/negotiate", s.handleProviderProtocolNegotiate)
	mux.HandleFunc("/v1/plans/explain", s.handlePlanExplain(baseDir))
	mux.HandleFunc("/v1/plans/graph", s.handlePlanGraph(baseDir))
	mux.HandleFunc("/v1/plans/graph/query", s.handlePlanGraphQuery(baseDir))
	mux.HandleFunc("/v1/plans/diff-preview", s.handlePlanDiffPreview(baseDir))
	mux.HandleFunc("/v1/plans/reproducibility-check", s.handlePlanReproducibility(baseDir))
	mux.HandleFunc("/v1/plans/risk-summary", s.handlePlanRiskSummary(baseDir))
	mux.HandleFunc("/v1/policy/simulate", s.handlePolicySimulation(baseDir))
	mux.HandleFunc("/v1/policy/enforcement-modes", s.handlePolicyEnforcementModes)
	mux.HandleFunc("/v1/policy/enforcement-modes/", s.handlePolicyEnforcementModeAction)
	mux.HandleFunc("/v1/policy/pull/sources", s.handlePolicyPullSources)
	mux.HandleFunc("/v1/policy/pull/sources/", s.handlePolicyPullSourceAction)
	mux.HandleFunc("/v1/policy/pull/execute", s.handlePolicyPullExecute(baseDir))
	mux.HandleFunc("/v1/policy/pull/results", s.handlePolicyPullResults)
	mux.HandleFunc("/v1/policy/bundles", s.handlePolicyBundles)
	mux.HandleFunc("/v1/policy/bundles/", s.handlePolicyBundleAction)
	mux.HandleFunc("/v1/query", s.handleQuery(baseDir))
	mux.HandleFunc("/v1/search", s.handleSearch(baseDir))
	mux.HandleFunc("/v1/inventory/groups", s.handleInventoryGroups(baseDir))
	mux.HandleFunc("/v1/inventory/import/cmdb", s.handleInventoryCMDBImport)
	mux.HandleFunc("/v1/inventory/import/assist", s.handleInventoryImportAssistant)
	mux.HandleFunc("/v1/inventory/import/brownfield-bootstrap", s.handleInventoryBrownfieldBootstrap)
	mux.HandleFunc("/v1/inventory/drift/analyze", s.handleInventoryDriftAnalyze)
	mux.HandleFunc("/v1/inventory/drift/reconcile", s.handleInventoryDriftReconcile)
	mux.HandleFunc("/v1/inventory/drift/reports", s.handleInventoryDriftReports)
	mux.HandleFunc("/v1/inventory/classification-rules", s.handleNodeClassificationRules)
	mux.HandleFunc("/v1/inventory/classification-rules/", s.handleNodeClassificationRuleByID)
	mux.HandleFunc("/v1/inventory/classify", s.handleNodeClassify)
	mux.HandleFunc("/v1/inventory/node-classifiers", s.handleENCProviders)
	mux.HandleFunc("/v1/inventory/node-classifiers/classify", s.handleENCClassify)
	mux.HandleFunc("/v1/inventory/node-classifiers/", s.handleENCProviderAction)
	mux.HandleFunc("/v1/compat/grains", s.handleCompatGrains)
	mux.HandleFunc("/v1/compat/grains/query", s.handleCompatGrainsQuery)
	mux.HandleFunc("/v1/inventory/discovery-sources", s.handleDiscoverySources)
	mux.HandleFunc("/v1/inventory/discovery-sources/sync", s.handleDiscoverySourceSync)
	mux.HandleFunc("/v1/inventory/cloud-sync", s.handleCloudInventorySync)
	mux.HandleFunc("/v1/inventory/discovery-sources/", s.handleDiscoverySourceAction)
	mux.HandleFunc("/v1/inventory/runtime-hosts", s.handleRuntimeHosts)
	mux.HandleFunc("/v1/inventory/runtime-hosts/", s.handleRuntimeHostAction)
	mux.HandleFunc("/v1/inventory/enroll", s.handleRuntimeEnrollAlias)
	mux.HandleFunc("/v1/fleet/health", s.handleFleetHealth(baseDir))
	mux.HandleFunc("/v1/agents/checkins", s.handleAgentCheckins)
	mux.HandleFunc("/v1/agents/dispatch-mode", s.handleAgentDispatchMode)
	mux.HandleFunc("/v1/agents/dispatch-environments", s.handleAgentDispatchEnvironments)
	mux.HandleFunc("/v1/agents/dispatch-environments/", s.handleAgentDispatchEnvironmentAction)
	mux.HandleFunc("/v1/agents/dispatch", s.handleAgentDispatch(baseDir))
	mux.HandleFunc("/v1/agents/proxy-minions", s.handleProxyMinions)
	mux.HandleFunc("/v1/agents/proxy-minions/", s.handleProxyMinionAction)
	mux.HandleFunc("/v1/agents/proxy-minions/dispatch", s.handleProxyMinionDispatch(baseDir))
	mux.HandleFunc("/v1/execution/network-transports", s.handleNetworkTransports)
	mux.HandleFunc("/v1/execution/network-transports/validate", s.handleNetworkTransportValidate)
	mux.HandleFunc("/v1/execution/portable-runners", s.handlePortableRunners)
	mux.HandleFunc("/v1/execution/portable-runners/select", s.handlePortableRunnerSelect)
	mux.HandleFunc("/v1/execution/native-schedulers", s.handleNativeSchedulers)
	mux.HandleFunc("/v1/execution/native-schedulers/select", s.handleNativeSchedulerSelect)
	mux.HandleFunc("/v1/execution/package-managers", s.handlePackageManagers)
	mux.HandleFunc("/v1/execution/package-managers/resolve", s.handlePackageManagerResolve)
	mux.HandleFunc("/v1/execution/package-managers/render-action", s.handlePackageManagerRenderAction)
	mux.HandleFunc("/v1/execution/systemd/units", s.handleSystemdUnits)
	mux.HandleFunc("/v1/execution/systemd/units/", s.handleSystemdUnitAction)
	mux.HandleFunc("/v1/execution/systemd/units/render", s.handleSystemdRender)
	mux.HandleFunc("/v1/execution/reboot/policies", s.handleRebootPolicies)
	mux.HandleFunc("/v1/execution/reboot/plan", s.handleRebootPlan)
	mux.HandleFunc("/v1/execution/patch/policies", s.handlePatchPolicies)
	mux.HandleFunc("/v1/execution/patch/plan", s.handlePatchPlan)
	mux.HandleFunc("/v1/execution/image-baking/pipelines", s.handleImageBakePipelines)
	mux.HandleFunc("/v1/execution/image-baking/pipelines/", s.handleImageBakePipelineAction)
	mux.HandleFunc("/v1/execution/artifacts/deployments", s.handleArtifactDeployments)
	mux.HandleFunc("/v1/execution/artifacts/deployments/", s.handleArtifactDeploymentAction)
	mux.HandleFunc("/v1/execution/session-recordings", s.handleSessionRecordings)
	mux.HandleFunc("/v1/execution/session-recordings/", s.handleSessionRecordingAction)
	mux.HandleFunc("/v1/execution/adaptive-concurrency/policy", s.handleAdaptiveConcurrencyPolicy)
	mux.HandleFunc("/v1/execution/adaptive-concurrency/recommend", s.handleAdaptiveConcurrencyRecommend)
	mux.HandleFunc("/v1/execution/checkpoints", s.handleExecutionCheckpoints(baseDir))
	mux.HandleFunc("/v1/execution/checkpoints/resume", s.handleExecutionCheckpointResume(baseDir))
	mux.HandleFunc("/v1/execution/checkpoints/", s.handleExecutionCheckpointByID)
	mux.HandleFunc("/v1/execution/snapshots", s.handleStepSnapshots)
	mux.HandleFunc("/v1/execution/snapshots/", s.handleStepSnapshotByID)
	mux.HandleFunc("/v1/execution/environments", s.handleExecutionEnvironments)
	mux.HandleFunc("/v1/execution/environments/", s.handleExecutionEnvironmentAction)
	mux.HandleFunc("/v1/execution/admission-policy", s.handleExecutionAdmissionPolicy)
	mux.HandleFunc("/v1/execution/admit-check", s.handleExecutionAdmissionCheck)
	mux.HandleFunc("/v1/execution/credentials", s.handleExecutionCredentials)
	mux.HandleFunc("/v1/execution/credentials/validate", s.handleExecutionCredentialValidate)
	mux.HandleFunc("/v1/execution/credentials/", s.handleExecutionCredentialAction)
	mux.HandleFunc("/v1/execution/masterless/mode", s.handleMasterlessMode)
	mux.HandleFunc("/v1/execution/masterless/render", s.handleMasterlessRender)
	mux.HandleFunc("/v1/execution/relays/endpoints", s.handleRelayEndpoints)
	mux.HandleFunc("/v1/execution/relays/endpoints/", s.handleRelayEndpointAction)
	mux.HandleFunc("/v1/execution/relays/sessions", s.handleRelaySessions)
	mux.HandleFunc("/v1/control/syndic/nodes", s.handleSyndicNodes)
	mux.HandleFunc("/v1/control/syndic/route", s.handleSyndicRoute)
	mux.HandleFunc("/v1/security/crypto/fips-mode", s.handleFIPSMode)
	mux.HandleFunc("/v1/security/crypto/fips/validate", s.handleFIPSValidate)
	mux.HandleFunc("/v1/security/host-profiles", s.handleHostSecurityProfiles)
	mux.HandleFunc("/v1/security/host-profiles/evaluate", s.handleHostSecurityEvaluate)
	mux.HandleFunc("/v1/security/signatures/keyrings", s.handleSignatureKeyrings)
	mux.HandleFunc("/v1/security/signatures/keyrings/", s.handleSignatureKeyringAction)
	mux.HandleFunc("/v1/security/signatures/admission-policy", s.handleSignatureAdmissionPolicy)
	mux.HandleFunc("/v1/security/signatures/admit-check", s.handleSignatureAdmissionCheck)
	mux.HandleFunc("/v1/secrets/runtime/sessions", s.handleRuntimeSecretSessions)
	mux.HandleFunc("/v1/secrets/runtime/sessions/", s.handleRuntimeSecretSessionAction)
	mux.HandleFunc("/v1/secrets/runtime/consume", s.handleRuntimeSecretConsume)
	mux.HandleFunc("/v1/secrets/encrypted-store/items", s.handleEncryptedSecrets)
	mux.HandleFunc("/v1/secrets/encrypted-store/items/", s.handleEncryptedSecretAction)
	mux.HandleFunc("/v1/secrets/encrypted-store/expired", s.handleEncryptedSecretExpired)
	mux.HandleFunc("/v1/access/delegation-tokens", s.handleDelegationTokens)
	mux.HandleFunc("/v1/access/delegation-tokens/validate", s.handleDelegationTokenValidate)
	mux.HandleFunc("/v1/access/delegation-tokens/", s.handleDelegationTokenAction)
	mux.HandleFunc("/v1/access/approval-policies", s.handleApprovalPolicies)
	mux.HandleFunc("/v1/access/approval-policies/", s.handleApprovalPolicyAction)
	mux.HandleFunc("/v1/access/break-glass/requests", s.handleBreakGlassRequests)
	mux.HandleFunc("/v1/access/break-glass/requests/", s.handleBreakGlassRequestAction)
	mux.HandleFunc("/v1/access/jit-grants", s.handleJITAccessGrants)
	mux.HandleFunc("/v1/access/jit-grants/validate", s.handleJITAccessGrantValidate)
	mux.HandleFunc("/v1/access/jit-grants/", s.handleJITAccessGrantAction)
	mux.HandleFunc("/v1/access/rbac/roles", s.handleRBACRoles)
	mux.HandleFunc("/v1/access/rbac/roles/", s.handleRBACRoleAction)
	mux.HandleFunc("/v1/access/rbac/bindings", s.handleRBACBindings)
	mux.HandleFunc("/v1/access/rbac/check", s.handleRBACAccessCheck)
	mux.HandleFunc("/v1/access/abac/policies", s.handleABACPolicies)
	mux.HandleFunc("/v1/access/abac/check", s.handleABACCheck)
	mux.HandleFunc("/v1/identity/sso/providers", s.handleSSOProviders)
	mux.HandleFunc("/v1/identity/sso/providers/", s.handleSSOProviderAction)
	mux.HandleFunc("/v1/identity/sso/login/start", s.handleSSOLoginStart)
	mux.HandleFunc("/v1/identity/sso/login/callback", s.handleSSOLoginCallback)
	mux.HandleFunc("/v1/identity/sso/sessions", s.handleSSOSessions)
	mux.HandleFunc("/v1/identity/sso/sessions/", s.handleSSOSessionAction)
	mux.HandleFunc("/v1/identity/scim/roles", s.handleSCIMRoles)
	mux.HandleFunc("/v1/identity/scim/roles/", s.handleSCIMRoleAction)
	mux.HandleFunc("/v1/identity/scim/teams", s.handleSCIMTeams)
	mux.HandleFunc("/v1/identity/scim/teams/", s.handleSCIMTeamAction)
	mux.HandleFunc("/v1/identity/oidc/workload/providers", s.handleOIDCWorkloadProviders)
	mux.HandleFunc("/v1/identity/oidc/workload/providers/", s.handleOIDCWorkloadProviderAction)
	mux.HandleFunc("/v1/identity/oidc/workload/exchange", s.handleOIDCWorkloadExchange)
	mux.HandleFunc("/v1/identity/oidc/workload/credentials", s.handleOIDCWorkloadCredentials)
	mux.HandleFunc("/v1/identity/oidc/workload/credentials/", s.handleOIDCWorkloadCredentialAction)
	mux.HandleFunc("/v1/security/mtls/authorities", s.handleMTLSAuthorities)
	mux.HandleFunc("/v1/security/mtls/policies", s.handleMTLSPolicies)
	mux.HandleFunc("/v1/security/mtls/handshake-check", s.handleMTLSHandshakeCheck)
	mux.HandleFunc("/v1/secrets/integrations", s.handleSecretIntegrations)
	mux.HandleFunc("/v1/secrets/resolve", s.handleSecretResolve)
	mux.HandleFunc("/v1/secrets/traces", s.handleSecretUsageTraces)
	mux.HandleFunc("/v1/packages/artifacts", s.handlePackageArtifacts)
	mux.HandleFunc("/v1/packages/artifacts/", s.handlePackageArtifactAction)
	mux.HandleFunc("/v1/packages/signing-policy", s.handlePackageSigningPolicy)
	mux.HandleFunc("/v1/packages/verify", s.handlePackageVerify)
	mux.HandleFunc("/v1/packages/certification-policy", s.handlePackageCertificationPolicy)
	mux.HandleFunc("/v1/packages/certify", s.handlePackageCertify)
	mux.HandleFunc("/v1/packages/certifications", s.handlePackageCertifications)
	mux.HandleFunc("/v1/packages/publication/check", s.handlePackagePublicationCheck)
	mux.HandleFunc("/v1/packages/maintainers/health", s.handlePackageMaintainerHealth)
	mux.HandleFunc("/v1/packages/maintainers/health/", s.handlePackageMaintainerHealthAction)
	mux.HandleFunc("/v1/packages/provenance/report", s.handlePackageProvenanceReport)
	mux.HandleFunc("/v1/packages/quality", s.handlePackageQuality)
	mux.HandleFunc("/v1/packages/quality/evaluate", s.handlePackageQualityEvaluate)
	mux.HandleFunc("/v1/packages/content-channels", s.handleContentChannels)
	mux.HandleFunc("/v1/packages/content-channels/sync-policy", s.handleContentChannelPolicy)
	mux.HandleFunc("/v1/packages/content-channels/remotes", s.handleContentChannelRemotes)
	mux.HandleFunc("/v1/packages/content-channels/remotes/", s.handleContentChannelRemoteAction)
	mux.HandleFunc("/v1/packages/scaffold/templates", s.handleModuleScaffoldTemplates)
	mux.HandleFunc("/v1/packages/scaffold/generate", s.handleModuleScaffoldGenerate(baseDir))
	mux.HandleFunc("/v1/packages/interface-compat/analyze", s.handleInterfaceCompatAnalyze)
	mux.HandleFunc("/v1/packages/pinning/policies", s.handlePackagePinPolicies)
	mux.HandleFunc("/v1/packages/pinning/evaluate", s.handlePackagePinEvaluate)
	mux.HandleFunc("/v1/agents/cert-policy", s.handleAgentCertPolicy)
	mux.HandleFunc("/v1/agents/catalogs", s.handleAgentCatalogs(baseDir))
	mux.HandleFunc("/v1/agents/catalogs/replay", s.handleAgentCatalogReplay(baseDir))
	mux.HandleFunc("/v1/agents/catalogs/replays", s.handleAgentCatalogReplays)
	mux.HandleFunc("/v1/agents/catalogs/", s.handleAgentCatalogAction)
	mux.HandleFunc("/v1/agents/attestation/policy", s.handleAgentAttestationPolicy)
	mux.HandleFunc("/v1/agents/attestations", s.handleAgentAttestations)
	mux.HandleFunc("/v1/agents/attestations/check", s.handleAgentAttestationCheck)
	mux.HandleFunc("/v1/agents/attestations/", s.handleAgentAttestationAction)
	mux.HandleFunc("/v1/agents/csrs", s.handleAgentCSRs)
	mux.HandleFunc("/v1/agents/csrs/", s.handleAgentCSRAction)
	mux.HandleFunc("/v1/agents/certificates", s.handleAgentCertificates)
	mux.HandleFunc("/v1/agents/certificates/expiry-report", s.handleAgentCertificateExpiryReport)
	mux.HandleFunc("/v1/agents/certificates/renew-expiring", s.handleAgentCertificateRenewExpiring)
	mux.HandleFunc("/v1/agents/certificates/", s.handleAgentCertificateAction)
	mux.HandleFunc("/v1/agents/certificates/rotate", s.handleAgentCertificateRotate)
	mux.HandleFunc("/v1/compliance/profiles", s.handleComplianceProfiles)
	mux.HandleFunc("/v1/compliance/profiles/", s.handleComplianceProfileAction)
	mux.HandleFunc("/v1/compliance/scans", s.handleComplianceScans)
	mux.HandleFunc("/v1/compliance/scans/", s.handleComplianceScanAction)
	mux.HandleFunc("/v1/compliance/continuous", s.handleComplianceContinuous)
	mux.HandleFunc("/v1/compliance/continuous/", s.handleComplianceContinuousAction)
	mux.HandleFunc("/v1/compliance/exceptions", s.handleComplianceExceptions)
	mux.HandleFunc("/v1/compliance/exceptions/", s.handleComplianceExceptionAction)
	mux.HandleFunc("/v1/compliance/scorecards", s.handleComplianceScorecards)
	mux.HandleFunc("/v1/gitops/previews", s.handleGitOpsPreviews(baseDir))
	mux.HandleFunc("/v1/gitops/previews/", s.handleGitOpsPreviewAction)
	mux.HandleFunc("/v1/gitops/environments", s.handleGitOpsEnvironments(baseDir))
	mux.HandleFunc("/v1/gitops/environments/materialize", s.handleGitOpsEnvironmentMaterializeAlias(baseDir))
	mux.HandleFunc("/v1/gitops/environments/", s.handleGitOpsEnvironmentAction)
	mux.HandleFunc("/v1/gitops/deployments", s.handleGitOpsDeployments(baseDir))
	mux.HandleFunc("/v1/gitops/deployments/trigger", s.handleGitOpsDeploymentTriggerAlias(baseDir, "api"))
	mux.HandleFunc("/v1/gitops/deployments/webhook", s.handleGitOpsDeploymentTriggerAlias(baseDir, "webhook"))
	mux.HandleFunc("/v1/gitops/deployments/", s.handleGitOpsDeploymentAction)
	mux.HandleFunc("/v1/deployments/rollout/policies", s.handleRolloutPolicies)
	mux.HandleFunc("/v1/deployments/rollout/plan", s.handleRolloutPlan)
	mux.HandleFunc("/v1/gitops/filesync/pipelines", s.handleGitOpsFileSyncPipelines)
	mux.HandleFunc("/v1/gitops/filesync/pipelines/", s.handleGitOpsFileSyncPipelineAction)
	mux.HandleFunc("/v1/gitops/promotions", s.handleGitOpsPromotions)
	mux.HandleFunc("/v1/gitops/promotions/", s.handleGitOpsPromotionAction)
	mux.HandleFunc("/v1/gitops/reconcile", s.handleGitOpsReconcile(baseDir))
	mux.HandleFunc("/v1/gitops/plan-artifacts/sign", s.handleGitOpsPlanArtifactSign(baseDir))
	mux.HandleFunc("/v1/gitops/plan-artifacts/verify", s.handleGitOpsPlanArtifactVerify(baseDir))
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
	mux.HandleFunc("/v1/drift/history", s.handleDriftHistory(baseDir))
	mux.HandleFunc("/v1/drift/suppressions", s.handleDriftSuppressions)
	mux.HandleFunc("/v1/drift/suppressions/", s.handleDriftSuppressionByID)
	mux.HandleFunc("/v1/drift/allowlists", s.handleDriftAllowlists)
	mux.HandleFunc("/v1/drift/allowlists/", s.handleDriftAllowlistByID)
	mux.HandleFunc("/v1/drift/remediate", s.handleDriftRemediation(baseDir))
	mux.HandleFunc("/v1/activity", s.handleActivity)
	mux.HandleFunc("/v1/activity/audit-timeline", s.handleAuditTimeline)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/events/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/event-stream/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/event-stream/webhooks/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/converge/triggers", s.handleConvergeTriggers(baseDir))
	mux.HandleFunc("/v1/converge/triggers/", s.handleConvergeTriggerByID)
	mux.HandleFunc("/v1/resources/exported", s.handleExportedResources)
	mux.HandleFunc("/v1/resources/collect", s.handleResourceCollect)
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
	mux.HandleFunc("/v1/ui/accessibility/profiles", s.handleAccessibilityProfiles)
	mux.HandleFunc("/v1/ui/accessibility/active", s.handleAccessibilityActive)
	mux.HandleFunc("/v1/ui/progressive-disclosure", s.handleProgressiveDisclosure)
	mux.HandleFunc("/v1/ui/progressive-disclosure/reveal", s.handleProgressiveDisclosureReveal)
	mux.HandleFunc("/v1/ui/shortcuts", s.handleUIShortcuts)
	mux.HandleFunc("/v1/ui/navigation-map", s.handleUINavigationMap)
	mux.HandleFunc("/v1/ui/dashboard/widgets", s.handleDashboardWidgets)
	mux.HandleFunc("/v1/ui/dashboard/widgets/", s.handleDashboardWidgetAction)
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
	mux.HandleFunc("/v1/control/topology-advisor", s.handleTopologyAdvisor(baseDir))
	mux.HandleFunc("/v1/control/deployment-profiles", s.handleDeploymentProfiles)
	mux.HandleFunc("/v1/control/deployment-profiles/evaluate", s.handleDeploymentProfileEvaluate)
	mux.HandleFunc("/v1/control/checklists", s.handleChecklists)
	mux.HandleFunc("/v1/control/checklists/", s.handleChecklistAction)
	mux.HandleFunc("/v1/control/bootstrap/ha", s.handleHABootstrap)
	mux.HandleFunc("/v1/control/capacity", s.handleCapacity)
	mux.HandleFunc("/v1/control/canary-health", s.handleCanaryHealth)
	mux.HandleFunc("/v1/control/health-probes", s.handleHealthProbes)
	mux.HandleFunc("/v1/control/health-probes/checks", s.handleHealthProbeChecks)
	mux.HandleFunc("/v1/control/health-probes/evaluate", s.handleHealthProbeGateEvaluate)
	mux.HandleFunc("/v1/control/channels", s.handleChannels)
	mux.HandleFunc("/v1/control/canary-upgrades", s.handleCanaryUpgrades)
	mux.HandleFunc("/v1/control/canary-upgrades/", s.handleCanaryUpgradeAction)
	mux.HandleFunc("/v1/control/upgrade-orchestration/plans", s.handleUpgradeOrchestrationPlans)
	mux.HandleFunc("/v1/control/upgrade-orchestration/plans/", s.handleUpgradeOrchestrationPlanAction)
	mux.HandleFunc("/v1/control/failover-drills", s.handleRegionalFailoverDrills)
	mux.HandleFunc("/v1/control/failover-drills/scorecards", s.handleRegionalFailoverScorecards)
	mux.HandleFunc("/v1/control/performance/profiles", s.handlePerformanceProfiles)
	mux.HandleFunc("/v1/control/performance/diagnostics", s.handlePerformanceDiagnostics)
	mux.HandleFunc("/v1/control/topology-placement/policies", s.handleTopologyPlacementPolicies)
	mux.HandleFunc("/v1/control/topology-placement/decide", s.handleTopologyPlacementDecision)
	mux.HandleFunc("/v1/control/chaos/experiments", s.handleChaosExperiments)
	mux.HandleFunc("/v1/control/chaos/experiments/", s.handleChaosExperimentAction)
	mux.HandleFunc("/v1/control/leak-detection/policy", s.handleLeakDetectionPolicy)
	mux.HandleFunc("/v1/control/leak-detection/snapshots", s.handleLeakDetectionSnapshots)
	mux.HandleFunc("/v1/control/leak-detection/reports", s.handleLeakDetectionReports)
	mux.HandleFunc("/v1/control/federation/peers", s.handleFederationPeers)
	mux.HandleFunc("/v1/control/federation/peers/", s.handleFederationPeerAction)
	mux.HandleFunc("/v1/control/federation/health", s.handleFederationHealth)
	mux.HandleFunc("/v1/control/scheduler/partitions", s.handleSchedulerPartitions)
	mux.HandleFunc("/v1/control/scheduler/partitions/", s.handleSchedulerPartitionAction)
	mux.HandleFunc("/v1/control/scheduler/partition-decision", s.handleSchedulerPartitionDecision)
	mux.HandleFunc("/v1/control/autoscaling/policy", s.handleWorkerAutoscalingPolicy)
	mux.HandleFunc("/v1/control/autoscaling/recommend", s.handleWorkerAutoscalingRecommend)
	mux.HandleFunc("/v1/control/cost-scheduling/policies", s.handleCostSchedulingPolicies)
	mux.HandleFunc("/v1/control/cost-scheduling/admit", s.handleCostSchedulingAdmit)
	mux.HandleFunc("/v1/control/artifact-distribution/policies", s.handleArtifactDistributionPolicies)
	mux.HandleFunc("/v1/control/artifact-distribution/plan", s.handleArtifactDistributionPlan)
	mux.HandleFunc("/v1/control/workspaces/isolation-policies", s.handleWorkspaceIsolationPolicies)
	mux.HandleFunc("/v1/control/workspaces/isolation/evaluate", s.handleWorkspaceIsolationEvaluate)
	mux.HandleFunc("/v1/control/tenancy/policies", s.handleTenantPolicies)
	mux.HandleFunc("/v1/control/tenancy/admit-check", s.handleTenantAdmissionCheck)
	mux.HandleFunc("/v1/security/tenant-keys", s.handleTenantCryptoKeys)
	mux.HandleFunc("/v1/security/tenant-keys/rotate", s.handleTenantCryptoRotate)
	mux.HandleFunc("/v1/security/tenant-keys/boundary-check", s.handleTenantCryptoBoundaryCheck)
	mux.HandleFunc("/v1/control/delegated-admin/grants", s.handleDelegatedAdminGrants)
	mux.HandleFunc("/v1/control/delegated-admin/authorize", s.handleDelegatedAdminAuthorize)
	mux.HandleFunc("/v1/control/multi-master/nodes", s.handleMultiMasterNodes)
	mux.HandleFunc("/v1/control/multi-master/nodes/", s.handleMultiMasterNodeAction)
	mux.HandleFunc("/v1/control/multi-master/cache", s.handleMultiMasterCache)
	mux.HandleFunc("/v1/control/schema-migrations", s.handleSchemaMigrations)
	mux.HandleFunc("/v1/schema/models", s.handleOpenSchemas)
	mux.HandleFunc("/v1/schema/models/", s.handleOpenSchemaByID)
	mux.HandleFunc("/v1/schema/validate", s.handleOpenSchemaValidate)
	mux.HandleFunc("/v1/control/preflight", s.handlePreflight)
	mux.HandleFunc("/v1/control/invariants/check", s.handleInvariantChecks)
	mux.HandleFunc("/v1/control/blast-radius-map", s.handleBlastRadiusMap(baseDir))
	mux.HandleFunc("/v1/control/disruption-budgets", s.handleDisruptionBudgets)
	mux.HandleFunc("/v1/control/disruption-budgets/evaluate", s.handleDisruptionBudgetEvaluate)
	mux.HandleFunc("/v1/control/queue", s.handleQueueControl)
	mux.HandleFunc("/v1/control/workers/lifecycle", s.handleWorkerLifecycle)
	mux.HandleFunc("/v1/control/execution-locks", s.handleExecutionLocks)
	mux.HandleFunc("/v1/control/execution-locks/release", s.handleExecutionLockRelease)
	mux.HandleFunc("/v1/control/execution-locks/cleanup", s.handleExecutionLockCleanup)
	mux.HandleFunc("/v1/control/run-leases", s.handleRunLeases)
	mux.HandleFunc("/v1/control/run-leases/heartbeat", s.handleRunLeaseHeartbeat)
	mux.HandleFunc("/v1/control/run-leases/release", s.handleRunLeaseRelease)
	mux.HandleFunc("/v1/control/run-leases/recover", s.handleRunLeaseRecover)
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
			"GET /v1/activity/audit-timeline",
			"GET /v1/search",
			"GET /v1/tasks/definitions",
			"POST /v1/tasks/definitions",
			"GET /v1/tasks/definitions/{id}",
			"GET /v1/wizards",
			"GET /v1/wizards/{id}",
			"POST /v1/wizards/{id}/launch",
			"GET /v1/tasks/plans",
			"POST /v1/tasks/plans",
			"GET /v1/tasks/plans/{id}",
			"POST /v1/tasks/plans/{id}/preview",
			"GET /v1/tasks/executions",
			"POST /v1/tasks/executions",
			"GET /v1/tasks/executions/{id}",
			"POST /v1/tasks/executions/{id}/cancel",
			"GET /v1/edge-relay/sites",
			"POST /v1/edge-relay/sites",
			"GET /v1/edge-relay/sites/{id}",
			"POST /v1/edge-relay/sites/{id}/heartbeat",
			"POST /v1/edge-relay/sites/{id}/deliver",
			"GET /v1/edge-relay/messages",
			"POST /v1/edge-relay/messages",
			"GET /v1/offline/mode",
			"POST /v1/offline/mode",
			"GET /v1/offline/bundles",
			"POST /v1/offline/bundles",
			"POST /v1/offline/bundles/verify",
			"GET /v1/offline/mirrors",
			"POST /v1/offline/mirrors",
			"GET /v1/offline/mirrors/{id}",
			"POST /v1/offline/mirrors/sync",
			"GET /v1/docs/generate",
			"POST /v1/docs/generate",
			"POST /v1/docs/examples/verify",
			"GET /v1/docs/api/version-diff",
			"POST /v1/docs/api/version-diff",
			"GET /v1/lint/style/rules",
			"POST /v1/lint/style/analyze",
			"POST /v1/format/canonicalize",
			"GET /v1/policy/pull/sources",
			"POST /v1/policy/pull/sources",
			"GET /v1/policy/pull/sources/{id}",
			"POST /v1/policy/pull/execute",
			"GET /v1/policy/pull/results",
			"GET /v1/policy/bundles",
			"POST /v1/policy/bundles",
			"GET /v1/policy/bundles/{id}",
			"POST /v1/policy/bundles/{id}/promote",
			"GET /v1/policy/bundles/{id}/promotions",
			"GET /v1/inventory/groups",
			"POST /v1/inventory/import/cmdb",
			"POST /v1/inventory/import/assist",
			"POST /v1/inventory/import/brownfield-bootstrap",
			"POST /v1/inventory/drift/analyze",
			"POST /v1/inventory/drift/reconcile",
			"GET /v1/inventory/drift/reports",
			"GET /v1/inventory/classification-rules",
			"POST /v1/inventory/classification-rules",
			"GET /v1/inventory/classification-rules/{id}",
			"POST /v1/inventory/classify",
			"GET /v1/inventory/node-classifiers",
			"POST /v1/inventory/node-classifiers",
			"GET /v1/inventory/node-classifiers/{id}",
			"POST /v1/inventory/node-classifiers/{id}/enable",
			"POST /v1/inventory/node-classifiers/{id}/disable",
			"POST /v1/inventory/node-classifiers/classify",
			"GET /v1/compat/grains",
			"POST /v1/compat/grains/query",
			"GET /v1/inventory/discovery-sources",
			"POST /v1/inventory/discovery-sources",
			"GET /v1/inventory/discovery-sources/{id}",
			"POST /v1/inventory/discovery-sources/sync",
			"POST /v1/inventory/cloud-sync",
			"GET /v1/fleet/health",
			"GET /v1/inventory/runtime-hosts",
			"POST /v1/inventory/runtime-hosts",
			"POST /v1/inventory/enroll",
			"GET /v1/inventory/runtime-hosts/{name}",
			"POST /v1/inventory/runtime-hosts/{name}/heartbeat",
			"POST /v1/inventory/runtime-hosts/{name}/bootstrap",
			"POST /v1/inventory/runtime-hosts/{name}/activate",
			"POST /v1/inventory/runtime-hosts/{name}/quarantine",
			"POST /v1/inventory/runtime-hosts/{name}/decommission",
			"GET /v1/agents/checkins",
			"POST /v1/agents/checkins",
			"GET /v1/agents/dispatch-mode",
			"POST /v1/agents/dispatch-mode",
			"GET /v1/agents/dispatch-environments",
			"POST /v1/agents/dispatch-environments",
			"GET /v1/agents/dispatch-environments/{environment}",
			"GET /v1/agents/dispatch",
			"POST /v1/agents/dispatch",
			"GET /v1/agents/proxy-minions",
			"POST /v1/agents/proxy-minions",
			"GET /v1/agents/proxy-minions/{id}",
			"GET /v1/agents/proxy-minions/dispatch",
			"POST /v1/agents/proxy-minions/dispatch",
			"GET /v1/execution/network-transports",
			"POST /v1/execution/network-transports",
			"POST /v1/execution/network-transports/validate",
			"GET /v1/execution/portable-runners",
			"POST /v1/execution/portable-runners",
			"POST /v1/execution/portable-runners/select",
			"GET /v1/execution/native-schedulers",
			"POST /v1/execution/native-schedulers/select",
			"GET /v1/execution/package-managers",
			"POST /v1/execution/package-managers/resolve",
			"POST /v1/execution/package-managers/render-action",
			"GET /v1/execution/systemd/units",
			"POST /v1/execution/systemd/units",
			"GET /v1/execution/systemd/units/{name}",
			"POST /v1/execution/systemd/units/render",
			"GET /v1/execution/reboot/policies",
			"POST /v1/execution/reboot/policies",
			"POST /v1/execution/reboot/plan",
			"GET /v1/execution/patch/policies",
			"POST /v1/execution/patch/policies",
			"POST /v1/execution/patch/plan",
			"GET /v1/execution/image-baking/pipelines",
			"POST /v1/execution/image-baking/pipelines",
			"GET /v1/execution/image-baking/pipelines/{id}",
			"POST /v1/execution/image-baking/pipelines/{id}/plan",
			"GET /v1/execution/artifacts/deployments",
			"POST /v1/execution/artifacts/deployments",
			"GET /v1/execution/artifacts/deployments/{id}",
			"GET /v1/execution/artifacts/deployments/{id}/plan",
			"GET /v1/execution/session-recordings",
			"GET /v1/execution/session-recordings/{id}",
			"GET /v1/execution/adaptive-concurrency/policy",
			"POST /v1/execution/adaptive-concurrency/policy",
			"POST /v1/execution/adaptive-concurrency/recommend",
			"GET /v1/execution/checkpoints",
			"POST /v1/execution/checkpoints",
			"GET /v1/execution/checkpoints/{id}",
			"POST /v1/execution/checkpoints/resume",
			"GET /v1/execution/snapshots",
			"POST /v1/execution/snapshots",
			"GET /v1/execution/snapshots/{id}",
			"GET /v1/execution/environments",
			"POST /v1/execution/environments",
			"GET /v1/execution/environments/{id}",
			"GET /v1/execution/admission-policy",
			"POST /v1/execution/admission-policy",
			"POST /v1/execution/admit-check",
			"GET /v1/execution/credentials",
			"POST /v1/execution/credentials",
			"POST /v1/execution/credentials/validate",
			"GET /v1/execution/credentials/{id}",
			"POST /v1/execution/credentials/{id}/revoke",
			"GET /v1/execution/masterless/mode",
			"POST /v1/execution/masterless/mode",
			"POST /v1/execution/masterless/render",
			"GET /v1/execution/relays/endpoints",
			"POST /v1/execution/relays/endpoints",
			"GET /v1/execution/relays/endpoints/{id}",
			"GET /v1/execution/relays/sessions",
			"POST /v1/execution/relays/sessions",
			"GET /v1/control/syndic/nodes",
			"POST /v1/control/syndic/nodes",
			"GET /v1/control/syndic/route",
			"POST /v1/control/syndic/route",
			"GET /v1/security/crypto/fips-mode",
			"POST /v1/security/crypto/fips-mode",
			"POST /v1/security/crypto/fips/validate",
			"GET /v1/security/host-profiles",
			"POST /v1/security/host-profiles",
			"POST /v1/security/host-profiles/evaluate",
			"GET /v1/security/signatures/keyrings",
			"POST /v1/security/signatures/keyrings",
			"GET /v1/security/signatures/keyrings/{id}",
			"GET /v1/security/signatures/admission-policy",
			"POST /v1/security/signatures/admission-policy",
			"POST /v1/security/signatures/admit-check",
			"GET /v1/secrets/runtime/sessions",
			"POST /v1/secrets/runtime/sessions",
			"GET /v1/secrets/runtime/sessions/{id}",
			"POST /v1/secrets/runtime/sessions/{id}/destroy",
			"POST /v1/secrets/runtime/consume",
			"GET /v1/secrets/encrypted-store/items",
			"POST /v1/secrets/encrypted-store/items",
			"GET /v1/secrets/encrypted-store/items/{name}",
			"POST /v1/secrets/encrypted-store/items/{name}/resolve",
			"POST /v1/secrets/encrypted-store/items/{name}/rotate",
			"GET /v1/secrets/encrypted-store/expired",
			"GET /v1/access/delegation-tokens",
			"POST /v1/access/delegation-tokens",
			"POST /v1/access/delegation-tokens/validate",
			"GET /v1/access/delegation-tokens/{id}",
			"POST /v1/access/delegation-tokens/{id}/revoke",
			"GET /v1/access/approval-policies",
			"POST /v1/access/approval-policies",
			"GET /v1/access/approval-policies/{id}",
			"GET /v1/access/break-glass/requests",
			"POST /v1/access/break-glass/requests",
			"GET /v1/access/break-glass/requests/{id}",
			"POST /v1/access/break-glass/requests/{id}/approve",
			"POST /v1/access/break-glass/requests/{id}/reject",
			"POST /v1/access/break-glass/requests/{id}/revoke",
			"GET /v1/access/jit-grants",
			"POST /v1/access/jit-grants",
			"POST /v1/access/jit-grants/validate",
			"GET /v1/access/jit-grants/{id}",
			"POST /v1/access/jit-grants/{id}/revoke",
			"GET /v1/access/rbac/roles",
			"POST /v1/access/rbac/roles",
			"GET /v1/access/rbac/roles/{id}",
			"GET /v1/access/rbac/bindings",
			"POST /v1/access/rbac/bindings",
			"POST /v1/access/rbac/check",
			"GET /v1/access/abac/policies",
			"POST /v1/access/abac/policies",
			"POST /v1/access/abac/check",
			"GET /v1/identity/sso/providers",
			"POST /v1/identity/sso/providers",
			"GET /v1/identity/sso/providers/{id}",
			"POST /v1/identity/sso/providers/{id}/enable",
			"POST /v1/identity/sso/providers/{id}/disable",
			"POST /v1/identity/sso/login/start",
			"POST /v1/identity/sso/login/callback",
			"GET /v1/identity/sso/sessions",
			"GET /v1/identity/sso/sessions/{id}",
			"GET /v1/identity/scim/roles",
			"POST /v1/identity/scim/roles",
			"GET /v1/identity/scim/roles/{id}",
			"GET /v1/identity/scim/teams",
			"POST /v1/identity/scim/teams",
			"GET /v1/identity/scim/teams/{id}",
			"GET /v1/identity/oidc/workload/providers",
			"POST /v1/identity/oidc/workload/providers",
			"GET /v1/identity/oidc/workload/providers/{id}",
			"POST /v1/identity/oidc/workload/exchange",
			"GET /v1/identity/oidc/workload/credentials",
			"GET /v1/identity/oidc/workload/credentials/{id}",
			"GET /v1/security/mtls/authorities",
			"POST /v1/security/mtls/authorities",
			"GET /v1/security/mtls/policies",
			"POST /v1/security/mtls/policies",
			"POST /v1/security/mtls/handshake-check",
			"GET /v1/secrets/integrations",
			"POST /v1/secrets/integrations",
			"POST /v1/secrets/resolve",
			"GET /v1/secrets/traces",
			"GET /v1/packages/artifacts",
			"POST /v1/packages/artifacts",
			"GET /v1/packages/artifacts/{id}",
			"GET /v1/packages/signing-policy",
			"POST /v1/packages/signing-policy",
			"POST /v1/packages/verify",
			"GET /v1/packages/certification-policy",
			"POST /v1/packages/certification-policy",
			"POST /v1/packages/certify",
			"GET /v1/packages/certifications",
			"POST /v1/packages/publication/check",
			"GET /v1/packages/maintainers/health",
			"POST /v1/packages/maintainers/health",
			"GET /v1/packages/maintainers/health/{maintainer}",
			"GET /v1/packages/provenance/report",
			"GET /v1/packages/quality",
			"POST /v1/packages/quality/evaluate",
			"GET /v1/packages/content-channels",
			"GET /v1/packages/content-channels/sync-policy",
			"POST /v1/packages/content-channels/sync-policy",
			"GET /v1/packages/content-channels/remotes",
			"POST /v1/packages/content-channels/remotes",
			"GET /v1/packages/content-channels/remotes/{id}",
			"POST /v1/packages/content-channels/remotes/{id}/rotate-token",
			"GET /v1/packages/scaffold/templates",
			"POST /v1/packages/scaffold/generate",
			"POST /v1/packages/interface-compat/analyze",
			"GET /v1/packages/pinning/policies",
			"POST /v1/packages/pinning/policies",
			"POST /v1/packages/pinning/evaluate",
			"GET /v1/agents/cert-policy",
			"POST /v1/agents/cert-policy",
			"GET /v1/agents/catalogs",
			"POST /v1/agents/catalogs",
			"GET /v1/agents/catalogs/{id}",
			"POST /v1/agents/catalogs/replay",
			"GET /v1/agents/catalogs/replays",
			"GET /v1/agents/attestation/policy",
			"POST /v1/agents/attestation/policy",
			"GET /v1/agents/attestations",
			"POST /v1/agents/attestations",
			"GET /v1/agents/attestations/{id}",
			"POST /v1/agents/attestations/check",
			"GET /v1/agents/csrs",
			"POST /v1/agents/csrs",
			"POST /v1/agents/csrs/{id}/approve",
			"POST /v1/agents/csrs/{id}/reject",
			"GET /v1/agents/certificates",
			"POST /v1/agents/certificates/{id}/revoke",
			"POST /v1/agents/certificates/rotate",
			"GET /v1/agents/certificates/expiry-report",
			"POST /v1/agents/certificates/renew-expiring",
			"GET /v1/compliance/profiles",
			"POST /v1/compliance/profiles",
			"GET /v1/compliance/profiles/{id}",
			"GET /v1/compliance/scans",
			"POST /v1/compliance/scans",
			"GET /v1/compliance/scans/{id}",
			"GET /v1/compliance/scans/{id}/evidence",
			"GET /v1/compliance/continuous",
			"POST /v1/compliance/continuous",
			"POST /v1/compliance/continuous/{id}/run",
			"GET /v1/compliance/exceptions",
			"POST /v1/compliance/exceptions",
			"POST /v1/compliance/exceptions/{id}/approve",
			"POST /v1/compliance/exceptions/{id}/reject",
			"GET /v1/compliance/scorecards",
			"GET /v1/gitops/previews",
			"POST /v1/gitops/previews",
			"GET /v1/gitops/previews/{id}",
			"POST /v1/gitops/previews/{id}/promote",
			"POST /v1/gitops/previews/{id}/close",
			"GET /v1/gitops/environments",
			"POST /v1/gitops/environments",
			"POST /v1/gitops/environments/materialize",
			"GET /v1/gitops/environments/{name}",
			"GET /v1/gitops/deployments",
			"POST /v1/gitops/deployments",
			"POST /v1/gitops/deployments/trigger",
			"POST /v1/gitops/deployments/webhook",
			"GET /v1/gitops/deployments/{id}",
			"GET /v1/deployments/rollout/policies",
			"POST /v1/deployments/rollout/policies",
			"POST /v1/deployments/rollout/plan",
			"GET /v1/gitops/filesync/pipelines",
			"POST /v1/gitops/filesync/pipelines",
			"GET /v1/gitops/filesync/pipelines/{id}",
			"POST /v1/gitops/filesync/pipelines/{id}/run",
			"GET /v1/gitops/promotions",
			"POST /v1/gitops/promotions",
			"GET /v1/gitops/promotions/{id}",
			"POST /v1/gitops/promotions/{id}/advance",
			"POST /v1/gitops/reconcile",
			"POST /v1/gitops/plan-artifacts/sign",
			"POST /v1/gitops/plan-artifacts/verify",
			"GET /v1/incidents/view",
			"GET /v1/fleet/nodes",
			"GET /v1/drift/insights",
			"GET /v1/drift/history",
			"GET /v1/drift/suppressions",
			"POST /v1/drift/suppressions",
			"DELETE /v1/drift/suppressions/{id}",
			"GET /v1/drift/allowlists",
			"POST /v1/drift/allowlists",
			"DELETE /v1/drift/allowlists/{id}",
			"POST /v1/drift/remediate",
			"GET /v1/metrics",
			"GET /v1/features/summary",
			"GET /v1/docs/actions",
			"GET /v1/docs/actions/{id}",
			"GET /v1/model/objects",
			"GET /v1/model/objects/resolve",
			"GET /v1/docs/inline",
			"POST /v1/plans/explain",
			"POST /v1/plans/graph",
			"POST /v1/plans/graph/query",
			"POST /v1/plans/diff-preview",
			"POST /v1/plans/reproducibility-check",
			"POST /v1/plans/risk-summary",
			"POST /v1/policy/simulate",
			"GET /v1/policy/enforcement-modes",
			"POST /v1/policy/enforcement-modes",
			"GET /v1/policy/enforcement-modes/{policy_ref}",
			"POST /v1/policy/enforcement-modes/evaluate",
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
			"GET /v1/ui/accessibility/profiles",
			"POST /v1/ui/accessibility/profiles",
			"GET /v1/ui/accessibility/active",
			"POST /v1/ui/accessibility/active",
			"GET /v1/ui/progressive-disclosure",
			"POST /v1/ui/progressive-disclosure",
			"GET /v1/ui/progressive-disclosure/reveal",
			"POST /v1/ui/progressive-disclosure/reveal",
			"GET /v1/ui/shortcuts",
			"GET /v1/ui/navigation-map",
			"GET /v1/ui/dashboard/widgets",
			"POST /v1/ui/dashboard/widgets",
			"GET /v1/ui/dashboard/widgets/{id}",
			"DELETE /v1/ui/dashboard/widgets/{id}",
			"POST /v1/ui/dashboard/widgets/{id}/pin",
			"POST /v1/ui/dashboard/widgets/{id}/refresh",
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
			"GET /v1/release/readiness/scorecards",
			"POST /v1/release/readiness/scorecards",
			"GET /v1/release/readiness/scorecards/{id}",
			"POST /v1/release/blocker-policy",
			"GET /v1/release/blocker-policy",
			"GET /v1/release/api-contract",
			"POST /v1/release/api-contract",
			"GET /v1/release/upgrade-assistant",
			"POST /v1/release/upgrade-assistant",
			"GET /v1/release/dependency-bot/policy",
			"POST /v1/release/dependency-bot/policy",
			"GET /v1/release/dependency-bot/updates",
			"POST /v1/release/dependency-bot/updates",
			"GET /v1/release/dependency-bot/updates/{id}",
			"POST /v1/release/dependency-bot/updates/{id}/evaluate",
			"GET /v1/release/performance-gates/policy",
			"POST /v1/release/performance-gates/policy",
			"POST /v1/release/performance-gates/evaluate",
			"GET /v1/release/performance-gates/evaluations",
			"GET /v1/release/tests/flake-policy",
			"POST /v1/release/tests/flake-policy",
			"POST /v1/release/tests/flake-observations",
			"GET /v1/release/tests/flake-cases",
			"GET /v1/release/tests/flake-cases/{id}",
			"POST /v1/release/tests/flake-cases/{id}/quarantine",
			"POST /v1/release/tests/flake-cases/{id}/unquarantine",
			"POST /v1/release/tests/impact-analysis",
			"GET /v1/release/tests/scenarios",
			"POST /v1/release/tests/scenarios",
			"GET /v1/release/tests/scenario-runs",
			"POST /v1/release/tests/scenario-runs",
			"GET /v1/release/tests/scenario-runs/{id}",
			"POST /v1/release/tests/scenario-runs/{id}/compare-baseline",
			"GET /v1/release/tests/scenario-baselines",
			"POST /v1/release/tests/scenario-baselines",
			"GET /v1/release/tests/scenario-baselines/{id}",
			"GET /v1/release/tests/environments",
			"POST /v1/release/tests/environments",
			"GET /v1/release/tests/environments/{id}",
			"POST /v1/release/tests/environments/{id}/run-check",
			"GET /v1/release/tests/environments/{id}/checks",
			"POST /v1/release/tests/environments/{id}/destroy",
			"GET /v1/release/tests/load-soak/suites",
			"POST /v1/release/tests/load-soak/suites",
			"GET /v1/release/tests/load-soak/runs",
			"POST /v1/release/tests/load-soak/runs",
			"GET /v1/release/tests/load-soak/runs/{id}",
			"GET /v1/release/tests/mutation/policy",
			"POST /v1/release/tests/mutation/policy",
			"GET /v1/release/tests/mutation/suites",
			"POST /v1/release/tests/mutation/suites",
			"GET /v1/release/tests/mutation/runs",
			"POST /v1/release/tests/mutation/runs",
			"GET /v1/release/tests/mutation/runs/{id}",
			"GET /v1/release/tests/property-harness/cases",
			"POST /v1/release/tests/property-harness/cases",
			"GET /v1/release/tests/property-harness/runs",
			"POST /v1/release/tests/property-harness/runs",
			"GET /v1/release/tests/property-harness/runs/{id}",
			"GET /v1/release/tests/harness/cases",
			"POST /v1/release/tests/harness/cases",
			"GET /v1/release/tests/harness/runs",
			"POST /v1/release/tests/harness/runs",
			"GET /v1/release/tests/harness/runs/{id}",
			"GET /v1/providers/conformance/suites",
			"POST /v1/providers/conformance/suites",
			"GET /v1/providers/conformance/runs",
			"POST /v1/providers/conformance/runs",
			"GET /v1/providers/conformance/runs/{id}",
			"GET /v1/providers/catalog",
			"POST /v1/providers/catalog/validate",
			"GET /v1/providers/protocol/descriptors",
			"POST /v1/providers/protocol/descriptors",
			"POST /v1/providers/protocol/negotiate",
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
			"POST /v1/event-stream/ingest",
			"POST /v1/event-stream/webhooks/ingest",
			"GET /v1/converge/triggers",
			"POST /v1/converge/triggers",
			"GET /v1/converge/triggers/{id}",
			"GET /v1/resources/exported",
			"POST /v1/resources/exported",
			"POST /v1/resources/collect",
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
			"GET /v1/control/topology-advisor",
			"GET /v1/control/deployment-profiles",
			"POST /v1/control/deployment-profiles/evaluate",
			"GET /v1/control/checklists",
			"POST /v1/control/checklists",
			"GET /v1/control/checklists/{id}",
			"POST /v1/control/checklists/{id}/complete",
			"POST /v1/control/bootstrap/ha",
			"POST /v1/control/capacity",
			"GET /v1/control/capacity",
			"GET /v1/control/canary-health",
			"GET /v1/control/health-probes",
			"POST /v1/control/health-probes",
			"POST /v1/control/health-probes/checks",
			"POST /v1/control/health-probes/evaluate",
			"POST /v1/control/channels",
			"GET /v1/control/channels",
			"GET /v1/control/canary-upgrades",
			"POST /v1/control/canary-upgrades",
			"GET /v1/control/canary-upgrades/{id}",
			"GET /v1/control/upgrade-orchestration/plans",
			"POST /v1/control/upgrade-orchestration/plans",
			"GET /v1/control/upgrade-orchestration/plans/{id}",
			"POST /v1/control/upgrade-orchestration/plans/{id}/advance",
			"POST /v1/control/upgrade-orchestration/plans/{id}/abort",
			"GET /v1/control/failover-drills",
			"POST /v1/control/failover-drills",
			"GET /v1/control/failover-drills/scorecards",
			"GET /v1/control/performance/profiles",
			"POST /v1/control/performance/profiles",
			"POST /v1/control/performance/diagnostics",
			"GET /v1/control/topology-placement/policies",
			"POST /v1/control/topology-placement/policies",
			"POST /v1/control/topology-placement/decide",
			"GET /v1/control/chaos/experiments",
			"POST /v1/control/chaos/experiments",
			"GET /v1/control/chaos/experiments/{id}",
			"POST /v1/control/chaos/experiments/{id}/complete",
			"POST /v1/control/chaos/experiments/{id}/abort",
			"GET /v1/control/leak-detection/policy",
			"POST /v1/control/leak-detection/policy",
			"POST /v1/control/leak-detection/snapshots",
			"GET /v1/control/leak-detection/reports",
			"GET /v1/control/federation/peers",
			"POST /v1/control/federation/peers",
			"GET /v1/control/federation/peers/{id}",
			"POST /v1/control/federation/peers/{id}/health",
			"GET /v1/control/federation/health",
			"GET /v1/control/scheduler/partitions",
			"POST /v1/control/scheduler/partitions",
			"GET /v1/control/scheduler/partitions/{id}",
			"POST /v1/control/scheduler/partition-decision",
			"GET /v1/control/autoscaling/policy",
			"POST /v1/control/autoscaling/policy",
			"POST /v1/control/autoscaling/recommend",
			"GET /v1/control/cost-scheduling/policies",
			"POST /v1/control/cost-scheduling/policies",
			"POST /v1/control/cost-scheduling/admit",
			"GET /v1/control/artifact-distribution/policies",
			"POST /v1/control/artifact-distribution/policies",
			"POST /v1/control/artifact-distribution/plan",
			"GET /v1/control/workspaces/isolation-policies",
			"POST /v1/control/workspaces/isolation-policies",
			"POST /v1/control/workspaces/isolation/evaluate",
			"GET /v1/control/tenancy/policies",
			"POST /v1/control/tenancy/policies",
			"POST /v1/control/tenancy/admit-check",
			"GET /v1/security/tenant-keys",
			"POST /v1/security/tenant-keys",
			"POST /v1/security/tenant-keys/rotate",
			"POST /v1/security/tenant-keys/boundary-check",
			"GET /v1/control/delegated-admin/grants",
			"POST /v1/control/delegated-admin/grants",
			"POST /v1/control/delegated-admin/authorize",
			"GET /v1/control/multi-master/nodes",
			"POST /v1/control/multi-master/nodes",
			"GET /v1/control/multi-master/nodes/{id}",
			"POST /v1/control/multi-master/nodes/{id}/heartbeat",
			"GET /v1/control/multi-master/cache",
			"POST /v1/control/multi-master/cache",
			"POST /v1/control/schema-migrations",
			"GET /v1/control/schema-migrations",
			"GET /v1/schema/models",
			"POST /v1/schema/models",
			"GET /v1/schema/models/{id}",
			"POST /v1/schema/validate",
			"POST /v1/control/preflight",
			"POST /v1/control/invariants/check",
			"POST /v1/control/blast-radius-map",
			"GET /v1/control/disruption-budgets",
			"POST /v1/control/disruption-budgets",
			"POST /v1/control/disruption-budgets/evaluate",
			"POST /v1/control/queue",
			"GET /v1/control/queue",
			"POST /v1/control/workers/lifecycle",
			"GET /v1/control/workers/lifecycle",
			"GET /v1/control/execution-locks",
			"POST /v1/control/execution-locks",
			"POST /v1/control/execution-locks/release",
			"POST /v1/control/execution-locks/cleanup",
			"GET /v1/control/run-leases",
			"POST /v1/control/run-leases",
			"POST /v1/control/run-leases/heartbeat",
			"POST /v1/control/run-leases/release",
			"POST /v1/control/run-leases/recover",
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
			"POST /v1/templates/{id}/render",
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
		ConfigPath     string `json:"config_path"`
		Priority       string `json:"priority"`
		LockKey        string `json:"lock_key,omitempty"`
		LockTTLSeconds int    `json:"lock_ttl_seconds,omitempty"`
		LockOwner      string `json:"lock_owner,omitempty"`
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
			lockKey := req.LockKey
			if strings.TrimSpace(lockKey) == "" {
				lockKey = r.Header.Get("X-Execution-Lock-Key")
			}
			lockOwner := req.LockOwner
			if strings.TrimSpace(lockOwner) == "" {
				lockOwner = r.Header.Get("X-Execution-Lock-Owner")
			}
			job, err := s.enqueueJobWithOptionalLock(req.ConfigPath, key, force, priority, lockKey, req.LockTTLSeconds, lockOwner)
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
		StrictMode  bool                           `json:"strict_mode,omitempty"`
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
				StrictMode:  req.StrictMode,
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
		mergedVars := control.MergeTemplateVariables(t.Defaults, launch.Answers)
		rendered, missing, renderErr := control.RenderTemplateFile(t.ConfigPath, mergedVars, t.StrictMode)
		if renderErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": renderErr.Error()})
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
			"template":           t,
			"job":                job,
			"answers":            launch.Answers,
			"resolved_variables": mergedVars,
			"missing_variables":  missing,
			"rendered_preview":   rendered,
		})
	case "render":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type renderReq struct {
			Answers map[string]string `json:"answers"`
		}
		var req renderReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}
		t, ok := s.templates.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
			return
		}
		if err := control.ValidateSurveyAnswers(t.Survey, req.Answers); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		mergedVars := control.MergeTemplateVariables(t.Defaults, req.Answers)
		rendered, missing, err := control.RenderTemplateFile(t.ConfigPath, mergedVars, t.StrictMode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"template_id":        t.ID,
			"strict_mode":        t.StrictMode,
			"resolved_variables": mergedVars,
			"missing_variables":  missing,
			"rendered":           rendered,
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
		OSFamily        string `json:"os_family,omitempty"`
		Backend         string `json:"scheduler_backend,omitempty"`
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
			osFamily := strings.TrimSpace(req.OSFamily)
			if osFamily == "" {
				osFamily = "linux"
			}
			selection, err := s.nativeSchedulers.Select(control.NativeSchedulerSelectionRequest{
				OSFamily:         osFamily,
				IntervalSeconds:  req.IntervalSeconds,
				JitterSeconds:    req.JitterSeconds,
				PreferredBackend: req.Backend,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if !selection.Supported {
				writeJSON(w, http.StatusConflict, map[string]string{"error": selection.Reason})
				return
			}

			assoc, err := s.assocs.Create(control.AssociationCreate{
				ConfigPath: req.ConfigPath,
				TargetKind: req.TargetKind,
				TargetName: req.TargetName,
				Backend:    selection.Backend.Name,
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
					"backend":        assoc.Backend,
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

func (s *Server) handleWorkerLifecycle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.WorkerLifecycleStatus())
	case http.MethodPost:
		var req control.WorkerLifecycleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		policy := s.queue.SetWorkerLifecyclePolicy(req)
		s.events.Append(control.Event{
			Type:    "control.worker_lifecycle.policy",
			Message: "worker lifecycle policy updated",
			Fields: map[string]any{
				"mode":                policy.Mode,
				"max_jobs_per_worker": policy.MaxJobsPerWorker,
				"restart_delay_ms":    policy.RestartDelayMS,
			},
		})
		writeJSON(w, http.StatusOK, s.queue.WorkerLifecycleStatus())
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
