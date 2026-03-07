# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Add pricing registry and cost aggregation layer: ModelPricing struct with five Claude models, Cost.Add accumulation, and session/task/project aggregates computed before HTTP server start
- Add Claude Code log scanner and parser as the reference Provider implementation with two-phase loading (session index at startup, transcripts on demand)

- Scaffolded `doug-stats` with a two-stage build pipeline: Vite + React + TypeScript + Tailwind frontend embedded in a Go binary via `embed.FS`.
- Added runtime server behavior for auto-selected port binding and automatic browser launch on startup.
- Added CLI flags `--logs-dir`, `--port`, and `--no-ui` with provider directory auto-detection for `.claude`, `.gemini`, and `.codex`.
- Added GitHub Actions CI workflow for frontend build plus Go build/test on pull requests.
- Added GitHub Actions release workflow and `.goreleaser.yaml` for tagged binary publishing (darwin/arm64 and linux/amd64).
- Added KB articles documenting build pipeline, CI/CD setup, and CLI/provider-detection behavior.

### Changed

### Fixed

### Removed
