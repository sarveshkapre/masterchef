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

Planning and architecture phase.

## Repository Documents

- `README.md`: project overview and design decisions
- `FEATURES.md`: capability inventory and parity goals
- `agents.md`: runtime agent model and responsibilities
- `ROADMAP.md`: phased build and release plan
