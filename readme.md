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
Drift trend analytics with root-cause hints/remediations are available via `GET /v1/drift/insights`.
Run-step observability correlation IDs are available via `GET /v1/runs/{id}/correlations`.
Notification integrations are managed via `/v1/notifications/targets` and `/v1/notifications/deliveries` for ChatOps/incident/ticket routing.
Change records and approval workflows are exposed via `/v1/change-records` to tie execution to ticketed change control.
Self-service runbook catalog with approval-gated launches is available via `/v1/runbooks`.
Operator checklist mode for high-risk changes is available via `/v1/control/checklists`.
Guided topology advisor for scaling from small teams to large fleets is available via `GET /v1/control/topology-advisor`.
Saved views with share tokens and dashboard pinning are available via `/v1/views`.
Bulk operation staging with preview/conflict detection/confirmed execution is available via `/v1/bulk/preview` and `/v1/bulk/execute`.
Persona-based home views for SRE/platform/release/service-owner workflows are available via `GET /v1/views/home`.
Workload-centric operational views grouped by service/application are available via `GET /v1/views/workloads`.
Fleet node views with cursor-based incremental loading and compact mode are available via `GET /v1/fleet/nodes`.
Fleet health SLO/error-budget views are available via `GET /v1/fleet/health`.
Universal command-palette search across hosts, services, runs, policies, and modules is available via `GET /v1/search`.
Data bag/global object store with encrypted item support and structured search is available via `/v1/data-bags` and `/v1/data-bags/search`.
Chef-style role and environment objects with deterministic per-environment resolution are available via `/v1/roles`, `/v1/environments`, and `GET /v1/roles/{name}/resolve`.
Encrypted variable files with key rotation (Vault-style) are available via `/v1/vars/encrypted/files` and `/v1/vars/encrypted/keys`.
Pillar/Hiera-style hierarchical data resolution with explicit merge strategies is available via `POST /v1/pillar/resolve`.
Fact caching with TTL/invalidation and Salt Mine-style cross-node fact queries are available via `/v1/facts/cache` and `POST /v1/facts/mine/query`.
Variable precedence resolution with source graph, conflict detection, hard-fail policy, and explain output is available via `POST /v1/vars/resolve` and `POST /v1/vars/explain`.
External variable source plugins (`inline`, `env`, `file`, `http`) are available via `POST /v1/vars/sources/resolve`.
Unified CLI now includes `observe` and `drift` commands for local run telemetry and drift trend inspection.
Interactive CLI TUI inspection is available via `masterchef tui` with run browsing and per-step detail views.
Ansible-compatible plugin extension points (`callback`, `lookup`, `filter`, `vars`, `strategy`) are available via `/v1/plugins/extensions`.
Execution strategy controls (`linear`, `free`, `serial`) with failure thresholds (`max_fail_percentage`, `any_errors_fatal`) are supported in config and executor runtime.
Failure-domain-aware serial orchestration is supported with `execution.failure_domain` (`rack|zone|region`) to interleave hosts across domains.
Disruption budget definitions and rollout-gating evaluation are available via `/v1/control/disruption-budgets` and `/v1/control/disruption-budgets/evaluate`.
Privilege escalation controls for command resources are supported via `become` and `become_user`, with explicit run-result audit markers.
Session recording artifacts for privileged remote command executions are emitted under `.masterchef/sessions` and linked from run output.
Selective and targeted execution filters are supported in `check`/`apply` via `-hosts`, `-resources`, `-tags`, and `-skip-tags`.
Step-level retries and `until`-style retry conditions are supported for command resources via `retries`, `retry_delay_seconds`, and `until_contains`.
Command resources support `rescue_command` and `always_command` hooks for block/rescue/always-style error handling flows.
Delegated execution (`delegate_to`) is supported in resource definitions, allowing execution on a different inventory host than the target host.
Event bus integrations for webhook, Kafka, and NATS targets are available via `/v1/event-bus/targets`, `/v1/event-bus/publish`, and `/v1/event-bus/deliveries`.
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
Agent certificate issuance, policy-based autosigning/manual approval fallback, rotation, and revocation workflows are available via `/v1/agents/cert-policy`, `/v1/agents/csrs`, and `/v1/agents/certificates`.
Branch-based ephemeral environment previews are available via `/v1/gitops/previews` with lifecycle actions for promote/close and queued preview applies.
Branch-per-environment control-repo materialization is available via `/v1/gitops/environments/materialize` with generated environment configs and optional queued apply.
Webhook/API deployment triggers are available via `/v1/gitops/deployments/webhook` and `/v1/gitops/deployments/trigger`; CLI deployments are available via `masterchef deploy`.
Staging-to-live file-sync pipelines for distributed compiler/worker nodes are available via `/v1/gitops/filesync/pipelines`.
Promotion pipelines with immutable artifact pinning are available via `/v1/gitops/promotions` with stage-advance enforcement against digest drift.
Drift-aware GitOps reconciliation loop is available via `POST /v1/gitops/reconcile` with simulation guardrails and optional auto-enqueue.
Signed GitOps plan-artifact workflows are available via `/v1/gitops/plan-artifacts/sign` and `/v1/gitops/plan-artifacts/verify`.
Salt-style beacon/reactor compatibility patterns are available via `/v1/compat/beacon-reactor/rules` and `/v1/compat/beacon-reactor/emit`.
Inventory host grouping by roles, labels, and topology is available via `GET /v1/inventory/groups`.
Runtime host discovery and auto-enrollment are available via `/v1/inventory/enroll` and `/v1/inventory/runtime-hosts`, including lifecycle actions for bootstrap, activate, quarantine, and decommission.
Agent check-in jitter/splay controls are available via `POST /v1/agents/checkins` with deterministic per-agent splay assignment.
Message-bus dispatch mode for scalable agent execution is available via `/v1/agents/dispatch-mode` and `/v1/agents/dispatch` (`local` or `event_bus`).
gRPC automation API is available from `masterchef serve -grpc-addr :9090` with methods `/masterchef.v1.Control/Health` and `/masterchef.v1.Control/ListRuns`.
Agentless WinRM execution is supported in the executor for command/file resources (including deterministic localhost shim mode for CI/test paths).
Per-node execution backend auto-selection is supported via `transport: auto` with host capability and metadata discovery (local/ssh/winrm).
Connection plugin architecture is available via executor transport handlers with support for custom `plugin/*` transports.
SSH bastion/jump-host and proxy-aware routing are supported via host fields `jump_address`, `jump_user`, `jump_port`, and `proxy_command`.
Cross-signal incident views that correlate events, alerts, runs, canary status, and observability links are available via `GET /v1/incidents/view`.
Built-in action docs with inline endpoint examples are available via `GET /v1/docs/actions`.
Per-step plan explainability (reason/trigger/outcome/risk hints) is available via `POST /v1/plans/explain`.
Topology-aware blast-radius maps for impacted hosts/resources/dependencies are available via `POST /v1/control/blast-radius-map`.
Human-readable pre-apply risk summaries are available via `POST /v1/plans/risk-summary`.
Policy simulation and gating checks are available via `POST /v1/policy/simulate`.
Post-run invariant checks with configurable severity are available via `POST /v1/control/invariants/check`.
Release blocker policy enforcement with craftsmanship tiers is available via `GET/POST /v1/release/blocker-policy`.
Activity timeline filtering for audit workflows is available via `GET /v1/activity` query filters.
Migration assessment reports with parity/risk/urgency scoring are available via `/v1/migrations/assess` and `/v1/migrations/reports`.
Solution pack catalog is available via `/v1/solution-packs` and workspace-template catalog/bootstrap flows are available via `/v1/workspace-templates`.
Curated use-case templates for rollout/patching/DR/migration workflows are available via `/v1/use-case-templates`.

## Release Tooling (Current CLI)

- `masterchef release sbom -root . -files . -o sbom.json`
- `masterchef release sign -sbom sbom.json -key policy-private.key -o signed-sbom.json`
- `masterchef release verify -signed signed-sbom.json -pub policy-public.key`
- `masterchef release cve-check -root . -advisories advisories.json -blocked-severities critical,high`
- `masterchef release attest -root . -o attestation.json -test-cmd "go test ./..."`
- `masterchef release upgrade-assist -baseline baseline-api.json -current current-api.json -format human`

## Repository Documents

- `README.md`: project overview and design decisions
- `FEATURES.md`: capability inventory and parity goals
- `agents.md`: runtime agent model and responsibilities
- `ROADMAP.md`: phased build and release plan
