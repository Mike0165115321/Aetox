---
name: aetox
description: ตัว Aetox เอง — ของแต่ละอย่างเก็บที่ไหน (DataRoot, สกิล, ตัวแทน, โต๊ะ, โปรเจกต์, ประวัติแชท, ผลงาน), เพิ่มสกิล/MCP ยังไง, และโฟลเดอร์ไหนที่เครื่องมือไฟล์ปฏิเสธเสมอ
---

You are running inside Aetox. This document is what you are made of and where
your own things live, so you can answer a question about yourself from what is
true rather than from a guess.

Read it before answering anything about Aetox's own storage, configuration or
extension points. Two failures it exists to stop: searching the web for
something that is on this machine, and reporting a skill or server as installed
without having looked.

Everything below is the *practical* answer — where a thing is, and how to add
one. The product direction it serves (which room is what, why there is one
assistant and not five) is `COMPANY.md` at the repository root. Read that
instead when the question is about intent; do not answer intent from this file.

## The data root

Everything Aetox persists about itself sits under one directory. Written here
as `<DataRoot>`.

- Default: the OS config directory + `aetox` — `%APPDATA%\aetox` on Windows,
  `~/.config/aetox` on Linux, `~/Library/Application Support/aetox` on macOS.
- `AETOX_DATA_ROOT` overrides it, wholesale. Set it and every path below moves
  with it. The dev launcher sets it so repeated dev runs do not grow the real
  one.

Skills are the deliberate exception and do **not** live here — see below.

| Path | What |
|---|---|
| `<DataRoot>/aetox.db` | SQLite: chat history, tool runs, jobs, projects |
| `<DataRoot>/identity` | every `*.md` here is folded into the system prompt of every session |
| `<DataRoot>/memory` | `MEMORY.md` (cross-desk) and `modes/<desk>.md` (per desk) |
| `<DataRoot>/modes` | user desk manifests; a file here overrides the bundled desk of the same name |
| `<DataRoot>/agents` | one folder per ตัวแทน: `<name>/AGENT.md` + `<name>/MEMORY.md` |
| `<DataRoot>/subagents` | ผู้ช่วยตัวแทน — read-only in practice, see below |
| `<DataRoot>/project` | โปรเจกต์ of the storefront door |
| `<DataRoot>/prompts` | user prompt presets |
| `<DataRoot>/mcp-servers.json` | MCP server list |
| `<DataRoot>/permissions.json` | approval rules |
| `<DataRoot>/hooks.json` | hooks |
| `<DataRoot>/credentials.json` | provider API keys |
| `<DataRoot>/oauth.json` | sign-ins |
| `<DataRoot>/model-preference.json` | model choice, last desk |
| `<DataRoot>/.env` | whatever the user put in it |
| `<DataRoot>/shell-audit.log` | every shell command run |
| `<DataRoot>/bin` | the downloaded rtk binary |
| `<DataRoot>/models` | speech models |
| `<DataRoot>/snapshots` | file snapshots |
| `<DataRoot>/webview` | the in-app browser's profile |
| `<DataRoot>/update-check.json` | update state |

## Skills

**Skills live at `~/.aetox/skills`, not under `<DataRoot>`.** One folder per
skill, each containing a `SKILL.md` (frontmatter `name` + `description`, then a
free-form body) plus whatever files that body sends you to. That directory is
the only place scanned.

Two sources, and the second wins:

- **Bundled** — compiled into the binary (this document is one). Nothing to
  download, nothing on disk to delete.
- **User** — a folder in `~/.aetox/skills`. A folder whose skill name matches a
  bundled one replaces it entirely. Editing a shipped skill means copying it
  out under the same name, never fighting the app.

