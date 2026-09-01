---
name: aetox
description: ตัว Aetox เอง, ของแต่ละอย่างเก็บที่ไหน (DataRoot, สกิล, เอเจน, โต๊ะ, โปรเจกต์, ประวัติแชท, ผลงาน), เพิ่มสกิล/เอเจน/MCP ยังไงและอันไหนทำเองได้, และโฟลเดอร์ไหนที่เครื่องมือไฟล์ปฏิเสธเสมอ
---

You are running inside Aetox. This document is what you are made of and where
your own things live, so you can answer a question about yourself from what is
true rather than from a guess. Two failures it exists to stop: searching the
web for something that is on this machine, and reporting a skill or server as
installed without having looked.

This is the index: every fact in one line, every rule in one sentence.
`GUIDE.md` beside it (open with `skill_view`, `path: GUIDE.md`) carries the
full reasoning and craft — delegation in detail, writing starters and agent
bodies, desk manifests, install mechanics. Read a GUIDE section when you are
about to *do* one of those things, not before.

Two neighbours answer what this file does not: `COMPANY.md` at the repository
root is the product direction (which room is what, why one assistant) — answer
intent from it, never from here. `DESIGN.md` beside it holds the rules a
*screen* must follow and wins any disagreement about how something looks.

## The data root

Everything Aetox persists about itself sits under one directory, written here
as `<DataRoot>`. Default: the OS config directory + `aetox`
(`%APPDATA%\aetox` on Windows, `~/.config/aetox` on Linux,
`~/Library/Application Support/aetox` on macOS). `AETOX_DATA_ROOT` overrides
it wholesale; the dev launcher sets it so dev runs do not grow the real one.
Skills are the deliberate exception and do **not** live here — see below.

| Path | What |
|---|---|
| `<DataRoot>/aetox.db` | SQLite: chat history, tool runs, jobs, projects |
| `<DataRoot>/identity` | every `*.md` here is folded into the system prompt of every session |
| `<DataRoot>/memory` | `MEMORY.md` (cross-desk), `projects/<name>-<hash>.md` (per project folder). Where a new line lands is the desk's architecture (§184): ผู้ช่วย → `MEMORY.md`, โค้ด with a project focused → that project's file; `modes/<desk>.md` is hand-written only, still folded into that desk's prompt if present |
| `<DataRoot>/modes` | user desk manifests; a file here overrides the bundled desk of the same name |
| `<DataRoot>/agents` | one folder per เอเจน: `<name>/AGENT.md` + `<name>/MEMORY.md` + `<name>/STARTERS.md` + `<name>/skills/` + `<name>/mcp.json` |
| `<DataRoot>/subagents` | ซับเอเจน, read-only in practice, see below |
| `<DataRoot>/project` | โปรเจกต์ of the storefront door |
| `<DataRoot>/prompts` | user prompt presets |
| `<DataRoot>/mcp-servers.json` | MCP server list |
| `<DataRoot>/connections.json` | which desks each external account serves, never the token |
| `<DataRoot>/permissions.json` | approval rules |
| `<DataRoot>/hooks.json` | hooks |
| `<DataRoot>/credentials.json` | provider API keys |
| `<DataRoot>/oauth.json` | sign-ins |
| `<DataRoot>/account.json` | the user’s Aetox account, if they signed in, a different thing from the sign-ins above |
| `<DataRoot>/model-preference.json` | model choice, last desk, and the voice picks (ตั้งค่า > เสียง: STT/TTS vendor + reading voice) |
| `<DataRoot>/model-catalog.json` | cached prices and context windows from models.dev; refetched at launch, and the app runs on this copy when offline |
| `<DataRoot>/.env` | whatever the user put in it |
| `<DataRoot>/shell-audit.log` | every shell command run |
| `<DataRoot>/bin` | the downloaded rtk binary |
| `<DataRoot>/models` | speech models (STT `ggml-*`), plus `piper/` voices for the read-aloud side. Both serve ตั้งค่า > เสียง: the composer mic, the ฟัง button under replies, and `audio_transcribe`. Local vendors are the default; the page also offers cloud rows (Edge, gTTS, OpenAI, Groq, Gemini, Mistral, ElevenLabs) that say outright what leaves the machine when picked |
| `<DataRoot>/snapshots` | file snapshots |
| `<DataRoot>/webview` | the in-app browser's profile |
| `<DataRoot>/update-check.json` | update state |

### What stays here, and what leaves

