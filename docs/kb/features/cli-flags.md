---
title: CLI Flags & Provider Auto-Detection
updated: 2026-03-06
category: Features
tags: [go, cli, flags, providers]
related_articles:
  - docs/kb/infrastructure/build-pipeline.md
---

# CLI Flags & Provider Auto-Detection

## Overview

The binary accepts three flags (`--logs-dir`, `--port`, `--no-ui`) and auto-detects provider log directories (`.claude`, `.gemini`, `.codex`) under the configured root. Missing providers emit warnings but do not crash; if none are found the binary exits with a helpful message.

## Implementation

**Flags:**

| Flag | Default | Behavior |
|------|---------|----------|
| `--logs-dir` | `~/` (via `os/user.Current()` + `HOME` fallback) | Root directory for provider subdirectory detection |
| `--port` | 0 (auto) | Bind port; falls back to auto-select if busy |
| `--no-ui` | false | Prints "not yet implemented", exits 0 |

**Key functions in `main.go`:**

```go
// Auto-selects a free port using OS assignment
func findAvailablePort() (int, error) {
    l, _ := net.Listen("tcp", "localhost:0")
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port, nil
}

// Tries requested port; falls back to auto if busy
func resolvePort(requested int) int { ... }

// Returns provider dirs that exist under root
func detectProviderDirs(root string) []string { ... }
```

**Provider subdirectories** are defined in a package-level slice (`providerSubdirs`) so tests can access them without hardcoding strings:
```go
var providerSubdirs = []string{".claude", ".gemini", ".codex"}
```

## Key Decisions

- **`net.Listen("tcp", "localhost:0")` for port auto-selection**: OS assigns a free port; avoids race conditions vs. manual port scanning.
- **`providerSubdirs` as package-level var**: Enables test injection without flag parsing overhead.
- **Warn-and-skip for missing providers**: Missing subdirs log a warning; the binary only exits if *all* providers are absent.
- **`--no-ui` exits 0**: Spec requires graceful exit, not an error, since the flag is a known stub.

## Edge Cases & Gotchas

- If `--port` is specified but busy, a warning is logged and the next available port is used; the selected port is always printed to stdout before the browser opens.
- `os/user.Current()` can fail in minimal environments (e.g., some containers); `os.Getenv("HOME")` is the fallback.

## Related Topics

See [Build Pipeline](../infrastructure/build-pipeline.md) for how the binary is built and embedded.
