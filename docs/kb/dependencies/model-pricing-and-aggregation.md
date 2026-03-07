---
title: Model Pricing Registry and Cost Aggregation
updated: 2026-03-06
category: Dependency
tags: [pricing, costs, aggregation, claude]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/integration/http-api-endpoints.md
  - docs/kb/features/dashboard-project-and-task-views.md
---

# Model Pricing Registry and Cost Aggregation

## Overview

Cost computation is centralized in a pricing registry keyed by model identifier, then rolled up at session, task, and project levels by the aggregator package. Unknown models are represented explicitly so totals do not silently underreport.

## Implementation

`pricing.Registry` stores per-million-token rates and supports the four token categories used by Claude:
- input
- cache creation
- cache read
- output

`pricing.Compute(model, tokens)` returns `pricing.Cost{USD, Unknown}`. `Cost.Add` propagates unknown status when either side is unknown.

`aggregator.Aggregate(sessions)` produces:
- `Sessions`: per-session totals
- `Tasks`: per-task totals (Doug-tagged only)
- `Projects`: per-project totals (all sessions)
- `CacheTierMinutes`: metadata set to 5

## Key Decisions

- **Single registry map**: Avoids scattered pricing constants and simplifies future model additions.
- **`Unknown` flag over NaN/zero**: Preserves truth in API/UI while staying JSON-safe.
- **Unknown propagation in `Cost.Add`**: Prevents false precision in task/project totals.
- **5-minute cache tier surfaced in summary**: Keeps the pricing assumption explicit to clients.

## Usage Example

```go
cost := pricing.Compute(model, tokens)
taskTotal = taskTotal.Add(cost)
```

## Edge Cases & Gotchas

- Empty or unrecognized model IDs always return `Unknown=true`.
- Task aggregation excludes sessions without a Doug task ID.

## Related Topics

See [Dashboard Project and Task Views](../features/dashboard-project-and-task-views.md) for how unknown totals are rendered in the UI.
