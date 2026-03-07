---
title: Dashboard Navigation and Cost Views
updated: 2026-03-06
category: Features
tags: [react, ui, filters, costs, transcript]
related_articles:
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/features/cli-flags.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/infrastructure/build-pipeline.md
---

# Dashboard Navigation and Cost Views

## Overview

The frontend dashboard renders four primary views: Projects, Tasks, Sessions, and Transcript. It includes health-based startup polling, provider and Doug-only filtering, breadcrumb navigation, and cost displays at aggregate and per-turn levels with explicit unknown-model handling.

## Implementation

State-driven UI flow in `frontend/src/App.tsx`:
- `connecting` view polls `GET /api/health` every second until ready
- `projects` view loads `/api/projects` with active filters
- `tasks` view loads `/api/tasks?project=...` with active filters
- `sessions` view loads `/api/sessions?task=...&project=...`
- `transcript` view loads `/api/sessions/:id/messages`

Filters:
- Multi-select provider chips (`claude`, `gemini`, `codex`)
- Doug-only checkbox (`doug_only=true`)

Presentation:
- Rows sorted by descending cost
- Manual/untagged usage shown in a dedicated section
- Cost badge renders `?` when `unknown=true`, else `$<fixed 4dp>`
- Session rows include provider, class, model, start time, and derived duration (when timestamps are available)
- Transcript turns are displayed chronologically with distinct user/assistant styles
- `tool_use` and `tool_result` blocks are collapsed by default with per-block expand toggles
- Assistant turns show inline per-turn cost plus a cache-tier pricing note

## Key Decisions

- **No router dependency**: Navigation across all views is handled by local React state and breadcrumbs.
- **Relative `/api/...` fetch paths**: Works both in local dev and embedded Go serving.
- **Manual sessions separated visually**: Keeps Doug task totals distinct while retaining visibility into non-tagged usage.
- **Unknown cost fallback marker**: UI preserves uncertainty from backend `Cost.Unknown`.
- **Transcript utilities extracted**: Shared formatting and content extraction logic lives in `frontend/src/transcript.ts` to keep `App.tsx` focused on view composition.

## Edge Cases & Gotchas

- If API calls fail, lists are reset to empty arrays rather than stale data.
- Provider filter options are static; providers without data naturally yield empty results.
- Session duration is omitted when message timestamps are missing or unparsable.
- Transcript ordering is sorted by parsed timestamp with stable index fallback for equal or invalid timestamps.

## Related Topics

See [CLI Flags & Provider Auto-Detection](./cli-flags.md) for startup provider directory discovery.
See [In-Memory HTTP API Endpoints](../integration/http-api-endpoints.md) for query/response contracts used by each view.
See [Claude JSONL Provider Pattern](../patterns/claude-jsonl-provider.md) for transcript content shapes surfaced in the UI.