You reach skills only through `skills_list` (one line per skill) and
`skill_view` (one skill's body, or one file inside its folder via `path`). Their
bodies are never in your context until you ask — which is why a skill is the
right home for knowledge like this, and a prompt layer is not.

**A bundled skill has no folder**, so `skill_view` with a `path` will refuse it,
and it cannot be deleted from Settings.

### Installing one

Four roads in, all landing in `~/.aetox/skills`:

1. `plugin_install` — a tool *you* have. Takes a GitHub repository URL.
2. Settings → Skills → install from GitHub. Same code path, run by the user.
3. Settings → Skills → install from a `.zip`.
4. The user drops a folder into `~/.aetox/skills` by hand (Settings has a
   button that opens it).

`plugin_install` accepts both shapes of repository, and the plain one is the
normal case:

- **No manifest** (almost every published skill) — the repository tree is read
  and *any folder that directly contains a `SKILL.md`* is a skill, at any
  depth. So `skills/foo/SKILL.md` installs as `foo`. A `SKILL.md` at the
  repository root wins outright: the repository itself is the skill, named
  after the repo. Skipped: dot-directories, and `node_modules`, `vendor`,
  `dist`, `build`, `target`, `out`, `coverage`, `__pycache__` — a `SKILL.md`
  under one of those belongs to somebody else's package. Every file beside the
  `SKILL.md` comes with it.
- **`aetox-plugin.json`** at the repository root — an explicit manifest
  (`type: skill-bundle`, a name, and a `files` list of source→target pairs) for
  a repository that wants to say exactly what it ships.

Never tell a user a repository is unsupported because it lacks
`aetox-plugin.json`. That is the ordinary case and it installs.

## ตัวแทน, ผู้ช่วยตัวแทน, and desks

All three are one markdown file with frontmatter. **The file's home is its
kind** — nothing inside the file decides which it is.

- **ตัวแทน (agents)** — the team the user can see and chat with directly.
  Bundled ones are compiled in; the user's live in `<DataRoot>/agents/<name>/`,
  as a folder (`AGENT.md` is the definition, `MEMORY.md` is what it learned).
  Hiring one is dropping one more folder — no release needed.
- **ผู้ช่วยตัวแทน (helpers)** — your own hands, never chatted with, and part of
  the system: the bundled set is the whole set. A user file in
  `<DataRoot>/subagents` is **not loaded** — it is reported as a conflict so it
  never vanishes silently — and the save door refuses. If a user asks to add
  one, say the team is what extends, not the hands.
- **Desks (modes)** — what is on the desk, never who is sitting at it. Bundled
  manifests are compiled in; a file in `<DataRoot>/modes` with the same name
  overrides one, and a new file is a new desk.

You have no tool that writes any of these three. Creating or editing them is
the user's job from Settings, or by hand in the folders above.

## โปรเจกต์ (storefront door)

A project is a folder: `<DataRoot>/project/<name>/`, with its context files in
`<DataRoot>/project/<name>/context/`. The folder is the truth — there is no
table of projects to fall out of step with the disk, so a folder made by hand
is a project and a folder deleted is a deleted project.

It groups chats and carries those context files into every session held in it.
**It does not move the sandbox wall** — you still reach the machine exactly as
you otherwise would. That is what separates it from the workshop door's
project, which roots the sandbox in a folder on disk and is a fence.

## Chat history and output

- **History** — SQLite at `<DataRoot>/aetox.db`.
- **Attachments** — copied into `<sandbox root>/.aetox-attachments/<session>/`,
  one folder per chat, deleted with the chat.
- **New files you write** — with a project focused, into the project itself.
  With none focused, into `output/<session>` under the working root, which is
  `<home>/aetox`. So the absolute destination unfocused is
  `<home>/aetox/output/<session>`.
- Deleting a chat does not delete its output files. They are ordinary files
  with their own life on disk.

## MCP

Servers are configured in `<DataRoot>/mcp-servers.json`, and each entry carries
a `for:` list naming the desks it is attached to — a desk that does not name a
server does not get it.

**You have no tool that adds, edits or removes an MCP server.** There is a
binding the Settings page calls, and nothing in your tool list reaches it. When
a user asks you to add a server, tell them: Settings → MCP servers. Do not
offer to edit the file for them — it is also refused to your file tools (below).

You do not need the file to answer questions about MCP: every tool bridged from
a server is already in your tool list and says which server it came from.

## Folders your own file tools always refuse

`read`, `write`, `list`, `grep`, `glob` and the rest go through one gate, and
these are refused in **every** mode, whatever folders the user added. Know this
before you try — a refusal you walked into looks to the user like a broken tool.

Home-relative, refused everywhere:

`.ssh` · `.aws` · `.gnupg` · `.azure` · `.kube` · `.netrc` · `.git-credentials`
· `.config/gh` · `.aetox` · `AppData/Roaming/Microsoft/Credentials` ·
`AppData/Local/Microsoft/Credentials` · `AppData/Roaming/Microsoft/Protect` ·
`AppData/Local/Google` · `AppData/Local/Microsoft/Edge` ·
`AppData/Roaming/Mozilla` · `AppData/Local/BraveSoftware`

**`.aetox` is on that list, and that is the skills folder.** You cannot read
`~/.aetox/skills` with `read` or `list`. Use `skills_list` and `skill_view` —
that is the door, and it is not a workaround.

Inside `<DataRoot>`, refused by name:

`credentials.json` · `oauth.json` · `.env` · `model-preference.json` ·
`mcp-servers.json` · `webview`

The rest of `<DataRoot>` — logs, memory, the agents' folders, the database — is
readable on purpose, so you can explain yourself.
