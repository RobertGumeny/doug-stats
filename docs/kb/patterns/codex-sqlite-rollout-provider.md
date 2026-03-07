---
title: Codex SQLite + Rollout Provider Pattern
updated: 2026-03-06
category: Patterns
tags: [go, codex, sqlite, jsonl, provider]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/dependencies/model-pricing-and-aggregation.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/gemini-logs-json-provider.md
---

# Codex SQLite + Rollout Provider Pattern

## Overview

The Codex provider uses `.codex/state_5.sqlite` as the only discovery index and parses rollout JSONL files referenced by the `threads` table. This avoids directory walks and keeps session inclusion aligned with Codex's authoritative thread metadata.

## Implementation

Phase 1:
- Query `threads` in `state_5.sqlite` for `id`, `rollout_path`, `git_origin_url`, and `cwd`.
- Resolve each rollout path directly from `rollout_path`.
- Derive project identity from `git_origin_url` repo name, falling back to `cwd`, then transcript `session_meta` cwd when needed.
- Scan rollout JSONL lines to extract task IDs, assistant model, and token usage from `event_msg` `token_count.info.last_token_usage`.
- Aggregate normalized tokens (`input`, `cached_input`, `output`, `reasoning`) into shared token counts.

Phase 2:
- Parse rollout JSONL on demand for transcript requests.
- Track current model from `turn_context` events.
- Correlate token_count events to the immediately preceding assistant turn.
- Preserve provider-native message content as normalized `ContentPart` data.

## Key Decisions

- **SQLite threads as sole source of truth**: Prevents drift from stale or orphaned rollout files.
- **Python sqlite bridge**: Uses standard-library `sqlite3` subprocess access where Go SQLite module installation is constrained.
- **`last_token_usage` only**: Avoids cumulative overcounting from aggregate token fields.
- **Warning-based resilience**: Malformed rollout lines are skipped without dropping entire sessions.

## Usage Example

```go
p := codex.New(filepath.Join(home, ".codex"))
sessions, _ := p.LoadSessions()
transcript, _ := p.LoadTranscript(sessions[0].ID)
```

## Edge Cases & Gotchas

- `LoadTranscript` requires prior `LoadSessions` indexing for session-to-rollout mapping.
- Missing or schema-shifted SQLite columns in `threads` fail discovery early.
- Project naming can differ from Claude/Gemini path-style project IDs.

## Related Topics

See [In-Memory HTTP API Endpoints](../integration/http-api-endpoints.md) for provider-filtered query behavior.
See [Model Pricing Registry and Cost Aggregation](../dependencies/model-pricing-and-aggregation.md) for Codex reasoning-token pricing behavior.
