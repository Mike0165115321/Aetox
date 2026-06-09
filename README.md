# Aetox CLI (Terminal AI Chat)

Aetox CLI runs two modes in the same session:

- **Conversation mode** (chat): respond as normal conversation in streaming.
- **Skill mode**: execute command first, then return one final summary message.

## Flow summary

- `/<command>` or known skill keyword runs skill mode.
- Free text runs conversation mode.
- Skill execution is treated as:
  - `intent := command`
  - execute tool
  - normalize output metadata
  - summarize result
  - print final summary

Status reported to user:

- `executed (done)`
- `executed (error)`
- `executed (blocked)`

## Current supported skills

- `help`
- `time`
- `echo <ข้อความ>`
- `list [path]` (sandboxed path listing)
- `read <path>` (read file content)
- `write <path> <content>` (high-risk; create/overwrite files under sandbox root)
- `delete <path>` (high-risk; remove files under sandbox root)
- `git status|log|branch|diff|show|fetch|add|commit|...` (mutating git requires confirmation)
- `fs pwd|ls|find|cat` (read-only fs)
- `shell <command>` (high-risk; audited)
- `github_repo_summary <url>` (read-only network)
- `plugin_install <url>` (high-risk; external write)

## Approval policy (v3)

Three approval modes control risk before execution:

| Mode | Behavior |
|---|---|
| `ask` | Prompt before every command with side-effects (default) |
| `unsafe-only` | Prompt only for delete, mutate-git, execute-shell, touch-outside-workspace |
| `full-access` | Run everything without confirmation |

All execution paths — explicit skill, inferred tool, and model-selected tool — pass through `internal/safety.AssessCommand` before running.

## Shell Audit Log

Every shell execution via `/shell <command>` is audited:

- **Location:** `~/.aetox/shell-audit.log` (JSONL, append-only)
- **Fields:** `time`, `command`, `workdir`, `success`, `duration_ms`, `error`
- Audit failure does not block shell execution
- Shell is not exposed to the model's tool selection — only explicit `/shell` path triggers it

## UX

- Thinking indicator shows for conversation and skill:
  - Conversation: `กำลังคิด...`
  - Skill: `กำลังรัน...`
- Indicator is removed once command execution ends and before final response.
- Thinking level displayed as `provider/model(level)` in header — canonical `off` level disables thinking.
- Model status line reflects normalized thinking level used at runtime.

## Usage

```powershell
aetox
aetox chat "สรุปโปรเจกต์นี้"
aetox help
aetox version
aetox --no-banner
```

Switch model or approval mode:

```powershell
aetox /model
aetox --approval unsafe-only
```

## Behavior notes

- `RunOnce` and skill commands return final summary when tool path is used.
- Fallback summary is used when tool summarization fails (still shows output and status).
- Output is sanitized in tool summarization to avoid leaking secrets (`token`, `api key`, `password` patterns).
- Shell execution is audited to `~/.aetox/shell-audit.log` (non-blocking on failure).
- Model can select tools via native tool calling (`time`, `list`, `read`, `write`, `delete`, `github_repo_summary`, `plugin_install`).
- Inferred tool path detects natural-language intent (e.g. "create file" → write, "list directory" → list).
