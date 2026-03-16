---
title: CI/CD — GitHub Actions & goreleaser
updated: 2026-03-06
category: Infrastructure
tags: [github-actions, goreleaser, ci, release]
related_articles:
  - docs/kb/infrastructure/build-pipeline.md
---

# CI/CD — GitHub Actions & goreleaser

## Overview

Two GitHub Actions workflows cover the full delivery pipeline: CI runs on every PR (build + test), and Release runs on `v*` tag pushes to publish cross-platform binaries via goreleaser.

## Implementation

**CI (`.github/workflows/ci.yml`)** — triggered on `pull_request`:
1. Install Node 20, run `npm ci` + `npm run build` in `frontend/`
2. Set up Go, run `go build ./...` and `go test ./...`

**Release (`.github/workflows/release.yml`)** — triggered on `v*` tags:
- Uses `goreleaser/goreleaser-action@v6` with `GITHUB_TOKEN`
- `fetch-depth: 0` required for goreleaser changelog generation

**goreleaser (`.goreleaser.yaml`)** — v2 config:
```yaml
before:
  hooks:
    - npm ci --prefix frontend
    - npm run build --prefix frontend
builds:
  - goos: [darwin, linux]
    goarch: [arm64, amd64]
    ignore:
      - goos: darwin
        goarch: amd64
      - goos: linux
        goarch: arm64
```

## Key Decisions

- **goreleaser `before.hooks` for npm**: Satisfies the Go embed requirement before `go build` runs; mirrors the local Makefile approach.
- **darwin/arm64 + linux/amd64 only**: Other combinations excluded via the `ignore` list to keep release artifacts minimal.
- **`fetch-depth: 0`**: Required by goreleaser to read git history for changelog generation; without it the release fails.

## Related Topics

See [Build Pipeline](./build-pipeline.md) for the two-stage embed approach that CI and goreleaser replicate.
