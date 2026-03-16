# Knowledge Base

Reference articles for doug-stats. Organized by category.

## Architecture

- [Two-Phase Session Loading](./architecture/two-phase-session-loading.md) — startup vs. on-demand session loading model
- [Canonical Project Identity](./architecture/canonical-project-identity.md) — cross-provider project grouping via resolver priority chain

## Patterns

- [Claude JSONL Provider](./patterns/claude-jsonl-provider.md) — `history.jsonl`-driven session ingestion
- [Gemini logs.json Provider](./patterns/gemini-logs-json-provider.md) — project-scoped log index and chat file parsing
- [Codex SQLite + Rollout Provider](./patterns/codex-sqlite-rollout-provider.md) — SQLite thread index + JSONL rollout parsing

## Integration

- [HTTP API Endpoints](./integration/http-api-endpoints.md) — in-memory REST API surface

## Infrastructure

- [Build Pipeline](./infrastructure/build-pipeline.md)
- [CI/CD](./infrastructure/ci-cd.md)

## Dependencies

- [Model Pricing and Aggregation](./dependencies/model-pricing-and-aggregation.md) — cost calculation per provider token category

## Features

- [CLI Flags](./features/cli-flags.md)
- [Dashboard Project and Task Views](./features/dashboard-project-and-task-views.md)
