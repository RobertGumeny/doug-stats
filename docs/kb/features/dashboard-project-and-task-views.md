---
title: Dashboard Project and Task Views
updated: 2026-03-06
category: Features
tags: [react, ui, filters, costs]
related_articles:
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/features/cli-flags.md
---

# Dashboard Project and Task Views

## Overview

The frontend replaces the placeholder page with a two-view dashboard: project list and task list. It includes health-based startup polling, provider and Doug-only filtering, breadcrumb navigation, and prominent cost badges that distinguish known and unknown totals.

## Implementation

State-driven UI flow in `frontend/src/App.tsx`:
- `connecting` view polls `GET /api/health` every second until ready
- `projects` view loads `/api/projects` with active filters
- `tasks` view loads `/api/tasks?project=...` with active filters

Filters:
- Multi-select provider chips (`claude`, `gemini`, `codex`)
- Doug-only checkbox (`doug_only=true`)

Presentation:
- Rows sorted by descending cost
- Manual/untagged usage shown in a dedicated section
- Cost badge renders `?` when `unknown=true`, else `$<fixed 4dp>`

## Key Decisions

- **No router dependency**: Two views are handled by local React state.
- **Relative `/api/...` fetch paths**: Works both in local dev and embedded Go serving.
- **Manual sessions separated visually**: Keeps Doug task totals distinct while retaining visibility into non-tagged usage.
- **Unknown cost fallback marker**: UI preserves uncertainty from backend `Cost.Unknown`.

## Edge Cases & Gotchas

- If API calls fail, lists are reset to empty arrays rather than stale data.
- Provider filter options are static; providers without data naturally yield empty results.

## Related Topics

See [CLI Flags & Provider Auto-Detection](./cli-flags.md) for startup provider directory discovery.
