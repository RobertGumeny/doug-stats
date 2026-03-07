---
title: Two-Stage Build Pipeline
updated: 2026-03-06
category: Infrastructure
tags: [go, react, vite, embed, makefile]
related_articles:
  - docs/kb/infrastructure/ci-cd.md
  - docs/kb/features/cli-flags.md
---

# Two-Stage Build Pipeline

## Overview

doug-stats uses a two-stage build: npm builds the React frontend into `frontend/dist/`, then `go build` embeds that directory into the binary via `embed.FS`. The result is a single self-contained binary that serves the UI without external assets.

## Implementation

**Stage 1 — Frontend:**
```
npm install && npm run build   # outputs to frontend/dist/
```
Vite + React + TypeScript + Tailwind CSS. Entry: `frontend/src/main.tsx`.

**Stage 2 — Go binary:**
```go
//go:embed all:frontend/dist
var embedFS embed.FS

sub, _ := fs.Sub(embedFS, "frontend/dist")
http.FileServer(http.FS(sub))
```
`fs.Sub` strips the `frontend/dist` path prefix so `/index.html` is served at the root.

**Makefile targets:** `all` (default), `build`, `frontend-build`, `test`, `clean`.

## Key Decisions

- **`all:frontend/dist` prefix on embed directive**: Includes all files, even dotfiles, under `frontend/dist/`.
- **Committed placeholder `frontend/dist/index.html`**: Allows `go build` and `go test ./...` to succeed on a clean clone without running npm. `make build` overwrites it with real Vite output.
- **`fs.Sub` for path stripping**: Avoids serving files under a `/frontend/dist/` prefix; the HTTP server sees a clean root.

## Edge Cases & Gotchas

- The placeholder `frontend/dist/index.html` must remain committed and not gitignored; forgetting this breaks `go test ./...` on CI without npm.
- Running `go build` directly (without `make build`) will embed the placeholder, not the real frontend.

## Related Topics

See [CI/CD](./ci-cd.md) for how this two-stage build is replicated in GitHub Actions and goreleaser.
