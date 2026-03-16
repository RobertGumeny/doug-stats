---
title: Gemini logs.json Provider Pattern
updated: 2026-03-16
category: Patterns
tags: [go, gemini, json, provider, tokens]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/architecture/canonical-project-identity.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
---

# Gemini logs.json Provider Pattern

## Overview

The Gemini provider indexes sessions from project-scoped `logs.json` files under `~/.gemini/tmp/<project>/`, then parses `chats/session-*.json` on demand for transcripts. This keeps discovery deterministic while preserving Gemini-specific content types like thoughts and tool call results.

## Implementation

Phase 1:
- Iterate `~/.gemini/tmp/*` project directories.
- Resolve project identity from `.project_root`, else fallback to the tmp directory name.
- Use `logs.json` as the session index; only fall back to `chats/*.json` scanning when `logs.json` is absent.
- For each indexed session, parse the chat file to aggregate `input`, `output`, `cached`, `thoughts`, and `tool` tokens.
- Classify sessions as Doug/Manual/Untagged by scanning user content for `[DOUG_TASK_ID: ...]`.
- Call `resolver.ParseDougMeta` + `resolver.Resolve` to populate canonical project identity fields on `SessionMeta`.

Phase 2:
- Parse the full session chat JSON for `GET /api/sessions/:id/messages`.
- Normalize Gemini assistant/user messages into provider `ContentPart` items (`text`, `thinking`, `tool_use`, `tool_result`).
- Attach per-message token counts when available.

## Key Decisions

- **logs.json-first discovery**: Avoids orphaned sessions and mirrors Gemini CLI's own session index.
- **Project root override via `.project_root`**: Preserves absolute repo paths when available.
- **Schema-tolerant parsing**: Unknown message types are skipped with warnings rather than aborting ingestion.
- **Token category mapping**: Gemini-specific categories map into shared token fields (`CacheRead`, `Thoughts`, `Tool`) for unified pricing.

## Usage Example

```go
p := gemini.New(filepath.Join(home, ".gemini"))
sessions, _ := p.LoadSessions()
transcript, _ := p.LoadTranscript(sessions[0].ID)
```

## Edge Cases & Gotchas

- Malformed `logs.json` marks the index as present but can yield zero discoverable sessions.
- Missing session files referenced by `logs.json` are warned and skipped.
- Gemini exposes only one cached-token bucket, so cost uses a single cached read rate.

## Related Topics

See [Canonical Project Identity](../architecture/canonical-project-identity.md) for the resolver that populates `CanonicalProjectID` in Phase 1.
See [Two-Phase Session Loading Architecture](../architecture/two-phase-session-loading.md) for startup/on-demand boundaries.
See [Model Pricing Registry and Cost Aggregation](../dependencies/model-pricing-and-aggregation.md) for Gemini cached/thought/tool cost handling.
