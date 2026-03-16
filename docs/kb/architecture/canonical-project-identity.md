---
title: Canonical Project Identity
updated: 2026-03-16
category: Architecture
tags: [go, provider, project-identity, resolver]
related_articles:
  - docs/kb/architecture/two-phase-session-loading.md
  - docs/kb/patterns/claude-jsonl-provider.md
  - docs/kb/patterns/gemini-logs-json-provider.md
  - docs/kb/patterns/codex-sqlite-rollout-provider.md
---

# Canonical Project Identity

## Overview

EPIC-4 introduced a shared project resolver (`provider/resolver`) that maps raw provider session paths into a stable, cross-provider grouping key. Before this, each provider used its own path format as the project ID, so the same repo could appear as multiple distinct projects depending on which tool logged the session.

## Implementation

### SessionMeta fields

`provider.SessionMeta` now carries five identity fields populated at the end of Phase 1:

| Field | Description |
|---|---|
| `RawProjectPath` | Verbatim path/name from the provider before any normalization |
| `CanonicalProjectID` | Stable cross-provider grouping key |
| `CanonicalProjectSource` | Which resolution step produced the ID (see priority table) |
| `DisplayProjectName` | Human-readable project name for UI display |
| `ProjectAliases` | Other known paths or names (reserved; populated by future epics) |

### Resolution priority

`resolver.Resolve(Input)` applies four levels in order, stopping at the first that yields a non-empty value:

| Priority | Source | `CanonicalProjectSource` value |
|---|---|---|
| 1 | `DOUG_PROJECT_ID` in `AGENTS.md` managed block | `"doug"` |
| 2 | Git remote URL repo slug (HTTPS or SCP format) | `"git-remote"` |
| 3 | Normalized absolute filesystem path (lowercased) | `"normalized-path"` |
| 4 | Basename of raw path | `"basename-fallback"` |

### AGENTS.md parsing

`resolver.ParseDougMeta(agentsFilePath)` reads `DOUG_PROJECT_ID` and `DOUG_PROJECT_NAME` from inside the `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START/END -->` managed block only, ignoring all content outside that block. Duplicate keys produce a warning; the first value wins.

### Codex fix

The Codex adapter previously used the git remote repo basename as the raw `ProjectPath` grouping key, which was fragile (basename collisions across repos). After EPIC-4, Codex passes `GitOriginURL` and `CWD` to the resolver explicitly, so it follows the same four-level priority as Claude and Gemini.

## Key Decisions

- **Standalone resolver package**: `provider/resolver` is not embedded in any single adapter — all three providers call it identically.
- **Provenance always preserved**: `CanonicalProjectSource` is an enum-style string, never empty, even for fallback cases. This makes resolution debuggable without inspecting raw paths.
- **Raw path never discarded**: `RawProjectPath` stores the original before normalization so auditing or future remapping remains possible.
- **No persistence or caching**: Resolution runs at Phase 1 load time per session; no state is stored outside the in-memory session index.

## Usage Example

```go
dougMeta := resolver.ParseDougMeta(filepath.Join(projectPath, "AGENTS.md"))
res := resolver.Resolve(resolver.Input{
    DougProjectID:   dougMeta.ProjectID,
    DougProjectName: dougMeta.ProjectName,
    GitRemoteURL:    gitRemoteURL, // Codex only; empty for Claude/Gemini
    RawPath:         projectPath,
})
meta.CanonicalProjectID     = res.CanonicalProjectID
meta.CanonicalProjectSource = res.CanonicalProjectSource
meta.DisplayProjectName     = res.DisplayProjectName
```

## Edge Cases & Gotchas

- If `AGENTS.md` is absent or unreadable, `ParseDougMeta` returns an empty struct silently — resolution falls through to git remote or path.
- Normalized-path level requires the path to be absolute (`filepath.IsAbs`); relative paths fall through to basename.
- Git remote slug extraction handles both HTTPS and SCP formats and strips `.git` suffixes before extracting the basename.

## Related Topics

See [Two-Phase Session Loading Architecture](./two-phase-session-loading.md) for when resolution runs during startup.
See [Codex SQLite + Rollout Provider Pattern](../patterns/codex-sqlite-rollout-provider.md) for the Codex-specific identity fix.
