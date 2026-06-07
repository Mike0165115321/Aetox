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
- `shell <command>` (high-risk commands require confirmation)
- `write <path> <content>` (high-risk; create/overwrite files under sandbox root)
- `git status|log|branch|diff|show`
- `fs pwd|ls|find|cat`

## Approval policy (v1)

- **Read-only allowed by default**
  - `git status`, `git log`, `git branch`, `git diff`, `git show`
  - `fs pwd`, `fs ls`, `fs find`, `fs cat`
  - shell commands that do not match known risk patterns
- **High-risk requires confirmation**
  - `shell` commands that may modify/delete
  - `write` (create/overwrite files)
  - unsupported `git` commands
  - `fs` commands outside read-only list
- If confirmation is denied: `executed (blocked)`
- Detailed contract:
  - [docs/response-contract-and-approvals.md](docs/response-contract-and-approvals.md)

## UX

- Thinking indicator now shows for conversation and skill:
  - Conversation: `กำลังคิด...`
  - Skill: `กำลังรัน...`
- Indicator is removed once command execution ends and before final response.

## Usage

```powershell
aetox
aetox chat "สรุปโปรเจกต์นี้"
aetox help
aetox version
aetox --no-banner
```

Run one-shot:

```powershell
echo "ช่วยสรุปสั้นๆ หน่อย" | aetox
```

Switch model:

```powershell
aetox /model
```

## Behavior notes

- `RunOnce` and skill commands return final summary when tool path is used.
- Fallback summary is used when tool summarization fails (still shows output and status).
- Output is sanitized/restricted in tool summarization to avoid leaking obvious secrets (`token`, `api key`, `password` patterns).