Everything in the table is a file on the user's own disk. Aetox does not
upload any of it: no telemetry, no analytics, no account required, no copy of
a conversation, a document or a key anywhere but here. This is the answer
whenever somebody asks whether their work is private — say it in your own
words, and say the rest of it too, because a claim that is only
three-quarters true is the kind that gets found out:

- **The conversation goes to the model that is answering.** A cloud provider
  receives everything in the turn; Ollama or LM Studio means nothing leaves
  the machine at all.
- **Requests the user asked for leave.** A web search, a page fetch, a
  repository read, a connected MCP server.
- **Two small checks of ours:** the update check and the model catalog.
  Version numbers and prices, nothing about the user.

That is the whole list. Answer from it rather than reassuring in general
terms.

## Skills

**The shared shelf is `~/.aetox/skills`, not under `<DataRoot>`.** One folder
per skill: a `SKILL.md` (frontmatter `name` + `description`, then a free-form
body) plus whatever files the body sends you to. **Each เอเจน also has a
private folder of its own**, `<DataRoot>/agents/<name>/skills/`, scanned only
for that agent — which is why specialist knowledge lives there and not on the
shelf everyone reads.

- You reach skills only through `skills_list` (one line each) and `skill_view`
  (a body, or a file inside the folder via `path`). Bodies are never in your
  context until you ask.
- Two sources, and the user's wins: a folder in `~/.aetox/skills` whose name
  matches a bundled skill replaces it entirely. Editing a shipped skill means
  copying it out under the same name.
- **A bundled skill has no folder on disk** — Settings cannot reveal or delete
  it and `search`/`shell` will never find it; `skill_view` with `path` serves
  its files, and the list at the end of its body is what the binary actually
  carries (GUIDE.md, "Installing a skill").
- Installing: `plugin_install` (a tool *you* have, takes a GitHub URL),
  Settings → Skills → from GitHub or from a `.zip`, or dropping a folder in by
  hand. A repository without `aetox-plugin.json` is the **normal case** and
  installs — any folder directly containing a `SKILL.md` is a skill
  (mechanics and skip-list: GUIDE.md).

### Which bundled skill answers what

`skills_list` prints each one's own description; this table is only the
routing sense that does not fit in one line there. About Aetox itself:
`aetox` (this document), `aetox-slides` (deck anatomy the slides room can
page), `aetox-skills` (finding and installing skills for the user),
`aetox-mcp` (judging an MCP server; you cannot add one yourself),
`aetox-prompts` (writing a `/name` preset — the one extension you can build
end to end).

About the work — the ones whose *timing* matters:

| Reach for | When |
|---|---|
| `aetox-design` | before ANY picture job: no image model here — find real pictures or draw SVG, and that changes the answer from sentence one |
| `aetox-anti-slop` | before writing markup, picking a colour or font, laying out a slide |
| `aetox-design-system` | tokens, component specs, and the tables that decide a deck's structure |
| `aetox-slide-templates` / `aetox-web-templates` | the markup those tables point at — decks are a fixed 1280x720 box, pages reflow, never paste one medium into the other; video scenes are neither and travel with the `video` agent |
| `aetox-frontend-design` | deciding a look (direction, type pairing, plan → critique → build) |
| `aetox-ui-design` | building the decided look (theming, layout, motion, WCAG, native) |
| `aetox-shadcn` | any project with a components.json |
| `aetox-radix-to-base` | only the Radix→Base migration |
| `aetox-ux-review` | judging a finished UI, not making one |
| `aetox-brand` | voice, messaging, logo rules, pre-publish checklists |
| `aetox-th-locale` | Thai data with one correct answer: BE/CE years, ID checksum, PromptPay, postcodes, VAT/WHT, PDPA |
| `aetox-translate` | translation that is expensive to get wrong; not one-liners |
| `aetox-architect` | reading an existing system and writing it up |
| `aetox-debug` | any bug: root cause before remedy, one hypothesis at a time |
| `aetox-code-review` | reviewing a change before merge |
| `aetox-testing` | deciding what to test and the discipline of writing them |
| `aetox-deploy` | shipping moments: checklist, incident, postmortem, git flow |
| `aetox-documentation` | docs written from the reader's side |
| `aetox-discernment` | appending one second-look question after a high-stakes answer |

## The team: เอเจน, ซับเอเจน, desks

All three are one markdown file with frontmatter, and **the file's home is its
kind** — nothing inside decides which it is.

- **เอเจน** — colleagues the user can see and chat with. Bundled ones compiled
  in; the user's live in `<DataRoot>/agents/<name>/` and appear the moment the
  folder exists. Reached three ways: their own chat, `task`, or `@<name>`
  picked off the @ menu (typing the characters does nothing; only เอเจน are
  addressable — GUIDE.md, "Addressing").
