# doug-stats

`doug-stats` is a local dashboard for AI coding assistant usage and cost across Claude Code, Gemini CLI, and Codex CLI logs.

## Supported Providers

The binary auto-detects these provider directories under a log root:

- `~/.claude`
- `~/.gemini`
- `~/.codex`

If one or more provider directories are missing, they are skipped with a warning. If none are found, startup exits with a helpful error.

## Log Directory Defaults and `--logs-dir`

By default, `doug-stats` scans your home directory (`~/`) for the provider subdirectories above.

You can override the scan root with `--logs-dir`:

```bash
doug-stats --logs-dir /path/to/log-root
```

Examples:

```bash
# Use defaults (~/.claude, ~/.gemini, ~/.codex)
doug-stats

# Point at fixture data or a nonstandard root
doug-stats --logs-dir /tmp/doug-fixtures
```

Related flags:

- `--port <n>`: request a port; if busy, the app falls back to an available port.
- `--no-ui`: accepted but currently not implemented (prints `not yet implemented` and exits).

## Known Limitations

The following are known gaps in the current implementation:

1. `--no-ui` is a stub and does not run the server without opening UI.
2. Project identity is provider-specific (`Claude: project path`, `Gemini: .project_root or tmp dir name`, `Codex: git origin repo name or cwd`), so the same repo may appear as separate projects when metadata differs.
