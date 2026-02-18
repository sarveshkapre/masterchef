# Features

This document defines the target feature set for Masterchef and maps it to legacy tool capabilities.

## Capability Categories

1. Configuration model
- Declarative, typed configuration (YAML + CUE constraints)
- Resource graph with explicit dependency edges
- Variable layering (global, environment, role, host, run-time)
- Reusable modules and composition

2. Planning and execution
- Mandatory `plan` before `apply`
- Deterministic run ordering from dependency DAG
- Idempotent resource providers
- Parallel execution with bounded concurrency controls
- Check mode (dry-run) and diff previews
- Partial apply and targeted resource scopes

3. Transport modes
- Agentless mode over SSH/WinRM
- Agent mode with periodic converge loop
- Hybrid mode by node/group policy

4. Inventory and orchestration
- Dynamic and static inventory
- Grouping by labels, tags, roles, topology
- Rolling updates with health gates
- Maintenance windows and disruption budgets

5. State, drift, and reconciliation
- Desired vs observed state tracking
- Drift detection alerts
- Auto-remediation rules (opt-in)
- Safe rollback points per run

6. Security and secrets
- Native integration with major secret managers
- Ephemeral credentials and short-lived execution tokens
- Command/resource allowlists and deny policies
- Signed module/provider packages

7. Policy and compliance
- Pre-apply policy validation
- Continuous compliance scanning
- CIS/STIG-style profile support
- Compliance evidence export (JSON, SARIF, CSV)

8. Extensibility
- Provider SDK with strict idempotency contracts
- Event hooks and webhooks
- Versioned module registry
- Backward-compatible provider protocol guarantees

9. UX and operations
- Unified CLI (`init`, `validate`, `plan`, `apply`, `observe`, `drift`, `policy`)
- Web UI for run history, approvals, and compliance posture
- REST/gRPC APIs for automation and ecosystem integration
- Structured logs, metrics, traces, and run replay

## Feature Parity and Better-Than-Parity Goals

1. Chef-like strengths to include
- Rich resource model
- Strong convergence semantics
- Mature policy/compliance workflows

2. Ansible-like strengths to include
- Fast onboarding and agentless bootstrap
- Human-readable run output
- Flexible inventory-driven orchestration

3. Puppet-like strengths to include
- Continuous desired-state enforcement with agent mode
- Clear model for drift remediation
- Scalable environment and role separation

4. Gaps to fix
- Remove DSL lock-in by using open typed schemas.
- Make plans deterministic and reviewable by default.
- Enforce provider quality via conformance tests.
- Provide a single control plane for both push and pull models.

## MVP Feature Cut (v0.1)

- Typed config + schema validation
- Core resources: `file`, `package`, `service`, `user`, `group`, `command`
- Static inventory + SSH agentless execution
- Plan/apply with diff output
- Run logs + local state tracking
- Minimal policy checks (blocking unsafe resources/commands)

## v1.0 Minimum Product Bar

- Agent mode converge loop
- PostgreSQL-backed control plane
- RBAC and audit logs
- Secrets manager integrations
- Drift dashboard and alerting
- Provider SDK and signed providers
- HA controller deployment reference

## Non-Goals (Initial)

- Full cloud provisioning replacement for Terraform
- Monolithic built-in CMDB
- Hard dependency on a hosted SaaS control plane
