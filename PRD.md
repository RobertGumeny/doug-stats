# doug-stats — Product Requirements Document

> **Status:** Ready for implementation
> **Prototype:** 0 (doug orchestrator validation run)
> **Epic structure:** Three epics — Infrastructure, Core Features, Multi-Provider
> **Schema status:** All three provider log schemas documented. EPIC-3 is unblocked.

---

## What We Are Building

`doug-stats` is a companion CLI tool to `doug`. It reads coding agent session logs from disk (Claude Code, Gemini, Codex), correlates sessions with doug task IDs, and serves a local browser UI showing token usage, cost breakdowns, and conversation transcripts — grouped by task and provider.

It ships as a single Go binary with an embedded React frontend. One command, one process, browser opens automatically. No runtime dependencies, no setup.

This is Prototype 0: the primary goal is to validate the doug orchestration loop on a realistic, multi-layered project. The tool should be fully functional but is not required to be production-polished.

---

## Core Design Principles

- **Single binary.** Go backend embeds compiled Vite/React frontend via `embed.FS`. One command opens the browser.
- **Zero runtime dependencies.** All parsing, aggregation, and serving handled by the binary.
- **Read-only.** The tool never writes to log files or doug state.
- **Doug-native.** Task IDs are a first-class organizational concept, not an afterthought.
- **Graceful degradation.** Unknown models show cost as "Unknown." Sessions without a task ID are shown under "Manual." Unrecognized log formats are skipped with a warning, not a crash.

---

## Architecture

### Binary structure

```
doug-stats (Go binary)
├── cmd/             CLI entrypoint, flag parsing, browser launch
├── internal/
│   ├── scanner/     Log directory scanning per provider
│   ├── parser/      Per-provider log parsing (Claude, Gemini, Codex)
│   ├── pricing/     Model pricing registry
│   ├── aggregator/  Token + cost aggregation, task correlation
│   └── server/      Embedded HTTP server, API handlers
└── frontend/        React + TypeScript + Tailwind, compiled to dist/
    └── dist/        Embedded into binary via embed.FS at build time
```

### Build pipeline

The binary requires a two-stage build:

1. `npm run build` (or equivalent) compiles the frontend into `frontend/dist/`
2. `go build` embeds `frontend/dist/` via `//go:embed` and produces the binary

A `Makefile` (or `build.sh`) must orchestrate both steps. `go generate` may be used to trigger the frontend build. The CI workflow must replicate this sequence.

### Distribution

Cross-platform binaries are released via GitHub Releases using `goreleaser`. Targets: `darwin/arm64`, `linux/amd64`. A GitHub Actions workflow handles build and release on tag push.

### Startup model

All providers follow a consistent two-phase startup pattern:

**Phase 1 — Index and correlate (eager, at startup):**
- Read each provider's session index (SQLite `threads` for Codex, `logs.json` for Gemini, `history.jsonl` for Claude)
- For each session, partially read its session file until the first user message is found — extract task ID and stop
- Compute all aggregate data (token totals, cost, session counts) and hold in memory
- The HTTP server does not start until this phase is complete

**Phase 2 — Transcript reads (lazy, on demand):**
- Full session file parsing only happens when a user requests the transcript view for a specific session
- No full session file is ever read at startup

This means the project list, task list, and session list views are all served from memory with no disk I/O after startup. Only transcript requests touch the filesystem at runtime.

---

## CLI Interface

```
doug-stats [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--logs-dir` | `~/` | Root directory for provider log auto-detection. The binary looks for `~/.claude`, `~/.gemini`, and `~/.codex` under this root. Override to point at a non-standard home directory or test fixtures. |
| `--port` | `0` (auto) | HTTP server port. If 0 or busy, next available port is selected. URL always printed to stdout. |
| `--no-ui` | false | Accepted but prints "not yet implemented" and exits cleanly. Out of scope for Prototype 0. |

Browser launch uses the correct command per platform: `open` (macOS), `xdg-open` (Linux), `start` (Windows/WSL2).

---

## API Contract

All data is aggregated in memory at startup. The React frontend communicates exclusively through these endpoints. Response schemas are fixed — frontend and backend must not diverge.

