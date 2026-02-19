# Masterchef

Masterchef is an open source infrastructure automation platform designed in 2026 to combine the strengths of Chef, Ansible, and Puppet while removing their core weaknesses.

The goal is not to clone legacy behavior. The goal is a modern, verifiable, and extensible system for desired-state infrastructure management at any scale.

## Vision

Build a single platform that supports:

- Agentless execution for fast bootstrap and simple operations
- Agent-based convergence for continuous drift correction
- Deterministic planning before execution
- Policy and compliance as first-class capabilities
- Strong plugin boundaries so the community can extend safely
- Upgrade channels with explicit protocol support matrix (`stable`, `candidate`, `edge`, `lts`)
- API versioning with deprecation lifecycle guarantees for safe contract evolution

## First-Principles Design

Masterchef is based on these principles:

1. Desired state is the source of truth, never imperative scripts.
2. Every apply operation must be preceded by a plan.
3. Idempotency is mandatory at the resource-provider level.
4. Auditability is built in: every run is traceable and replayable.
5. Execution is data-driven by a typed intermediate graph, not ad hoc task order.
6. Security defaults to least privilege and explicit approvals for dangerous operations.

## Why Existing Tools Fall Short

- Chef: powerful model, but higher operational complexity and Ruby DSL coupling.
- Ansible: fast to start, but predictability and scale controls are weaker without stronger typing/planning.
- Puppet: mature convergence and policy model, but steeper DSL/operator learning and less flexible modern workflows.

Masterchef combines declarative modeling, strict plan/apply semantics, and dual-mode execution (agentless + agent) behind one engine.

## Language and Runtime Choices

- Core and agent implementation: Go
- Policy/schema layer: CUE + JSON Schema
- Plugin runtime boundary: gRPC + protobuf
- Optional sandbox for third-party providers: WASI/WASM

Why Go:

- Excellent cross-platform static binaries
- Strong concurrency and networking for orchestrators/agents
- Fast startup, low operational overhead, and strong ecosystem for CLIs and control planes
- Easier contributor onboarding than niche DSL ecosystems

## Architecture (High Level)

1. Config compiler
- Parses user config into a typed resource graph (IR).
- Validates schema, references, and policy gates.

2. Planner
- Computes changeset, dependency DAG, and execution plan.
- Emits a stable plan artifact for review and approvals.

3. Executor
- Applies resources with transactional checkpoints and retries.
- Supports SSH/WinRM agentless mode and gRPC agent mode.

4. State and events
- Stores desired state snapshots, observed state, run metadata, and audit logs.

5. Policy and compliance
- Enforces policy before apply and continuously checks fleet compliance.

See `FEATURES.md`, `agents.md`, and `ROADMAP.md` for detailed scope.

## Project Status

Active implementation phase.

