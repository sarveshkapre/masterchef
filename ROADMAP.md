# Roadmap

This roadmap starts in Q1 2026 and prioritizes correctness, safety, and extensibility over feature count.

## Strategy

1. Build a robust core engine before broad integrations.
2. Deliver dual-mode execution early (agentless first, then agent pull loop).
3. Keep provider/plugin boundaries strict from the start.
4. Treat policy, auditability, and observability as first-class, not add-ons.

## Weaknesses We Intentionally Address

1. Legacy DSL lock-in
- Approach: typed open schemas (YAML + CUE), not a closed language runtime.

2. Non-deterministic or hard-to-review changes
- Approach: mandatory plan artifacts and stable ordering.

3. Fragile ecosystem extensions
- Approach: versioned provider protocol + conformance test suite.

4. Security gaps in ad hoc remote execution
- Approach: signed bundles/providers, short-lived credentials, policy gates.

5. Push-only or pull-only operational limitations
- Approach: unified engine that supports both agentless push and agent pull.

## Language and Platform Decision

- Core language: Go
- Reasoning:
  - Good operational profile for CLIs, agents, and controllers
  - Strong concurrency model for distributed orchestration
  - Simple static binary distribution across Linux/macOS/Windows
  - Broad contributor pool and mature cloud-native ecosystem

## Release Phases

## Phase 0: Foundations (Q1 2026)

Goals:
- Finalize architecture and contracts.
- Lock config schema and IR format.

Deliverables:
- Architecture decision records (ADRs)
- Typed config spec and validator
- IR graph model and serialization format
- Conformance spec for core providers

Exit criteria:
- Reproducible compile of config to IR
- Deterministic plan output for identical inputs
- Spec review sign-off from maintainers

## Phase 1: Core Engine MVP (Q2 2026)

Goals:
- Ship a useful local and SSH-based orchestration MVP.

Deliverables:
- CLI: `init`, `validate`, `plan`, `apply`, `diff`
- Resource providers: file/package/service/user/group/command
- Static inventory support
- Plan artifact generation and apply execution engine
- Structured logs and local run history

Exit criteria:
- End-to-end runs on Ubuntu, Debian, RHEL, and macOS targets
- Idempotency pass rate >= 99% in test matrix
- Dry-run and apply parity validated

## Phase 2: Control Plane Alpha (Q3 2026)

Goals:
- Move from local-only workflows to team-ready operations.

Deliverables:
- Control plane API and scheduler
- PostgreSQL state/event store
- Basic RBAC
- Remote run queue and approvals
- Initial web UI for plans and runs

Exit criteria:
- Multi-user concurrency without state corruption
- Full audit trail for every run
- Horizontal scaling test for controller workers

## Phase 3: Agent Mode and Drift (Q4 2026)

Goals:
- Add continuous convergence and drift remediation.

Deliverables:
- Node agent with signed policy pull
- Drift detection and alert pipeline
- Auto-remediation policies (opt-in)
- Agent upgrade channels and rollout control

Exit criteria:
- Fleet drift detection latency < 5 minutes (target profile)
- Controlled remediation success rate >= 95% in staging
- Agent-controller protocol compatibility tests passing

## Phase 4: Security and Compliance (Q1 2027)

Goals:
- Make security and compliance production-grade.

Deliverables:
- OPA-style policy integration hooks
- Secrets manager integrations
- Signed provider/module registry
- Compliance profiles and evidence exports

Exit criteria:
- Policy gate coverage for all built-in providers
- Zero plaintext secret persistence in standard path tests
- Compliance reporting validated in regulated test scenarios

## Phase 5: Extensibility and Ecosystem (Q2 2027)

Goals:
- Enable strong community growth and enterprise integration.

Deliverables:
- Provider SDK + scaffolding tools
- Public module registry workflows
- Event hooks/webhooks and notification integrations
- Terraform/CI/CD interoperability references

Exit criteria:
- Third-party provider onboarding docs + golden tests
- Backward-compatible provider protocol policy published
- At least 5 external providers validated by community testers

## Phase 6: Production Hardening and v1.0 (Q3-Q4 2027)

Goals:
- Deliver a stable, documented, high-confidence v1.0.

Deliverables:
- HA deployment blueprints
- Performance tuning and scale benchmarks
- Disaster recovery and backup procedures
- Long-term support policy and upgrade guarantees

Exit criteria:
- 10k-node scale test in reference environment
- Published SLOs for control plane and agent channels
- v1.0 release candidate with migration guides

## Cross-Cutting Workstreams (All Phases)

1. Developer experience
- Fast local dev environment
- Reproducible integration test harness
- Clear error messages and actionable diagnostics

2. Testing and quality
- Unit, integration, chaos, and soak testing
- Determinism and idempotency regression suites
- Security scanning and signed release artifacts

3. Open source governance
- RFC/ADR process
- Contributor guide and maintainer rotation
- Transparent release notes and deprecation policy

## Success Metrics

- Plan determinism rate
- Idempotency pass rate
- Drift detection and remediation reliability
- Mean time to recovery for failed runs
- Adoption metrics: contributors, modules, active deployments

## Risks and Mitigations

1. Scope sprawl
- Mitigation: strict phase exit criteria and feature freeze windows.

2. Provider quality variance
- Mitigation: mandatory conformance + certification badges.

3. Security regressions
- Mitigation: threat modeling each release and signed artifacts everywhere.

4. Operational complexity
- Mitigation: opinionated defaults, guided setup, and reference architectures.
