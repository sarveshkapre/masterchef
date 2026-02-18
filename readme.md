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
Noise-reduction alert inbox is available via `GET/POST /v1/alerts/inbox` with dedup, suppression windows, and routing.
Run failure triage bundles are exportable via `POST /v1/runs/{id}/triage-bundle` for incident debugging context.

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