Current control-plane DR surface includes backup, point-in-time restore, and automated restore-verification drills.
Managed file resources now emit filebucket-style backups under `.masterchef/filebucket` with checksum-addressed objects and append-only history records.
File integrity enforcement is available on file resources via `content_checksum` and optional ed25519 signed metadata (`content_signature` + `content_signing_pubkey`) with apply-time verification.
Regional failover drills with recovery-time scorecards are available via `/v1/control/failover-drills` and `/v1/control/failover-drills/scorecards`.
Fault-injection and chaos testing workflows for orchestrator resilience are available via `/v1/control/chaos/experiments`.
Memory and resource leak detection for long-running control-plane components is available via `/v1/control/leak-detection/policy`, `/v1/control/leak-detection/snapshots`, and `/v1/control/leak-detection/reports`.
API contract governance includes deprecation lifecycle checks plus an upgrade-assistant endpoint for migration guidance.
Schema evolution controls enforce migration plans and stepwise compatibility for control-plane state model upgrades.
Plan snapshot baselines are available via `masterchef plan -snapshot <file>` to detect deterministic plan regressions.
On-call handoff packages are available via `GET /v1/control/handoff` to summarize risks, active rollouts, and blocked actions.
Deployment-window change digests are available via `GET /v1/runs/digest` with latent-risk scoring.
Time-travel run timelines (before/during/after change windows) are available via `GET /v1/runs/{id}/timeline`.
One-click retry and safe rollback actions from run failure context are available via `POST /v1/runs/{id}/retry` and `POST /v1/runs/{id}/rollback`.
Noise-reduction alert inbox is available via `GET/POST /v1/alerts/inbox` with dedup, suppression windows, and routing.
Run failure triage bundles are exportable via `POST /v1/runs/{id}/triage-bundle` for incident debugging context.
Cross-run diff analysis (failed vs successful execution comparison) is available via `GET /v1/runs/compare`.
Drift trend analytics with suppression/allowlist filtering, root-cause hints/remediations, policy management, safe-mode auto-remediation, and desired-vs-observed diff history are available via `GET /v1/drift/insights`, `GET /v1/drift/history`, `/v1/drift/suppressions`, `/v1/drift/allowlists`, and `POST /v1/drift/remediate`.
Run-step observability correlation IDs are available via `GET /v1/runs/{id}/correlations`.
Notification integrations are managed via `/v1/notifications/targets` and `/v1/notifications/deliveries` for ChatOps/incident/ticket routing.
Change records and approval workflows are exposed via `/v1/change-records` to tie execution to ticketed change control.
Self-service runbook catalog with approval-gated launches is available via `/v1/runbooks`.
Operator checklist mode for high-risk changes is available via `/v1/control/checklists`.
Guided topology advisor for scaling from small teams to large fleets is available via `GET /v1/control/topology-advisor`.
One-command bootstrap planning for single-region HA control planes is available via `POST /v1/control/bootstrap/ha`.
Saved views with share tokens plus pin-to-dashboard widget workflows are available via `/v1/views` and `/v1/ui/dashboard/widgets`.
Bulk operation staging with preview/conflict detection/confirmed execution is available via `/v1/bulk/preview` and `/v1/bulk/execute`.
Persona-based home views for SRE/platform/release/service-owner workflows are available via `GET /v1/views/home`.
Workload-centric operational views grouped by service/application are available via `GET /v1/views/workloads`.
Guided workflow wizards for bootstrap, rollout, rollback, and incident remediation are available via `/v1/wizards` and `/v1/wizards/{id}/launch`.
Accessibility-first UX profiles (keyboard-first, screen-reader optimized, high-contrast, reduced-motion) are available via `/v1/ui/accessibility/profiles` and `/v1/ui/accessibility/active`.
Progressive disclosure UI controls (simple, balanced, advanced, plus workflow-based advanced reveal) are available via `/v1/ui/progressive-disclosure` and `/v1/ui/progressive-disclosure/reveal`.
Keyboard-first workflow shortcut catalog is available via `GET /v1/ui/shortcuts`.
Keyboard-first no-mouse workflow coverage maps are available via `GET /v1/ui/navigation-map`.
Consistent object-model naming across CLI/UI/API is available via `GET /v1/model/objects` and `GET /v1/model/objects/resolve`.
Fleet node views with cursor-based incremental loading plus `compact`, `virtualized`, and `low-bandwidth` render modes are available via `GET /v1/fleet/nodes`.
Fleet health SLO/error-budget views are available via `GET /v1/fleet/health`.
Universal command-palette search across hosts, services, runs, policies, and modules is available via `GET /v1/search`.
Inline action guidance with endpoint-aware examples is available via `GET /v1/docs/inline` to surface docs at point of action.
Data bag/global object store with encrypted item support and structured search is available via `/v1/data-bags` and `/v1/data-bags/search`.
Chef-style role and environment objects with deterministic per-environment resolution are available via `/v1/roles`, `/v1/environments`, and `GET /v1/roles/{name}/resolve`.
Role/profile/environment inheritance is supported via role `profiles`, with parent-role run-list and attribute resolution plus cycle detection in `GET /v1/roles/{name}/resolve`.
Open schema model registry and validation (YAML/CUE/JSON Schema) are available via `/v1/schema/models` and `POST /v1/schema/validate`.
Configuration composition with recursive `includes`, `imports`, and `overlays` is supported by the config loader with deterministic precedence and cycle detection.
Configuration conditionals, loops, and matrix expansion are supported on resources via `when`, `loop`/`loop_var`, and `matrix`, with deterministic cartesian expansion during config load.
Encrypted variable files with key rotation (Vault-style) are available via `/v1/vars/encrypted/files` and `/v1/vars/encrypted/keys`.
Pillar/Hiera-style hierarchical data resolution with explicit merge strategies is available via `POST /v1/pillar/resolve`.
Fact caching with TTL/invalidation and Salt Mine-style cross-node fact queries are available via `/v1/facts/cache` and `POST /v1/facts/mine/query`.
Variable precedence resolution with source graph, conflict detection, hard-fail policy, and explain output is available via `POST /v1/vars/resolve` and `POST /v1/vars/explain`.
External variable source plugins (`inline`, `env`, `file`, `http`) are available via `POST /v1/vars/sources/resolve`.
Unified CLI now includes `observe` and `drift` commands for local run telemetry and drift trend inspection.
Contributor-friendly single-binary local dev runtime (control plane + worker + local registry/object store) is available via `masterchef dev -state-dir .masterchef/dev -grpc-addr :9090`.
Interactive CLI TUI inspection is available via `masterchef tui` with run browsing and per-step detail views.
Ansible-compatible plugin extension points (`callback`, `lookup`, `filter`, `vars`, `strategy`) are available via `/v1/plugins/extensions`.
Execution strategy controls (`linear`, `free`, `serial`) with failure thresholds (`max_fail_percentage`, `any_errors_fatal`) are supported in config and executor runtime.
Failure-domain-aware serial orchestration is supported with `execution.failure_domain` (`rack|zone|region`) to interleave hosts across domains.
Disruption budget definitions and rollout-gating evaluation are available via `/v1/control/disruption-budgets` and `/v1/control/disruption-budgets/evaluate`.
Privilege escalation controls for command resources are supported via `become` and `become_user`, with explicit run-result audit markers.
Session recording artifacts for privileged remote command executions are emitted under `.masterchef/sessions` and linked from run output.
Selective and targeted execution filters are supported in `check`/`apply` via `-hosts`, `-groups`, `-resources`, `-tags`, and `-skip-tags`.
Step-level retries and `until`-style retry conditions are supported for command resources via `retries`, `retry_delay_seconds`, `retry_backoff`, `retry_jitter_seconds`, and `until_contains`.
Command resources support `rescue_command` and `always_command` hooks for block/rescue/always-style error handling flows.
Explicit `require`/`before`/`notify`/`subscribe` resource relationships are supported in config and influence planner dependency ordering for event-driven orchestration.
Refresh-on-change execution semantics are supported via command guards (`only_if`, `unless`) and refresh controls (`refresh_only`, `refresh_command`) for event-triggered actions.
Task and plan framework for module-packaged actions is available via `/v1/tasks/definitions` and `/v1/tasks/plans`, including typed parameter contracts, sensitive-parameter masking in plan/preview responses, step `tags`, and async task execution with poll/timeout controls plus `include_tags`/`exclude_tags` run filters via `/v1/tasks/executions`.
Built-in template rendering now includes a safe function library (`upper`, `lower`, `trim`, `default`) and strict undefined-variable enforcement via template `strict_mode`, `POST /v1/templates/{id}/render`, and launch-time validation.
Handler/notification model for event-triggered resource actions is supported with `notify_handlers` plus top-level `handlers` definitions, with deduplicated post-change handler execution.
Delegated execution (`delegate_to`) is supported in resource definitions, allowing execution on a different inventory host than the target host.
Event bus integrations for webhook, Kafka, and NATS targets are available via `/v1/event-bus/targets`, `/v1/event-bus/publish`, and `/v1/event-bus/deliveries`.
External SaaS/webhook event ingress endpoints are available via `POST /v1/event-stream/ingest` and `POST /v1/event-stream/webhooks/ingest` (aliases to the core ingest pipeline).
Hermetic execution environments with pinned image digests are available via `/v1/execution/environments` and admission evaluation endpoints.
Short-lived execution credentials are available via `/v1/execution/credentials` with scope-aware validation and explicit revoke workflows.
Signed collection/image admission with client-side verification keyrings is available via `/v1/security/signatures/keyrings` and `/v1/security/signatures/admit-check`.
Runtime secret materialization with in-memory session lifecycle and consume-time zeroization is available via `/v1/secrets/runtime/sessions` and `/v1/secrets/runtime/consume`.
Time-bound delegation tokens for automated run pipelines are available via `/v1/access/delegation-tokens` with validation and revoke endpoints.
Multi-stage approval policies with quorum rules are available via `/v1/access/approval-policies`.
Break-glass workflows with audited approvals are available via `/v1/access/break-glass/requests` including approve/reject/revoke actions.
Just-in-time access grants for sensitive operations are available via `/v1/access/jit-grants` with token validation and revoke controls.
Compliance profile engine (CIS/STIG/custom), continuous scan configuration, and evidence exports (JSON/CSV/SARIF) are available via `/v1/compliance/profiles`, `/v1/compliance/continuous`, and `/v1/compliance/scans/{id}/evidence`.
Compliance exceptions with expiry + approval workflow and compliance scorecards by team/environment/service are available via `/v1/compliance/exceptions` and `/v1/compliance/scorecards`.
RBAC with scoped permissions is available via `/v1/access/rbac/roles`, `/v1/access/rbac/bindings`, and `/v1/access/rbac/check`.
ABAC with context-aware policy conditions is available via `/v1/access/abac/policies` and `/v1/access/abac/check`.
SSO enterprise identity integration is available via `/v1/identity/sso/providers`, `/v1/identity/sso/login/start`, and `/v1/identity/sso/login/callback`.
SCIM provisioning for teams and roles is available via `/v1/identity/scim/teams` and `/v1/identity/scim/roles`.
OIDC workload identity support is available via `/v1/identity/oidc/workload/providers` and `/v1/identity/oidc/workload/exchange`.
mTLS component trust/policy management with handshake verification is available via `/v1/security/mtls/authorities`, `/v1/security/mtls/policies`, and `/v1/security/mtls/handshake-check`.
Secrets manager integrations plus secret-usage tracing with redaction-by-default logs are available via `/v1/secrets/integrations`, `/v1/secrets/resolve`, and `/v1/secrets/traces`.
Signed module/provider package artifacts with provenance metadata and policy-driven verification are available via `/v1/packages/artifacts`, `/v1/packages/signing-policy`, and `/v1/packages/verify`.
Private/public registry visibility controls are available via package artifact `visibility` and `GET /v1/packages/artifacts?visibility=public|private`.
Module/provider provenance and vulnerability reports are available via `GET /v1/packages/provenance/report`.
Agent certificate issuance, policy-based autosigning/manual approval fallback, rotation, and revocation workflows are available via `/v1/agents/cert-policy`, `/v1/agents/csrs`, and `/v1/agents/certificates`.
Catalog compile/distribute flows with cached artifacts and signed replay for disconnected nodes are available via `/v1/agents/catalogs`, `POST /v1/agents/catalogs/replay`, and `/v1/agents/catalogs/replays`.
Certificate expiry SLO visibility and automatic renewal workflows are available via `/v1/agents/certificates/expiry-report` and `/v1/agents/certificates/renew-expiring`.
Identity bootstrap attestation gates (TPM/cloud IID evidence) are available via `/v1/agents/attestation/policy`, `/v1/agents/attestations`, and `/v1/agents/attestations/check` to enforce verification before certificate issuance.
Branch-based ephemeral environment previews are available via `/v1/gitops/previews` with lifecycle actions for promote/close and queued preview applies.
Branch-per-environment control-repo materialization is available via `/v1/gitops/environments/materialize` with generated environment configs and optional queued apply.
Webhook/API deployment triggers are available via `/v1/gitops/deployments/webhook` and `/v1/gitops/deployments/trigger`; CLI deployments are available via `masterchef deploy`.
Staging-to-live file-sync pipelines for distributed compiler/worker nodes are available via `/v1/gitops/filesync/pipelines`.
Promotion pipelines with immutable artifact pinning are available via `/v1/gitops/promotions` with stage-advance enforcement against digest drift.
Drift-aware GitOps reconciliation loop is available via `POST /v1/gitops/reconcile` with simulation guardrails and optional auto-enqueue.
Signed GitOps plan-artifact workflows are available via `/v1/gitops/plan-artifacts/sign` and `/v1/gitops/plan-artifacts/verify`.
Policy pull from control plane or signed Git sources is available via `/v1/policy/pull/sources`, `POST /v1/policy/pull/execute`, and `GET /v1/policy/pull/results` with signature verification enforcement for trusted Git sources.
Versioned policy bundles with lockfiles and staged policy-group/run-list promotions are available via `/v1/policy/bundles`, `POST /v1/policy/bundles/{id}/promote`, and `GET /v1/policy/bundles/{id}/promotions`.
Salt-style beacon/reactor compatibility patterns are available via `/v1/compat/beacon-reactor/rules` and `/v1/compat/beacon-reactor/emit`.
Salt-style grains compatibility and grain-query translation are available via `GET /v1/compat/grains` and `POST /v1/compat/grains/query`.
Inventory host grouping by roles, labels, and topology is available via `GET /v1/inventory/groups`.
Bulk runtime-host import from CMDB/asset systems is available via `POST /v1/inventory/import/cmdb` with dry-run support.
Import assistants for secrets, facts, and role/group hierarchies are available via `POST /v1/inventory/import/assist`.
Brownfield bootstrap from observed host state into desired-state baselines is available via `POST /v1/inventory/import/brownfield-bootstrap`.
Node classification rules based on facts/labels/policy are available via `/v1/inventory/classification-rules` and `POST /v1/inventory/classify`.
External node classifier (ENC) integration with third-party engines is available via `/v1/inventory/node-classifiers` and `POST /v1/inventory/node-classifiers/classify`.
Runtime host discovery and auto-enrollment are available via `/v1/inventory/enroll` and `/v1/inventory/runtime-hosts`, including lifecycle actions for bootstrap, activate, quarantine, and decommission.
Service-discovery-backed inventory sources (Consul, Kubernetes, cloud tags) are available via `/v1/inventory/discovery-sources` with sync-driven runtime host materialization via `POST /v1/inventory/discovery-sources/sync`; provider-specific cloud inventory sync for AWS/Azure/GCP/vSphere is available via `POST /v1/inventory/cloud-sync`.
Agent check-in jitter/splay controls are available via `POST /v1/agents/checkins` with deterministic per-agent splay assignment.
Message-bus dispatch mode for scalable agent execution is available via `/v1/agents/dispatch-mode` and `/v1/agents/dispatch` (`local` or `event_bus`).
Hybrid push/pull execution routing per environment is available via `/v1/agents/dispatch-environments`, allowing environment strategy overrides (`push`, `pull`, `hybrid`) while preserving global dispatch defaults.
Minimal-footprint and scalable deployment profile guidance is available via `/v1/control/deployment-profiles` and `POST /v1/control/deployment-profiles/evaluate`.
Proxy-minion mode for devices that cannot run full agents is available via `/v1/agents/proxy-minions` and `/v1/agents/proxy-minions/dispatch`.
Network device transport support (NETCONF, RESTCONF, API-driven, and plugin/custom extensions) is available via `/v1/execution/network-transports` and `/v1/execution/network-transports/validate`, and is enforced for proxy-minion bindings.
Multi-master control mode with centralized job/event cache is available via `/v1/control/multi-master/nodes` and `/v1/control/multi-master/cache` for cross-controller status and replay-oriented cache synchronization.
Multi-region control-plane federation is available via `/v1/control/federation/peers` and `/v1/control/federation/health`.
Fleet sharding and tenancy-aware scheduler partitioning are available via `/v1/control/scheduler/partitions` and `/v1/control/scheduler/partition-decision`.
Adaptive worker autoscaling recommendations based on queue depth and p95 latency are available via `/v1/control/autoscaling/policy` and `/v1/control/autoscaling/recommend`.
Cost-aware scheduling and throttling controls are available via `/v1/control/cost-scheduling/policies` and `/v1/control/cost-scheduling/admit`.
Bandwidth-aware artifact distribution and caching controls are available via `/v1/control/artifact-distribution/policies` and `/v1/control/artifact-distribution/plan`.
Short-lived stateless worker execution mode (to reduce long-running process drift) is configurable via `GET/POST /v1/control/workers/lifecycle`, including max jobs per worker and restart delay controls.
Long-running run leases with heartbeat and stale-lease recovery are available via `/v1/control/run-leases`, `/v1/control/run-leases/heartbeat`, and `/v1/control/run-leases/recover`.
Per-step execution snapshots for forensic analysis are available via `/v1/execution/snapshots` with filterable run/job queries and snapshot-by-id retrieval.
Adaptive concurrency control (host-health and failure-rate aware) is available via `/v1/execution/adaptive-concurrency/policy` and `/v1/execution/adaptive-concurrency/recommend`.
Transaction checkpoints and resumable execution are available via `/v1/execution/checkpoints` and `POST /v1/execution/checkpoints/resume`, which materializes a trimmed resume config for remaining steps.
Distributed execution locks to prevent conflicting runs are available via `/v1/control/execution-locks`, with optional lock binding on `POST /v1/jobs` using `lock_key`.
Per-tenant rate limits and noisy-neighbor protections are available via `/v1/control/tenancy/policies` and `/v1/control/tenancy/admit-check`.
Edge relay mode for intermittently connected sites is available via `/v1/edge-relay/sites` and `/v1/edge-relay/messages` with store-and-forward queueing and explicit delivery controls.
Egress-only execution-node connectivity through hosted hop/ingress relays is available via `/v1/execution/relays/endpoints` and `/v1/execution/relays/sessions`.
Hierarchical relay/syndic topology modeling for segmented mega-fleet routing is available via `/v1/control/syndic/nodes` and `/v1/control/syndic/route`.
Offline and air-gapped operation controls with signed offline bundle creation/verification are available via `/v1/offline/mode`, `/v1/offline/bundles`, and `/v1/offline/bundles/verify`.
Offline registry mirroring and synchronization workflows are available via `/v1/offline/mirrors` and `POST /v1/offline/mirrors/sync`.
FIPS-compatible cryptography mode controls and validation are available via `/v1/security/crypto/fips-mode` and `/v1/security/crypto/fips/validate`.
Masterless execution mode with local state/pillar rendering is available via `/v1/execution/masterless/mode` and `POST /v1/execution/masterless/render`.
Public plugin/module certification pipelines and publication quality gates are available via `/v1/packages/certify`, `/v1/packages/certification-policy`, and `POST /v1/packages/publication/check`.
Maintainer health metrics (test pass rate, issue latency, release cadence, open security issues) are available via `/v1/packages/maintainers/health`.
Module/provider quality scoring with craftsmanship tiers (`gold`/`silver`/`bronze`) and trust badges (`trusted`/`verified`/`community`) is available via `GET /v1/packages/quality` and `POST /v1/packages/quality/evaluate`.
Continuous conformance suites for built-in providers are available via `/v1/providers/conformance/suites` and `/v1/providers/conformance/runs`.
Versioned provider protocol descriptors with backward-compatibility negotiation and feature-flag capability mapping are available via `/v1/providers/protocol/descriptors` and `/v1/providers/protocol/negotiate`.
Curated content channels (`certified`, `validated`, `community`) with controlled sync policies and per-organization sync remotes secured by API tokens are available via `/v1/packages/content-channels`, `/v1/packages/content-channels/sync-policy`, and `/v1/packages/content-channels/remotes`.
Module/provider scaffolding generator with best-practice templates is available via `GET /v1/packages/scaffold/templates` and `POST /v1/packages/scaffold/generate`.
Breaking-change detection for module/provider interface updates is available via `POST /v1/packages/interface-compat/analyze`.
Control-plane canary upgrade workflow with automatic rollback on regression is available via `/v1/control/canary-upgrades`.
Health probe integrations for promotion/rollback gating are available via `/v1/control/health-probes`, `/v1/control/health-probes/checks`, and `/v1/control/health-probes/evaluate`.
gRPC automation API is available from `masterchef serve -grpc-addr :9090` with methods `/masterchef.v1.Control/Health` and `/masterchef.v1.Control/ListRuns`.
Agentless WinRM execution is supported in the executor for command/file resources (including deterministic localhost shim mode for CI/test paths).
Windows-oriented resource support now includes `registry` and `scheduled_task` resource types with deterministic local/WinRM-localhost shim state handling for convergent runs.
Pythonless managed-node execution path using portable remote runners is available via `/v1/execution/portable-runners` and `POST /v1/execution/portable-runners/select`.
Native scheduler-first recurring execution planning (systemd timers, cron, Windows Task Scheduler, with embedded fallback) is available via `/v1/execution/native-schedulers` and `POST /v1/execution/native-schedulers/select`, and association creation stores the selected scheduler backend.
Real-time event-driven converge triggering for policy/package/security changes is available via `GET/POST /v1/converge/triggers`, with trigger history, enqueue outcomes, and direct trigger lookup by id.
Virtual/exported resource discovery patterns are supported via `GET/POST /v1/resources/exported` and `POST /v1/resources/collect`, including collector selector syntax (`type=... and attrs.key=value`) for cross-node service lookup.
Per-node execution backend auto-selection is supported via `transport: auto` with host capability and metadata discovery (local/ssh/winrm).
Connection plugin architecture is available via executor transport handlers with support for custom `plugin/*` transports.
SSH bastion/jump-host and proxy-aware routing are supported via host fields `jump_address`, `jump_user`, `jump_port`, and `proxy_command`.
Cross-signal incident views that correlate events, alerts, runs, canary status, and observability links are available via `GET /v1/incidents/view`.
Built-in action docs with inline endpoint examples are available via `GET /v1/docs/actions`.
Documentation generator for modules/providers/policy APIs is available via `GET/POST /v1/docs/generate`.
Executable documentation examples verification is available via `POST /v1/docs/examples/verify` and `masterchef docs verify-examples`.
API docs version-diff views with deprecation timelines are available via `GET/POST /v1/docs/api/version-diff`.
Built-in style and best-practice analyzers for policy/module/provider code are available via `/v1/lint/style/rules` and `/v1/lint/style/analyze`.
Deterministic formatting and canonicalization for config and plan documents are available via `POST /v1/format/canonicalize`.
Per-step plan explainability (reason/trigger/outcome/risk hints) is available via `POST /v1/plans/explain`.
Execution graph visualization for UI/automation consumers is available via `POST /v1/plans/graph` (structured nodes/edges plus DOT and Mermaid renderings).
Resource graph query API for dependency/impact analysis is available via `POST /v1/plans/graph/query` with upstream/downstream traversal controls.
Change diff previews for each planned resource action are available via `POST /v1/plans/diff-preview`.
Cross-runner plan reproducibility checks for baseline/runner artifacts are available via `POST /v1/plans/reproducibility-check`.
Topology-aware blast-radius maps for impacted hosts/resources/dependencies are available via `POST /v1/control/blast-radius-map`.
Human-readable pre-apply risk summaries are available via `POST /v1/plans/risk-summary`.
Policy simulation and gating checks are available via `POST /v1/policy/simulate`.
Per-policy enforcement modes (`audit`, `apply-and-monitor`, `apply-and-autocorrect`) are configurable via `GET/POST /v1/policy/enforcement-modes` with decision evaluation support at `POST /v1/policy/enforcement-modes/evaluate`.
Simulation coverage reporting now includes per-resource-type support breakdown and explicit unsupported-action inventory in `POST /v1/policy/simulate` responses.
Post-run invariant checks with configurable severity are available via `POST /v1/control/invariants/check`.
Release blocker policy enforcement with craftsmanship tiers is available via `GET/POST /v1/release/blocker-policy`.
Release readiness scorecards that aggregate quality, reliability, and performance signals are available via `/v1/release/readiness/scorecards`.
Automated dependency update bot workflows with compatibility/performance verification are available via `/v1/release/dependency-bot/policy` and `/v1/release/dependency-bot/updates`.
Performance regression gates with latency, throughput, and error-budget thresholds are available via `/v1/release/performance-gates/policy` and `/v1/release/performance-gates/evaluate`.
Flake detection and quarantine workflows for unstable test cases are available via `/v1/release/tests/flake-policy`, `/v1/release/tests/flake-observations`, and `/v1/release/tests/flake-cases`.
Safety-aware test impact analysis for targeted CI runs is available via `POST /v1/release/tests/impact-analysis` with safe fallback recommendations.
End-to-end scenario test runner APIs for fleet simulations are available via `/v1/release/tests/scenarios` and `/v1/release/tests/scenario-runs`, with golden-run baselines and regression detection via `/v1/release/tests/scenario-baselines` and `/v1/release/tests/scenario-runs/{id}/compare-baseline`.
Ephemeral test environment runner workflows for integration checks are available via `/v1/release/tests/environments`, including run-check and teardown actions.
Load and soak test suites for control plane, scheduler, and execution workers are available via `/v1/release/tests/load-soak/suites` and `/v1/release/tests/load-soak/runs`.
Mutation testing support for critical provider logic is available via `/v1/release/tests/mutation/policy`, `/v1/release/tests/mutation/suites`, and `/v1/release/tests/mutation/runs`.
Property-based testing harness for idempotency and convergence invariants is available via `/v1/release/tests/property-harness/cases` and `/v1/release/tests/property-harness/runs`.
Pinned toolchain reproducibility checks for local/CI pipelines are available via `masterchef release toolchain-check`.
Activity timeline filtering for audit workflows is available via `GET /v1/activity` query filters and `GET /v1/activity/audit-timeline` identity/resource categories.
Migration assessment reports with parity/risk/urgency scoring are available via `/v1/migrations/assess` and `/v1/migrations/reports`.
Solution pack catalog is available via `/v1/solution-packs` and workspace-template catalog/bootstrap flows are available via `/v1/workspace-templates`.
Curated use-case templates for rollout/patching/DR/migration workflows are available via `/v1/use-case-templates`.

## Release Tooling (Current CLI)

- `masterchef dev -state-dir .masterchef/dev -addr :8080 -grpc-addr :9090`
- `masterchef release sbom -root . -files . -o sbom.json`
- `masterchef release sign -sbom sbom.json -key policy-private.key -o signed-sbom.json`
- `masterchef release verify -signed signed-sbom.json -pub policy-public.key`
- `masterchef release cve-check -root . -advisories advisories.json -blocked-severities critical,high`
- `masterchef release attest -root . -o attestation.json -test-cmd "go test ./..."`
- `masterchef release upgrade-assist -baseline baseline-api.json -current current-api.json -format human`
- `masterchef release toolchain-check -root . -format human`

Release provenance attestations include source commit/branch/tag/remote linkage, test-run output digest, and build-environment metadata.

## Repository Documents

- `README.md`: project overview and design decisions
- `FEATURES.md`: capability inventory and parity goals
- `agents.md`: runtime agent model and responsibilities
- `ROADMAP.md`: phased build and release plan