| Endpoint | Description |
|----------|-------------|
| `GET /api/health` | Returns `{"status":"ok"}`. Used by frontend to confirm server is ready. |
| `GET /api/projects` | List of all projects with aggregate totals. Supports `?provider=claude,gemini,codex` and `?doug_only=true`. |
| `GET /api/projects/:id/tasks` | Task list for a project with per-task aggregates. Same filter params. |
| `GET /api/tasks/:id/sessions` | Session list for a task (or `manual` as a virtual task ID). Per-session metadata. |
| `GET /api/sessions/:id/messages` | Full message list for transcript view. |

All error responses use a consistent envelope: `{"error": "<message>"}`.

---

## Data Sources

### Claude Code

```
~/.claude/projects/
└── <project-slug>/
    └── <session-id>.jsonl
```

Each `.jsonl` file is one session. One JSON object per line. The `cwd` field on each record is the canonical project path — **use `cwd` to derive the project name, not the directory slug.**

`~/.claude/history.jsonl` provides a fast `sessionId → project` index and is the primary source for session discovery and project grouping at startup. The `cwd` field within session JSONL files is authoritative for project name resolution when `history.jsonl` is ambiguous.

Token fields per `assistant` message in `message.usage`:
- `input_tokens`
- `cache_creation_input_tokens`
- `cache_read_input_tokens`
- `output_tokens`

Model identifier: `message.model` on `assistant` records.

**Task ID correlation (Phase 1 — partial read):** Open each session JSONL and read lines until the first `user` record is found. Scan its prompt content for `[DOUG_TASK_ID: <value>]`, then stop reading. Do not parse the full file at startup.

**Transcript data (Phase 2 — full read):** Parse the complete JSONL file only when the transcript view is requested for that session.

**Subagent sessions** appear under a parent session UUID directory:
```
~/.claude/projects/<project-slug>/<parent-session-uuid>/subagents/agent-<id>.jsonl
```
Subagent token costs are **not attributed** to the parent session in Prototype 0. Subagent JSONL files are not scanned. See Known Gaps.

---

### Gemini

```
~/.gemini/tmp/<project-name>/
├── .project_root        # Contains absolute path to the project root
├── logs.json            # Lightweight session index
└── chats/
    └── session-<timestamp>.json   # One file per session (full detail)
```

The `<project-name>` directory component is the canonical project identifier. `.project_root` contains the absolute filesystem path and may be used to resolve the human-readable project name.

`~/.gemini/history/<project-name>/` contains archived sessions using the same structure. **Archived sessions are out of scope for Prototype 0.** Only `tmp/` is scanned. See Known Gaps.

**`chats/session-<timestamp>.json`** is a single JSON object (not JSONL). Top-level fields: `sessionId`, `projectHash`, `startTime`, `lastUpdated`, `messages`. The `messages` array contains turn objects with `id`, `timestamp`, `type` (`"user"` or `"gemini"`), and `content`.

Token data lives on Gemini turns only, in a `tokens` object:
- `input`
- `output`
- `cached` — no creation/read split; treat as cache read rate. Document this assumption in code and surface it in the UI.
- `thoughts`
- `tool`
- `total`

Model identifier: `model` string field on each Gemini turn (e.g. `"gemini-3-flash-preview"`).

**`logs.json`** is a flat JSON array with `sessionId`, `messageId`, `type`, `message`, and `timestamp`. Use as the primary session index at startup. Token data is only in the per-session chat files.

**Task ID correlation (Phase 1 — partial read):** For each session discovered via `logs.json`, open its `chats/session-<timestamp>.json` and read only the `messages` array until the first `user`-type message is found. Scan `content[0].text` for `[DOUG_TASK_ID: <value>]`, then stop. Do not process the full messages array at startup.

**Transcript data (Phase 2 — full read):** Parse the complete session JSON file only when the transcript view is requested for that session.

---

### Codex

```
~/.codex/sessions/YYYY/MM/DD/
└── rollout-<timestamp>-<thread_id>.jsonl
~/.codex/state_5.sqlite
```