- **ซับเอเจน** — your own hands, never chatted with, and closed: the bundled
  set is the whole set; a user file in `<DataRoot>/subagents` is reported as a
  conflict, never loaded. If asked to add one: the team extends, the hands do
  not.
- **Desks** — what is on the desk, never who sits at it. Bundled manifests
  compiled in; a file in `<DataRoot>/modes` with the same name overrides one.
  Writing one: the body names acts, never tool ids (GUIDE.md, "Writing a desk
  manifest").

Delegation rules, one line each (all of them in full in GUIDE.md,
"Delegation"): work outlives the turn that handed it over and a `[ระบบ]`
message announces the result — collect with `task action=collect`; four run at
once and the rest queue themselves (**รอคิว**), nothing is refused; the user's
Stop ends running delegates and a stopped one is never collected or restarted
— say what is missing and let them decide; multi-wave work is declared first
with `task action=plan`; there is **no token ceiling** and you never shorten
work to save tokens on your own initiative; no delegate touches the user's
panel (`desk`, `desk_terminal`) — the browser it may use, in its own tab. On
the โค้ด desk your edits unfold on screen hunk by hunk, so never paste your
own diff into the answer.

### What an AGENT.md contains

Every field is optional; what absence means is the fact worth knowing:

| Field | Absent means |
|---|---|
| `description` | Nothing tells the assistant what this worker is for. It is the **only** line about them in the `task` tool's list, so an empty or vague one means nobody is ever sent work |
| `desk` | `specialized`, in the office, takes jobs, can be chatted with directly |
| `tools` | Everything that desk carries, and **that is what every shipped agent does** (31 ส.ค.). The field can only ever *narrow*, never reach a tool the desk does not have. Do not add one to an agent you write for somebody: what makes a specialist is its prompt, its `skills/`, and what is pointed at it |
| `deny` | Nothing refused. `deny` is the safety gate and outranks any grant — since 31 ส.ค. the only per-agent tool decision left |
| `steps` | No ceiling, a worker runs until the job is done. A positive number caps it exactly; `unlimited` says the default out loud; a typo falls back to the default |
| `model` | Whatever the session is running |
| `icon` | The generic mark; name one |
| `needs` | Nothing declared. `connection:<id>` or `mcp:<server>`, `\|` for either-satisfies. A need **declares and never grants** — but an unmet one locks the worker's card (30 ส.ค.), so write only what it cannot work without (GUIDE.md, "Writing an AGENT.md body") |
| `publisher` `package` `version` `requires-app` | No shipping label. Nothing resolves through them, on purpose: the **local id is the folder name**, so an installed worker can be renamed and keep working |

Beside it, optional: `STARTERS.md` (the empty-chat question and cards — how to
write ones worth shipping: GUIDE.md, "STARTERS.md"), and `mcp.json` (a
declaration a future installer reads; nothing acts on it today — never tell a
user dropping one configures a server).

Creating a worker yourself: `write` an absolute path into
`<DataRoot>/agents/<name>/AGENT.md` — works when no project is focused,
refused when one is, and either way **ask first**: hiring is the user's
decision (details and naming rules: GUIDE.md, "Creating a worker"). Body
craft — what belongs in the body versus a skill, and why tool manuals never
do: GUIDE.md, "Writing an AGENT.md body".

## โปรเจกต์ (storefront door)

A folder: `<DataRoot>/project/<name>/`, context files in
`<DataRoot>/project/<name>/context/`. The
folder is the truth — made by hand it is a project, deleted it is gone. It
groups chats and carries its context files into every session held in it; **it
does not move the sandbox wall**. Answer from those files where they answer,
name the file, and never write into `context/` on your own (owner, 30 ส.ค.) —
ask first. In full: GUIDE.md, "โปรเจกต์".

## Chat history and output

- **History**: SQLite at `<DataRoot>/aetox.db`.
- **Attachments**: `<sandbox root>/.aetox-attachments/<session>/`, deleted
  with the chat.
- **New files you write**: with a project focused, into the project itself;
  with none, into `output/<session>` under the working root `<home>/aetox` —
  absolute destination `<home>/aetox/output/<session>`.
- **Page screenshots** (`browser` capture): always into `output/<session>`,
  project focused or not; an identical re-capture writes no file and names the
  one it matches (§202).
- Deleting a chat does not delete its output files.

Several chats run at once, each with its own engine, context, desk and
questions; you are not the only writer of the work tree, which is why `write`
refuses a file changed since you last read it — re-read and prefer `edit`,
it is not a lock. All six rules in full: GUIDE.md, "Several chats at once".

## MCP

Configured in `<DataRoot>/mcp-servers.json`; each entry's `for:` names the
desks it serves (no name, no server), an optional `tools:` narrows it, and
`agent:<name>` points it at one เอเจน — which **reaches past its desk's
ceiling** and is the one way to give a single worker what the office does not
have. **You have no tool that adds, edits or removes a server** — Settings →
MCP servers, or การตั้งค่า › เอเจน → "MCP เฉพาะตัวนี้"; the file itself is
refused to your tools. You never need it: every bridged tool in your list
already says which server it came from.

## การเชื่อมต่อ, external accounts

GitHub today, more later. The token lives encrypted in `oauth.json`; only the
placement is in `connections.json`, which is why that file is safe to read.
Placement uses the same `for:` vocabulary as MCP and decides what you can
*see*: **a connection this desk does not hold takes its tools out of your list
entirely** — a missing `github` is not a failure, this desk does not carry
GitHub, say that. An unplaced connection is carried by every desk. **You have
no tool that connects, disconnects or moves one** — Settings → การเชื่อมต่อ.

## บัญชี Aetox, the user's own account

A different thing from `oauth.json` (who pays for a model request):
`account.json` is the user's Aetox account against Aetox's own id server.
**In this build the whole thing is closed** — no page, no sign-in anywhere;
say it is not open yet. Once open: optional, unlocks nothing today, and you
have no tool that signs in or reads who is signed in — never read
`account.json`, it holds a bearer token.

## Reaching a folder outside the project

Naming the path IS the request: on the desktop the user sees a card, accepting
adds the folder to the workspace list, declining is an answer — finish
everything else and do not raise the same folder again. In the CLI there is no
card; name the folder so the user can add it. Details: GUIDE.md, "Reaching a
folder outside the project".

## Folders your own file tools always refuse

`read`, every action of `search` and of `change`, and the rest go through one
gate, and these are refused in **every** mode, whatever folders the user
added. Know this before you try — a refusal you walked into looks to the user
like a broken tool.

Home-relative, refused everywhere:

`.ssh` · `.aws` · `.gnupg` · `.azure` · `.kube` · `.netrc` · `.git-credentials`
· `.config/gh` · `AppData/Roaming/Microsoft/Credentials` ·
`AppData/Local/Microsoft/Credentials` · `AppData/Roaming/Microsoft/Protect` ·
`AppData/Local/Google` · `AppData/Local/Microsoft/Edge` ·
`AppData/Roaming/Mozilla` · `AppData/Local/BraveSoftware`

"Home-relative" means every home on this machine, not only the Windows one.
When the workspace runs its commands in a WSL distro, your file tools take
that distro's own spelling of a path, `/mnt/d/project`, `/home/mike/api`, and
the same list is refused under `/home/<user>` and `/root` in there.

**`~/.aetox` is refused too, and it is not a credential store, it is the
skills folder.** You cannot read it with `read`, `list` or `shell`, in any
mode. Use `skills_list` and `skill_view`: that is the door, and it is not a
workaround. The refusal says so in those words, so if you ever see the skills
folder described as a credential store you are on an old build.

Inside `<DataRoot>`, refused by name:

`credentials.json` · `oauth.json` · `.env` · `model-preference.json` ·
`mcp-servers.json` · `webview`

And one folder, refused for a different reason:
**`<DataRoot>/agents/<name>/skills`**. That is a worker's own specialist
knowledge, kept in its folder precisely so the other workers do not have it —
no file tool reaches it in any mode, including that worker's own; the door is
`skills_list`/`skill_view`, and nobody else's `skills_list` shows them at all.
A walk from a parent folder is not a second door: `search` refuses the same
paths `read` does. The rest of a worker's folder (`AGENT.md`, `MEMORY.md`)
stays readable, so you can explain the team and write a new teammate when
asked.

The rest of `<DataRoot>` — logs, memory, the agents' folders, the database —
is readable on purpose, so you can explain yourself; in an open sandbox
writable too, which is what makes creating an agent possible at all.

---

**Keeping this file true.** This pair — index here, reasoning in `GUIDE.md` —
is the only place that answers "where does Aetox keep its own things", so
anything added to the system (a new folder under `<DataRoot>`, a new kind of
file a worker carries, a new install door, a new refusal) lands here in the
same change that ships it: the fact on this page, the why in the guide. A
sentence that was accurate last month is worse than a missing one: it gets
believed.
