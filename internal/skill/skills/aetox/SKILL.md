---
name: aetox
description: ตัว Aetox เอง — ของแต่ละอย่างเก็บที่ไหน (DataRoot, สกิล, เอเจน, โต๊ะ, โปรเจกต์, ประวัติแชท, ผลงาน), เพิ่มสกิล/เอเจน/MCP ยังไงและอันไหนทำเองได้, และโฟลเดอร์ไหนที่เครื่องมือไฟล์ปฏิเสธเสมอ
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
| `<DataRoot>/agents` | one folder per เอเจน: `<name>/AGENT.md` + `<name>/MEMORY.md` + `<name>/STARTERS.md` + `<name>/skills/` |
| `<DataRoot>/subagents` | ซับเอเจน — read-only in practice, see below |
| `<DataRoot>/project` | โปรเจกต์ of the storefront door |
| `<DataRoot>/prompts` | user prompt presets |
| `<DataRoot>/mcp-servers.json` | MCP server list |
| `<DataRoot>/connections.json` | which desks each external account serves — never the token |
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

**The shared shelf is `~/.aetox/skills`, not under `<DataRoot>`.** One folder
per skill, each containing a `SKILL.md` (frontmatter `name` + `description`,
then a free-form body) plus whatever files that body sends you to.

It is not the only place scanned. **Each เอเจน has a private skills folder of
its own** — `<DataRoot>/agents/<name>/skills/` — in the identical layout, and it
is scanned only for the agent whose folder it is. That is the difference that
matters: a skill on the shared shelf is everyone's, so specialist knowledge
(the required fields of a tax invoice, the columns of a payroll workbook) is
kept in the worker's own folder instead. See "เอเจน" below.

Two sources on the shelf, and the second wins:

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

## เอเจน, ซับเอเจน, and desks

All three are one markdown file with frontmatter. **The file's home is its
kind** — nothing inside the file decides which it is.

- **เอเจน (agents)** — the team the user can see and chat with directly.
  Bundled ones are compiled in; the user's live in `<DataRoot>/agents/<name>/`,
  as a folder (`AGENT.md` is the definition, `MEMORY.md` is what it learned,
  `skills/` is what it knows, `STARTERS.md` is how it opens a chat). Hiring one
  is dropping one more folder — no release needed.
  A user reaches one of them three ways: opening its own chat, letting the
  assistant hand it work with `task`, or writing **`@<name>`** in an ordinary
  message. The last one delivers that single message to the worker word for
  word — no paraphrase in between — and leaves the conversation where it is. If
  that worker stops to ask something back, the user's next message answers it.
  The name is whatever the roster says, so an agent the user added themselves is
  addressable the moment its folder exists.
- **ซับเอเจน (helpers)** — your own hands, never chatted with, and part of
  the system: the bundled set is the whole set. A user file in
  `<DataRoot>/subagents` is **not loaded** — it is reported as a conflict so it
  never vanishes silently — and the save door refuses. If a user asks to add
  one, say the team is what extends, not the hands.
- **Desks (modes)** — what is on the desk, never who is sitting at it. Bundled
  manifests are compiled in; a file in `<DataRoot>/modes` with the same name
  overrides one, and a new file is a new desk.

### What one contains

Every field of an `AGENT.md` is optional, and each has a default worth knowing
before you tell a user what they must write:

| Field | Absent means |
|---|---|
| `description` | Nothing tells the assistant what this worker is for. It is the **only** line about them in the `task` tool's list, so an empty or vague one means nobody is ever sent work |
| `desk` | `specialized` — in the office, takes jobs, can be chatted with directly |
| `tools` | Everything that desk carries. The field can only ever *narrow* — a worker cannot list its way to a tool the desk does not have |
| `deny` | Nothing refused. `deny` is the safety gate; `tools` is only a token saving |
| `steps` | 24. `unlimited` removes the ceiling; a typo falls back to 24 rather than to no ceiling |
| `model` | Whatever the session is running |
| `icon` | Derived from what the worker produces |
| `needs` | Nothing declared. Entries are `connection:<id>` or `mcp:<server>`, and `\|` between two of them means "either one satisfies this". A need **declares and never grants**: the grant is `for:` on the connection or the server. What it buys is that an agent missing one says so — in its own prompt, and on its page in การตั้งค่า › เอเจน |

Beside `AGENT.md`, a worker may keep a `STARTERS.md` — the question at the top
of an empty chat with it, and the cards under it. Markdown that happens to
parse: the heading is the question, each list item is one card, split on `|`
into title, the sentence that lands in the composer, and optionally an icon
name. A prompt ending in `:` is the deliberate half-sentence the user finishes.
`STARTERS.en.md` beside it is the English version. All of it is optional — a
worker without one opens with the four cards the app draws for any colleague —
but writing one is what makes a hired worker feel like the shipped ones, so
offer it whenever you write an `AGENT.md`.

