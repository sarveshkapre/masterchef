# Agents

This document specifies the Masterchef agent architecture and operating model.

## Codex Delivery Standards

- All code generated for Masterchef must be production-grade by default.
- Implementations must prioritize modular design with clear boundaries between components.
- Every feature must be extensible through stable interfaces and plugin/provider contracts.
- All core paths must be designed for horizontal scalability and high-concurrency operation.
- Code must be testable by design with dependency injection and deterministic behavior.
- Every implemented feature must include automated tests (unit and integration as applicable).
- Every implemented feature must be verified locally before commit, including lint, tests, and run-path validation.
- Failures in verification block merge-quality completion until fixed.
- Changes are committed directly to `main` after verification unless explicitly instructed otherwise.

## Goals

- Support both agentless and agent-based workflows in one platform
- Keep agent behavior deterministic and auditable
- Minimize trust and privilege on managed nodes

## Agent Types

1. Node Agent
- Runs on managed hosts.
- Pulls signed policies on schedule.
- Evaluates desired vs observed state.
- Executes local providers and reports results.

2. Ephemeral Execution Agent
- Short-lived worker started by control plane for one-off operations.
- Used for controlled remote run jobs and remediation tasks.

3. Observer Agent
- Collects facts, inventory metadata, and compliance evidence.
- Can run in read-only mode for high-security environments.

## Agentless Mode

- Control plane connects directly to hosts via SSH/WinRM.
- Useful for bootstrap, low-footprint environments, and temporary orchestration.
- Uses the same plan artifacts as agent mode to preserve behavior consistency.

## Core Agent Responsibilities

1. Policy ingestion
- Accept only signed policy bundles.
- Validate compatibility against local runtime version.

2. Plan execution
- Run only plan-approved resource actions.
- Enforce step-level timeouts and retry budgets.

3. State reporting
- Emit execution results, drift findings, and resource-level evidence.
- Never store long-lived secrets in logs or local plaintext caches.

4. Safety controls
- Respect deny policies and command/resource guardrails.
- Require explicit approval tokens for high-risk operations.

## Security Model

- mTLS identity for all agent-control plane communication
- Per-agent short-lived credentials
- Signed provider binaries/modules
- Optional sandboxing for third-party providers (WASM/WASI)
- Tamper-evident audit trail for every run

## Upgrade Strategy

- Rolling upgrade channels: `stable`, `candidate`, `edge`
- Control plane enforces minimum supported agent protocol
- Agents support N-1 control plane version compatibility
- Automatic rollback to last known good runtime on failed self-upgrade

## Reliability Model

- Local run queue with bounded persistence
- Exponential backoff for control plane reconnects
- Idempotent run replays after transient failures
- Circuit breakers for repeatedly failing resource actions

## Plugin and Provider Contracts

- Providers declare capabilities and side-effect boundaries
- Providers must pass idempotency and convergence conformance tests
- Versioned provider protocol over gRPC/protobuf
- Strict separation between control-plane logic and provider implementation

## Minimum v0.1 Agent Scope

- Node fact collection
- Policy pull and signature validation
- Core resource execution
- Result upload and local audit cache
