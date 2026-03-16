---
title: In-Memory HTTP API Endpoints
updated: 2026-03-16
category: Integration
tags: [http, api, filters, sessions]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/architecture/canonical-project-identity.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/gemini-logs-json-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# In-Memory HTTP API Endpoints

## Overview

The API layer serves project, task, and session data from in-memory startup aggregates, with one on-demand transcript endpoint for message detail. EPIC-5 shifted list and drill-down contracts to canonical project identity, added richer aggregate metadata, and kept sort/filter logic in the API instead of pushing it into the frontend.

## Implementation

Endpoints:
- `GET /api/health`
- `GET /api/projects`
- `GET /api/tasks?project=<canonicalProjectID>[&sort=cost]`
- `GET /api/sessions?task=<id>[&project=<canonicalProjectID>][&sort=recent|cost]`
- `GET /api/sessions/:id/messages`

Behavior highlights:
- `/api/projects` groups sessions by `canonicalProjectID`, not raw provider path.
- `/api/projects` returns `canonicalProjectID`, `displayName`, `aliases`, `providerCoverage`, `sessionCount`, `taskCount`, `totalCost`, and `unknownPricing`.
- `task=manual` is a virtual task that matches manual and untagged sessions.
- `/api/tasks` is scoped to a single canonical project and returns `taskID`, `providerCoverage`, `sessionCount`, `totalCost`, and `unknownPricing`.
- `/api/tasks` appends a virtual `"manual"` entry when non-Doug sessions exist in the selected project (unless `doug_only=true`).
- `/api/tasks` requires `project=<canonicalProjectID>` and sorts by descending `totalCost`; `sort=cost` is the only supported sort key.
- `/api/sessions` requires `task=<id>` and supports `sort=recent` (default) or `sort=cost`, both descending.
- `/api/sessions` returns `canonicalProjectID` and `rawProjectPath` together so clients can drill down by stable identity while still showing provider-native context.
- Errors use a stable envelope: `{"error":"..."}`.
- Session list responses include `class`, `model`, and precomputed `duration` in milliseconds when Phase 1 could derive it from timestamps.
- Message responses preserve raw provider content parts while attaching computed per-turn cost fields.
- Provider filters support all currently integrated providers (`claude`, `gemini`, `codex`) and can be combined.

Only `/api/sessions/:id/messages` triggers Phase 2 transcript parsing through provider `LoadTranscript`.

### Response notes

- `providerCoverage` is a sorted provider name list assembled from matching sessions.
- `unknownPricing` rolls up any pricing uncertainty from the aggregated result row.
- `aliases` currently include raw provider paths plus any provider-supplied alternate project names that differ from `canonicalProjectID`.
- `project` on `/api/sessions` is optional, but it is the only way to scope the virtual `manual` task to one project.

## Key Decisions

- **Dedicated `api` package**: Keeps HTTP concerns separate from startup orchestration.
- **Pure `filterSessions` helper**: Centralized filter logic across handlers and tests.
- **Canonical project drill-down**: `/api/tasks` and `/api/sessions` accept canonical project IDs so cross-provider project merges stay intact through navigation.
- **Virtual manual task ID**: Preserves non-Doug usage visibility without requiring source task tags.
- **Prefix route for message endpoint**: `/api/sessions/` captures `:id/messages` without external router deps.
- **Provider-agnostic API contracts**: New providers are integrated without changing endpoint shapes.

## Usage Example

```bash
curl '/api/projects?provider=claude&provider=codex'
curl '/api/tasks?project=project-alpha&provider=claude&doug_only=true&sort=cost'
curl '/api/sessions?task=TASK-123&project=project-alpha&sort=recent'
curl '/api/sessions?task=manual&project=project-alpha&sort=cost'
```

## Edge Cases & Gotchas

- Missing `project` on `/api/tasks` and missing `task` on `/api/sessions` return `400`.
- Unsupported `sort` values on `/api/tasks` or `/api/sessions` return `400`.
- Unknown provider name in session metadata during message lookup returns `500`.
- `task=manual` with `doug_only=true` returns no rows by design.
- `taskID` in `/api/sessions` remains empty for manual and untagged sessions even though those rows are selected by the virtual `task=manual` filter.

## Related Topics

See [Two-Phase Session Loading Architecture](../architecture/two-phase-session-loading.md) for startup sequencing guarantees.
See [Canonical Project Identity](../architecture/canonical-project-identity.md) for how `canonicalProjectID` and `displayName` are resolved.
See [Dashboard Navigation and Cost Views](../features/dashboard-project-and-task-views.md) for endpoint consumption in the React UI.