`~/.codex/state_5.sqlite` is the primary session index. The `threads` table contains one row per session with `id`, `rollout_path`, `cwd`, `model_provider`, `tokens_used`, `git_branch`, `git_origin_url`, and timestamps. Use this for session discovery and project grouping at startup — do not walk the JSONL directory tree to discover sessions.

JSONL files are used for two purposes only: task ID correlation (partial read, Phase 1) and transcript data (full read, Phase 2).

**Phase 1 — Index and correlate (at startup):**
1. Query `threads` to get all sessions, their `rollout_path`, project info, and token rollups
2. For each session, open its JSONL file at `rollout_path` and read lines until the first `response_item` where `payload.role == "user"` is found. Scan `payload.content` items for `type: "input_text"` and extract `[DOUG_TASK_ID: <value>]` from the `text` field, then stop reading.

**Phase 2 — Transcript data (on demand):**
Full JSONL parse is triggered only when a transcript is requested. Relevant line types for full parse:

`session_meta` — first line of each file. Contains `payload.payload.cwd` for project name derivation and `payload.payload.model_provider`. Note the double-nested `payload.payload` path — this is correct for the JSONL format and differs from the flat `cwd` column in SQLite.

`turn_context` — one per agent turn. Contains `payload.model` (e.g. `"gpt-5.3-codex"`).

`event_msg` where `payload.type == "token_count"` — emitted after each turn. Token data in `payload.info.last_token_usage`:
- `input_tokens`
- `cached_input_tokens` — treat as cache read only (no creation equivalent)
- `output_tokens`
- `reasoning_output_tokens`
- `total_tokens`

**Do not use `total_token_usage`** — it is a running cumulative total across the session, not a per-turn value.

Token correlation: `token_count` events are not directly attached to a `turn_context` line. The parser must correlate `last_token_usage` with the preceding agent turn by sequence in the file.

Task ID correlation: scan `payload.content` items of the first `response_item` line where `payload.role == "user"`. Look for items with `type: "input_text"` and scan the `text` field for `[DOUG_TASK_ID: <value>]`.

---

## Known Gaps — Out of Scope for Prototype 0

The following are known limitations. They are explicitly deferred. Agents must not attempt to handle them.

| Gap | Detail |
|-----|--------|
| **Claude subagent costs** | Sessions under `~/.claude/projects/<slug>/<parent-uuid>/subagents/` are not scanned. Subagent token costs are not attributed to the parent session. |
| **Gemini archived sessions** | `~/.gemini/history/` is not scanned. Only `~/.gemini/tmp/` sessions are included. |

---

## Task ID Correlation

Doug injects a task ID into the agent's prompt using this exact format:

```
[DOUG_TASK_ID: EPIC-1-002]
```

`doug-stats` parses this by scanning the prompt content of the first message in a session for the pattern `[DOUG_TASK_ID: <value>]` and extracting the value.

### Session classification

| Category | Definition |
|---|---|
| **Doug session** | Contains a valid `[DOUG_TASK_ID: ...]` in the first message prompt |
| **Manual session** | No task ID found — run outside of doug |
| **Untagged** | Task ID field present but fails to parse (fallback) |

---

## Navigation Hierarchy

```
Project
├── Doug Tasks
│   └── Task (e.g. EPIC-1-002)
│       └── Session (e.g. 14d75d9b-...)
│           └── Messages (turn-by-turn transcript)
└── Manual Sessions
    └── Session
        └── Messages
```

Project-level aggregates include all session types by default. A toggle allows filtering to Doug sessions only.

---

## Pricing Registry

Pricing is defined in a structured Go map (not scattered constants). The registry maps model identifier strings to a `ModelPricing` struct containing per-token rates for each token type. New models are added by extending the map — no changes to parsing or aggregation logic required.

Cost is displayed in USD, rounded to 4 decimal places.

### Supported models

**Claude:**
- Claude Haiku 4.5
- Claude Sonnet 4.5
- Claude Sonnet 4.6
- Claude Opus 4.5
- Claude Opus 4.6

**Gemini:**
- Gemini 3.1 Pro Preview
- Gemini 3 Flash Preview
- Gemini 3 Flash-Lite Preview
- Gemini 2.5 Flash
- Gemini 2.5 Pro

