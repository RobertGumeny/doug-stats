# Agent Onboarding Guide

You will be assigned a specific task. Complete it, then write your session summary.

## Where to Find Context

.doug/PRD.md — requirements, acceptance criteria, architectural decisions
docs/kb/ — patterns, infrastructure, and lessons learned for this project

## What You Can Touch

- Read source files, `.doug/PRD.md`, docs/kb/, and `.doug/ACTIVE_TASK.md`
- Modify source and test files
- Run build, test, and lint commands
- Write your session result, bug report, and failure report to the paths provided in your briefing

## Deny List

You must never:

- Run Git write/remote commands (`add`, `commit`, `push`, `pull`, `rebase`, `checkout -b`, etc.). Read-only Git context commands (`status`, `diff`, `log`, `show`) are allowed.
- Write to `project-state.yaml` or `tasks.yaml`
- Read `project-state.yaml` or `tasks.yaml`
- Create or modify any file inside `logs/`
- Read `.env` files or any file that may contain secrets
- Write any `.yaml` file
- Run `sudo`

## Available Skills

Skill files live in `.agents/skills/`. Your activation prompt specifies which skill to activate.

| Skill | Trigger | Description |
|-------|---------|-------------|
| `implement-feature` | `active_task.type: feature` | Full feature implementation: research, plan, code, test, report |
| `implement-bugfix` | `active_task.type: bugfix` | Root cause analysis, fix, regression test, report |
| `implement-documentation` | `active_task.type: documentation` | Synthesize session logs into `docs/kb/` knowledge base |

## Escalation

Check `.doug/PRD.md` → your skill file → docs/kb/ → existing code patterns before escalating.

- Unresolvable ambiguity → write failure report to path in your briefing, set `outcome: FAILURE`, stop
- Blocking bug unrelated to your task → write bug report to path in your briefing, set `outcome: BUG`, stop

Never guess on architectural or business logic decisions. Escalate instead.

## Session Summary

Your activation prompt provides the path to your pre-created session summary file. Fill it out when your task is complete — do not create a new file.

Valid outcomes: `SUCCESS` | `FAILURE` | `BUG` | `EPIC_COMPLETE`

## Platform Notes

**Windows**: The Bash tool is unavailable when running Claude Code natively on
Windows. Agents cannot run shell commands. Use WSL2 to run doug on Windows:

1. Install WSL2 and a Linux distribution (Ubuntu recommended)
2. Run all doug commands from a WSL2 terminal
3. Ensure `claude`, `git`, and your toolchain are installed inside WSL2


<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
## Doug-Specific Instructions

This section is managed by `doug init`. Keep repository-specific operating rules here, and keep task skills focused on their workflow.

### Progressive Disclosure

1. Read `.doug/ACTIVE_TASK.md` for the active task brief when it exists.
2. Read `.doug/PRD.md` for product context and constraints.
3. Read `docs/kb/README.md` for the knowledge base index.
4. Read only the KB articles relevant to the task at hand.

### Working Rules

- Treat `.doug/ACTIVE_TASK.md` as the canonical task brief for doug-managed work.
- Write your result directly into the `## Agent Result` block and summary sections at the bottom of `.doug/ACTIVE_TASK.md`.
- Do not depend on other internal doug control files. Only `.doug/ACTIVE_TASK.md` and `.doug/PRD.md` are part of the agent-facing contract.
- If you find a bug that is outside the current task scope, report it instead of fixing it opportunistically.
- Use `docs/kb/README.md` as the KB entrypoint instead of scanning the whole KB up front.
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
