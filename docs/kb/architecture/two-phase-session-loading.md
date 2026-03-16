---
title: Two-Phase Session Loading Architecture
updated: 2026-03-16
category: Architecture
tags: [go, providers, startup, aggregation]
related_articles:
  - docs/kb/architecture/canonical-project-identity.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/gemini-logs-json-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# Two-Phase Session Loading Architecture

## Overview

doug-stats separates data loading into two phases to keep startup deterministic while avoiding unnecessary transcript parsing. Phase 1 builds a complete in-memory session and cost index before the server starts; Phase 2 parses full transcripts only when a session details request is made.

## Implementation

At startup, `main.go` detects provider directories, initializes available providers, calls `LoadSessions()` for each provider, then runs `aggregator.Aggregate()` on the merged session list.

Phase 1 source-of-truth differs by provider:
- Claude: `history.jsonl`
- Gemini: `tmp/<project>/logs.json` (fallback to `chats/` only when logs index is missing)
- Codex: `state_5.sqlite` `threads` table + referenced rollout JSONL paths

Phase 1 also resolves canonical project identity via `provider/resolver` before returning each `SessionMeta`. All canonical fields (`CanonicalProjectID`, `CanonicalProjectSource`, `DisplayProjectName`) are populated and stable before aggregation runs.

The API handler receives:
- All Phase 1 `SessionMeta` records (including canonical identity)
- Precomputed session costs from `aggregator.Summary`
- A provider map for on-demand transcript loading

Only `GET /api/sessions/:id/messages` calls provider `LoadTranscript(sessionID)`. Project, task, and session list endpoints stay in memory.

## Key Decisions

- **Server starts only after Phase 1 completes**: Prevents partially populated API responses.
- **Phase 2 only for message view**: Reduces disk I/O and keeps common list endpoints fast.
- **Provider interface shared across phases**: `LoadSessions` and `LoadTranscript` are explicit contract boundaries.

## Usage Example

```go
sessions, _ := p.LoadSessions()
summary := aggregator.Aggregate(sessions)
handler := api.New(sessions, summary, providers)
```

## Edge Cases & Gotchas

- If a provider fails `LoadSessions`, it is skipped with a warning; startup still succeeds with remaining providers.
- Unknown models still aggregate cleanly via `Cost.Unknown` propagation.

## Related Topics

See [Canonical Project Identity](./canonical-project-identity.md) for the resolver called during Phase 1 to populate cross-provider grouping keys.
See [Claude JSONL Provider Pattern](../patterns/claude-jsonl-provider.md) for JSONL history-driven ingestion.
See [Gemini logs.json Provider Pattern](../patterns/gemini-logs-json-provider.md) for project-scoped JSON transcript ingestion.
See [Codex SQLite + Rollout Provider Pattern](../patterns/codex-sqlite-rollout-provider.md) for SQLite-indexed rollout ingestion.
