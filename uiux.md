# Masterchef UI/UX Implementation Guide

## Goal

Use the shadcn/ui Tailwind v4 model and Radix primitives directly so Masterchef ships a modular, accessible, production-grade UI system with consistent behavior across all surfaces.

## Canonical References

- shadcn Tailwind v4 docs: https://ui.shadcn.com/docs/tailwind-v4
- shadcn components/docs root: https://ui.shadcn.com/docs
- Radix primitives docs: https://www.radix-ui.com/primitives/docs/overview/introduction
- Radix composition (`asChild`) guide: https://www.radix-ui.com/primitives/docs/guides/composition
- Local reference codebase (checked out): `/Users/sarvesh/code/ui`
- Local v4 component registry: `/Users/sarvesh/code/ui/apps/v4/registry/new-york-v4/ui`
- Local v4 style foundation: `/Users/sarvesh/code/ui/apps/v4/styles/globals.css`
- Local v4 shadcn config shape: `/Users/sarvesh/code/ui/apps/v4/components.json`

## Required Stack

- Tailwind CSS v4
- Radix primitives
- shadcn-style component architecture (new-york style baseline)
- `class-variance-authority` (`cva`) for component variants
- shared `cn` utility for class merging
- `tw-animate-css` for state transitions

## Component Architecture Rules

1. Layering
- Design tokens: CSS variables and Tailwind v4 token mapping.
- Primitive wrappers: Radix primitives wrapped in project components.
- Product composites: composed building blocks for SRE workflows.
- Screens: route/page-level orchestration only.

2. Component contracts
- Every shared component exposes a stable, typed API.
- Use `cva` for variant and size systems; no scattered conditional classes.
- Use `data-slot` markers for stable DOM/test hooks.
- Prefer `asChild` composition on interactive primitives.
- Use `data-state` and related Radix attributes for animations/state styling.

3. Styling system
- Centralize theme variables in global styles.
- Use semantic tokens (`--background`, `--foreground`, `--muted`, etc.).
- Keep product-specific overrides in scoped layers; do not fork base primitives without need.

4. Accessibility
- Keyboard support is required for all interactive paths.
- Keep focus-visible ring treatment consistent.
- Respect ARIA contracts of Radix primitives.
- Include visible labels/help/error states for forms and operator workflows.

## Implementation Workflow

1. Check if a shadcn/Radix component already exists in local v4 registry.
2. Reuse/adapt the registry pattern before inventing custom structure.
3. Add or update variants with `cva`, preserving backwards compatibility.
4. Validate keyboard and focus behavior.
5. Add unit/integration coverage for state and interaction paths.
6. Run verification before commit.

## Product UX Rules for Masterchef

- Optimize for high-signal operational workflows: plans, runs, drift, incidents, approvals.
- Prefer dense but readable layouts for SRE usage.
- Keep critical actions explicit with clear risk labels and confirmations.
- Make command palette, filtering, and keyboard workflows first-class paths.
- Default to deterministic, auditable UI behavior and predictable state transitions.

## Definition of Done (UI Changes)

- Uses shadcn v4 + Radix conventions.
- Uses shared tokens and variant system (no ad-hoc styling sprawl).
- Accessible by keyboard and screen-reader-aware primitives.
- Includes automated tests for key interaction/state paths.
- Verified locally before commit and pushed directly to `main`.
