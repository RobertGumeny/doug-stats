---
title: Model Pricing Registry and Cost Aggregation
updated: 2026-03-06
category: Dependency
tags: [pricing, costs, aggregation, claude, gemini, codex]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/gemini-logs-json-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# Model Pricing Registry and Cost Aggregation

## Overview

Cost computation is centralized in a pricing registry keyed by model identifier, then rolled up at session, task, and project levels by the aggregator package. Unknown models are represented explicitly so totals do not silently underreport.

## Implementation

`pricing.Registry` stores per-million-token rates and supports normalized token categories across providers:
- input
- cache creation
- cache read
- output
- thoughts/reasoning
- tool

`pricing.Compute(model, tokens)` returns `pricing.Cost{USD, Unknown}`. `Cost.Add` propagates unknown status when either side is unknown.

Provider-specific pricing behavior:
- Gemini models use `GeminiSingleCachedPerMToken` for all cached tokens because logs do not split cache write/read.
- Codex reasoning tokens default to output-rate billing unless an explicit reasoning surcharge is configured.
- Tool tokens default to input-rate billing for models that do not define a separate tool rate.

`aggregator.Aggregate(sessions)` produces:
- `Sessions`: per-session totals
- `Tasks`: per-task totals (Doug-tagged only)
- `Projects`: per-project totals (all sessions)
- `CacheTierMinutes`: metadata set to 5

## Key Decisions

- **Single registry map**: Avoids scattered pricing constants and simplifies future model additions.
- **`Unknown` flag over NaN/zero**: Preserves truth in API/UI while staying JSON-safe.
- **Unknown propagation in `Cost.Add`**: Prevents false precision in task/project totals.
- **5-minute cache tier surfaced in summary**: Keeps Claude cache-read assumptions explicit to clients.
- **Provider-specific fallback rates**: Shared compute logic handles model-specific gaps without branching in aggregator or API code.

## Usage Example

```go
cost := pricing.Compute(model, tokens)
taskTotal = taskTotal.Add(cost)
```

## Edge Cases & Gotchas

- Empty or unrecognized model IDs always return `Unknown=true`.
- Task aggregation excludes sessions without a Doug task ID.
- Cross-provider project identity differences can split what users consider one repo into multiple project keys.

## Related Topics

See [Dashboard Project and Task Views](../features/dashboard-project-and-task-views.md) for how unknown totals are rendered in the UI.
See [Gemini logs.json Provider Pattern](../patterns/gemini-logs-json-provider.md) and [Codex SQLite + Rollout Provider Pattern](../patterns/codex-sqlite-rollout-provider.md) for how provider-native token fields are normalized before pricing.