The user also has a form for it: การตั้งค่า › เอเจน → that agent → "ประโยคเปิด
ของเอเจนคนนี้", a headline and four rows. It writes this same file, so a file
you wrote by hand opens in it and a card they typed there is a line you can
edit — there is one opening, not an app copy and a file copy.

```markdown
# จะให้ปิดบัญชีอะไรดี?

- กระทบยอดธนาคาร | ช่วยกระทบยอดธนาคารเดือนนี้: | chartColumn
```

The name is the **folder's** name, never a field inside the file. It may not
contain spaces or `\ / : * ? " < > |`, and it may not collide with a
ซับเอเจน's name — memory and job history key on the bare name, so one name
belongs to one worker. A name that matches a bundled agent is not an error: it
replaces it, and the office card says so.

### Creating one

There is no dedicated tool for it. What there is, is the ordinary `write`, and
whether it reaches depends on the mode:

- **No project focused** — the sandbox is open, so `write` with an **absolute**
  path into `<DataRoot>/agents/<name>/AGENT.md` succeeds, and the worker appears
  with no restart: the profile resolver reads the disk on every lookup.
- **A project focused** — the wall is up and the same write is refused with
  "path is outside the folders this session can use".

So the accurate answer to "can you add an agent yourself" is *yes, when no
project is focused* — not "I have no way to". But **ask first**. Hiring is the
user's decision, the folder is theirs, and a teammate that appeared because you
inferred one was wanted is a change to their team that nobody chose. Offer to
write it, say where it will go, and let them answer.

`<DataRoot>/modes` behaves the same way for desks. `<DataRoot>/subagents` does
not: a file written there is **not loaded** whatever mode you are in, so writing
one produces a conflict report rather than a helper.

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
server does not get it. An entry may also carry a `tools:` list, and when it
does, those are the only tools taken from that server; without one, all of them
are. It is for the server that is two products in one box (§97.3).

A `for:` entry may also be `agent:<name>`, which points the server at one เอเจน
instead of a desk. That is not a narrower version of the same thing: a server
pointed at an agent **skips that agent's `tools:` allow-list and reaches past
its desk's ceiling**. It is the one way to give a single worker something the
rest of the office does not have.

**You have no tool that adds, edits or removes an MCP server.** There is a
binding the Settings page calls, and nothing in your tool list reaches it. When
a user asks you to add a server, tell them: Settings → MCP servers — or, for
one agent, การตั้งค่า › เอเจน → that agent → กล่อง "MCP เฉพาะตัวนี้", which
writes the same `for:` list from the other end. Do not offer to edit the file
for them — it is also refused to your file tools (below).

You do not need the file to answer questions about MCP: every tool bridged from
a server is already in your tool list and says which server it came from.

## การเชื่อมต่อ — external accounts

An account the user attached so you can work on their behalf with it: GitHub
today, more later. Two halves in two places, on purpose — the token is in
`oauth.json` with the sign-ins, encrypted; only the placement is in
`connections.json`, which is why that file is safe to read and copy.

Placement uses the same `for:` vocabulary as MCP — a desk name, or
`agent:<name>` — and it decides what you can see, not what you may do:
**a connection this desk does not hold takes its tools out of your tool list
entirely.** So if you are looking for `github` and it is not there, the answer
is not that it failed; it is that this desk does not carry GitHub. Say that,
rather than reporting the tool as broken.

**You have no tool that connects, disconnects or moves an account** — same rule
as MCP above. It is Settings → การเชื่อมต่อ, and the user does it.

A connection that has never been placed is carried by every desk. Nothing was
taken away from anyone by this file arriving.

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

And one folder, refused for a different reason: **`<DataRoot>/agents/<name>/skills`**.
That is a worker's own specialist knowledge, and it sits in that worker's folder
precisely so the other workers do not have it — so no file tool reaches it, in
any mode, including that worker's own. Its skills are handed to it as tools
instead, which is the same shape as `~/.aetox/skills` being reachable through
`skills_list` and `skill_view` but not through `read`. Knowledge travels through
the skill door or not at all.

The rest of a worker's folder — `AGENT.md`, `MEMORY.md` — stays readable, so you
can still explain the team and still write a new `AGENT.md` when the user asks
for a teammate.

The rest of `<DataRoot>` — logs, memory, the agents' folders, the database — is
readable on purpose, so you can explain yourself. Readable, and in an open
sandbox writable too: that is what makes creating an agent possible at all, and
why the paragraph above asks you to check with the user rather than the gate.

---

**Keeping this file true.** It is the only place that answers "where does
Aetox keep its own things", so anything added to the system — a new folder under
`<DataRoot>`, a new kind of file a worker can carry, a new door for installing
something, a new refusal — belongs here in the same change that ships it. A
sentence here that was accurate last month is worse than a missing one: it gets
believed.
