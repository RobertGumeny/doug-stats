---
title: In-Memory HTTP API Endpoints
updated: 2026-03-06
category: Integration
tags: [http, api, filters, sessions]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# In-Memory HTTP API Endpoints

## Overview

The API layer serves project, task, and session data from in-memory startup aggregates, with one on-demand transcript endpoint for message detail. A shared filter model (`provider`, `doug_only`) applies consistently across endpoints.

## Implementation

Endpoints:
- `GET /api/health`
- `GET /api/projects`
- `GET /api/tasks?project=<path>`
- `GET /api/sessions?task=<id>[&project=<path>]`
- `GET /api/sessions/:id/messages`

Behavior highlights:
- `task=manual` is a virtual task that matches manual and untagged sessions.
- `/api/tasks` appends a virtual `"manual"` entry when non-Doug sessions exist in the selected project (unless `doug_only=true`).
- Errors use a stable envelope: `{"error":"..."}`.

Only `/api/sessions/:id/messages` triggers Phase 2 transcript parsing through provider `LoadTranscript`.

## Key Decisions

- **Dedicated `api` package**: Keeps HTTP concerns separate from startup orchestration.
- **Pure `filterSessions` helper**: Centralized filter logic across handlers and tests.
- **Virtual manual task ID**: Preserves non-Doug usage visibility without requiring source task tags.
- **Prefix route for message endpoint**: `/api/sessions/` captures `:id/messages` without external router deps.

## Usage Example

```bash
curl '/api/tasks?project=/home/user/repo&provider=claude&doug_only=true'
```

## Edge Cases & Gotchas

- Missing `project` on `/api/tasks` and missing `task` on `/api/sessions` return `400`.
- Unknown provider name in session metadata during message lookup returns `500`.

## Related Topics

See [Two-Phase Session Loading Architecture](../architecture/two-phase-session-loading.md) for startup sequencing guarantees.