**Codex:**
- GPT 5.4
- GPT 5.3 Codex
- GPT 5.2 Codex

Sessions from unrecognized model identifiers display cost as "Unknown — model not in pricing registry" rather than silently applying wrong rates or zero.

**Cache pricing notes:**
- Claude: 5-minute cache tier is assumed. Surface this assumption in the UI (tooltip or footnote).
- Gemini: `cached` tokens cover both creation and read (no distinction in logs). Treat all as read rate. Surface this assumption in the UI.
- Codex: `cached_input_tokens` is cache read only. No creation equivalent exists.

---

## Features — Must Have (Prototype 0)

### 1. Project view
- List all detected projects grouped by provider
- Show aggregate totals: total cost, total input/output/cache tokens, session count
- Toggle: "All sessions" vs "Doug tasks only"
- Provider filter: Claude / Gemini / Codex (multi-select)

### 2. Task view
- List all tasks for a project with per-task aggregates: total cost, total tokens, session count, time span
- Click through to session list for a task

### 3. Session view
- List sessions for a task or manual group
- Per-session: cost, token totals, model, timestamp, duration if derivable

### 4. Transcript view
- Turn-by-turn conversation display in chronological order
- User and assistant turns visually distinguished
- Main thread and sidechain (tool use) visually distinguished — sidechains indented or labeled
- Tool-use content blocks labeled, content collapsed by default
- Per-turn token cost shown
- Raw content rendered as plain text (no markdown rendering required)

### 5. Binary distribution
- Single binary, no runtime dependencies
- `goreleaser` config producing `darwin/arm64` and `linux/amd64` builds
- GitHub Actions workflow: build and release on tag push
- GitHub Actions workflow: build verification on PR

---

## Out of Scope for Prototype 0

The following are explicitly deferred. Do not build them:

- File export (CSV, JSON, PDF)
- Time-series or trending views
- Search within sessions
- Authentication or multi-user support
- Persistent on-disk cache (in-memory aggregation only)
- `gitBranch` surfaced in UI (may be captured in data model but not displayed)
- Incremental log scanning
- Markdown rendering in transcripts
- `--no-ui` stdout mode (flag accepted, not implemented)
- Claude subagent cost attribution (see Known Gaps)
- Gemini history/ archive scanning (see Known Gaps)

---

## Risks and Open Questions

| Risk | Severity | Mitigation |
|---|---|---|
| Gemini `cached` tokens don't distinguish creation vs read | Medium | Treat all `cached` as read rate; document in code; surface in UI |
| Codex token events require turn correlation | Medium | Match `last_token_usage` from `token_count` events to preceding agent turns by sequence; never use `total_token_usage` |
| Frontend build pipeline untested pattern | High | First task in Infra epic is a working scaffold proof — no feature work until this passes |
| Cache tier assumption for Claude pricing | Medium | Surface assumption in UI; document in code |
| Port conflicts on launch | Low | Auto-select next available port; print URL to stdout |
| Model identifier strings vary across log versions | Medium | Unknown model fallback is non-crashing; pricing registry is string-keyed and easy to extend |
| Codex `payload.payload.cwd` double-nesting | Low | Correct per JSONL format; differs from SQLite schema — comment in parser to prevent confusion |

---

## Success Criteria

At the end of Prototype 0, all of the following must be true:

- `doug-stats` compiles to a single binary with no runtime dependencies
- Running the binary opens a browser showing project → task → session → transcript navigation
- Token costs are displayed correctly for all supported Claude models
- Sessions are grouped under doug task IDs where present, "Manual" otherwise
- Conversation transcripts are readable turn-by-turn with tool use collapsed
- Binary is distributable via GitHub Releases for macOS arm64 and Linux amd64
- A GitHub Actions workflow builds and releases on tag push

---

## Prototype 0 Validation Metrics

Capture after the epic completes per the doug roadmap:

- Total token cost (per task and overall)
- Human intervention count (bugs raised, manual corrections, restarts)
- Time to completion
- Qualitative notes: where did doug struggle and why?