---
title: Claude JSONL Provider Pattern
updated: 2026-03-06
category: Patterns
tags: [go, jsonl, parsing, provider]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/patterns/gemini-logs-json-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# Claude JSONL Provider Pattern

## Overview

The Claude provider is the reference JSONL implementation for ingesting AI tool logs into a normalized provider contract. It uses `history.jsonl` as the single source for session discovery and scans session JSONL files for token totals, task IDs, and session classification.

## Implementation

Phase 1:
- Read `history.jsonl`
- De-duplicate sessions by `sessionId`
- Resolve each transcript path from encoded project path
- Scan transcript lines to accumulate final assistant token usage
- Classify session as Doug, Manual, or Untagged

Phase 2:
- Parse full transcript for a single session ID
- Preserve content parts as raw JSON for UI rendering
- Return normalized message objects with optional per-message token counts

## Key Decisions

- **No filesystem walking for discovery**: `history.jsonl` controls inclusion and naturally excludes subagents.
- **Streaming deduplication**: Count only final assistant records (`stop_reason != nil`) and dedupe by message ID.
- **Raw content preservation**: `json.RawMessage` avoids losing provider-specific content shapes.
- **Malformed line tolerance**: Parse errors log warnings and skip line-level failures.

## Usage Example

```go
p := claude.New(filepath.Join(home, ".claude"))
sessions, _ := p.LoadSessions()
transcript, _ := p.LoadTranscript(sessions[0].ID)
```

## Edge Cases & Gotchas

- `LoadTranscript` expects a known session ID from Phase 1 indexing.
- If a session has user messages but no task tag, it is `manual`; no user messages becomes `untagged`.

## Related Topics

See [HTTP API Endpoints](../integration/http-api-endpoints.md) for how session class and transcript data are exposed.
See [Dashboard Navigation and Cost Views](../features/dashboard-project-and-task-views.md) for UI handling of transcript content and tool blocks.
See [Gemini logs.json Provider Pattern](./gemini-logs-json-provider.md) and [Codex SQLite + Rollout Provider Pattern](./codex-sqlite-rollout-provider.md) for non-Claude source-of-truth variants under the same provider interface.
