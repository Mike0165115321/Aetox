<p align="center">
  <img src="docs/assets/logo.png" alt="Aetox" width="110">
</p>

<h1 align="center">Aetox</h1>

<p align="center">
  <strong>A Windows desktop app that finishes the work on your machine — files, browser, shell, documents.</strong>
</p>

<p align="center">
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/Mike0165115321/Aetox?color=2f81f7"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
  <img alt="Tests" src="https://img.shields.io/badge/tests-1%2C797%20Go%20%2B%20755%20UI-brightgreen">
  <img alt="Platform" src="https://img.shields.io/badge/platform-Windows%2010%2B-lightgrey">
</p>

<p align="center">
  <a href="README.th.md">ภาษาไทย</a> ·
  <a href="https://mike0165115321.github.io/Aetox/">Website</a> ·
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe">Download</a> ·
  <a href="ARCHITECTURE.md">Architecture</a> ·
  <a href="docs/DECISIONS.md">Every decision, and why</a>
</p>

<p align="center">
  <img src="docs/assets/hero-app.png" alt="Aetox desktop" width="90%">
</p>

---

## What it is

Aetox is a desktop application for Windows that runs an AI agent against your own machine.
You describe what needs doing; it reads and writes real files, runs real commands in a real
shell, and drives a real browser you can watch.

It is one self-contained 47.5 MB executable. There is no runtime to install alongside it, no
`node_modules`, no bundled copy of Chromium. It talks to whichever model you point it at —
a hosted API, a subscription you already pay for, or a 9B running in LM Studio or Ollama on
your own GPU — and the capability comes from the app rather than from the model's parameters.
That is why a small local model can still read a picture, transcribe a recording, and produce
a spreadsheet: `image_ocr`, `audio_transcribe` and `sheet_write` are the app's, not the model's.

Two concrete jobs, to make that less abstract. *"Go through this folder of receipts and give me
one spreadsheet"* — it OCRs each image, works out the totals in a JavaScript interpreter
compiled into the binary and shows you the script beside the answer, then writes a real `.xlsx`
with live formulas. *"Find why the login test is flaky"* — it greps the repo, runs the suite in
a terminal tab you can watch, reads the failure, and edits the file.

**The interface ships in Thai and English, and Thai is the default.** Every string exists in
both; the language switch is in Settings and in the first-run wizard. This README is in English;
[ภาษาไทย is here](README.th.md).

## Install

Windows 10 or later, x64. **You do not need an API key to start** — a built-in `aetox` provider
ships five test models that exercise the real machinery (real tool calls, a real delegation to a
sub-agent, a long reasoning stream), so you can see what the app does before signing up for
anything.

**Installer** — [aetox-amd64-installer.exe](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe) (20.9 MB)

It also fetches and SHA256-verifies the outside programs Aetox uses: WebView2, Tesseract (with
Thai), poppler, ffmpeg, and a starter whisper model for offline speech. Any one of them failing
skips that step with an explanation rather than aborting the install.

**Scoop**

```powershell
scoop install https://raw.githubusercontent.com/Mike0165115321/Aetox/main/scoop/aetox.json
```

**Portable** — [the zip](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-windows-amd64-portable.zip),
unpack, run `aetox.exe`. This is the only channel that can update itself in place.

### If SmartScreen or your antivirus complains

The installer is not code-signed yet, so the first run shows "Windows protected your PC" with an
unknown publisher — **More info → Run anyway**. Releases *are* signed: an ed25519 public key is
compiled into the binary and the updater verifies the signature over `checksums.txt` before it
trusts a single hash. An empty or wrong key refuses the update rather than falling back.

### Linux and macOS

Not shipped. The engine and the desktop package both compile and their suites run under `-race`
on Linux and macOS in CI; the browser pane is stubbed and packaging is not done.

**1.0.0 is the Windows release.** Until 2026-08-15 this line read *"1.0.0 ships all three or it is
not 1.0.0"* — that criterion was **changed by the owner, not met**. Holding a stable Windows build
behind a browser pane and an at-rest keystore that do not exist yet on the other two helps nobody
already running it. Linux and macOS ship under the same bar, in a later release. See
[PLATFORM-SUPPORT.md](PLATFORM-SUPPORT.md) for where the port actually stands, and
[DECISIONS §109](docs/DECISIONS.md) for why the bar moved.

<details>
<summary>Build it yourself</summary>

```powershell
cd desktop
wails build          # → desktop/build/bin/aetox.exe
wails build -nsis    # with the installer
```

</details>

## What you can do with it

**Hand over a folder and get a file back.** Point it at a directory of images, PDFs or
recordings and ask for the thing you actually want. OCR (Thai and English), PDF text with the
layout intact, and offline speech-to-text all feed the same conversation; `sheet_write`,
`doc_write` and `slides_write` hand back a real Office file, built from Go's standard-library
zip and XML with no third-party dependency — live formulas, dates that sort, Thai vowels in the
right place.

**Watch it work in a real browser.** Not a headless scrape: a WebView2 window composited into
the app, with an address bar, back/forward, DevTools, and eight device presets that resize the
native window and zoom the page so CSS media queries genuinely fire. The agent opens, reads,
clicks by reference and types into it while you watch the same tab.

<img src="docs/assets/cap-browser.png" alt="The agent driving a page in the workbench browser" width="100%">

**Read what the model cannot see.** `image_ocr` runs Tesseract with Thai and English, so a
screenshot, a scan or a photographed form becomes text a 9B model can reason about — no vision
model required, and the model that *can* see gets the image itself instead.

<img src="docs/assets/cap-image-ocr.png" alt="OCR pulling Thai text out of an image" width="100%">

**Point it at a repository.** File tree, Monaco editor, unlimited real PTY terminal tabs, `git`,
`grep` and `glob` over the whole tree, plus `diagnostics` and `symbol` backed by language servers
the app installs on first use (gopls, typescript-language-server, svelteserver).

**Delegate to a specialist.** Type `@doc`, `@sheet`, `@deck`, `@github`, `@automation` or
`@research` and your sentence reaches that agent word for word — not a paraphrase. Each is a folder on disk with its
own prompt, its own memory, optionally its own model, and its own private skills.

**Give it a job, not a step.** Work that takes twenty moves is planned before it is worked, and
`todo_write` puts that plan on screen while it runs, so what you watch is the order it chose
rather than a spinner. Up to four specialists run at once: `task` hands work out and returns
immediately, `task_result` collects it, so three jobs cost the time of the slowest rather than
the sum. One that reaches a decision it should not make alone comes back as a *question* instead
of a guess. One still working when your answer arrives keeps working — you collect it by the same
id in a later turn, so the end of a turn is not a deadline. And before any of it runs there is a
planning stance that can read anything and change nothing, on an allow-list, so a tool added next
month is held back by default rather than slipping in.

Run on 2026-08-15, from one sentence — *"find 20 CRMs a Thai SME could actually pick and give me
a spreadsheet comparing them"*: **6m 51s, two agents, 42 tool calls between them** — 8 by the
assistant, 27 by `research` reading pricing pages, 7 by `sheet` — and one tool failure it worked
around. The handoff between the two was the baton this README describes: `research` left a
markdown report in the session's output folder and `sheet` was given the path, not the contents.
Twenty rows came back, fourteen with a real numeric price sorted low to high, and
**six deliberately left blank** with the reason written in beside them — quoted-only pricing, or
a page that would not state a figure. Every row carries the date the page was read and the link
the number came from, and says whether the page was opened in the browser or only searched. The
blanks are the part worth trusting: a table with no gaps in it is a table that guessed.

**Ask it about your own past work.** Every conversation and every tool run lives in local SQLite
with FTS5, so `session_search` across months of history is a query, not an inference — zero
tokens, Thai and English alike.

**Have it build automations in n8n or Windmill.** Connect an instance you host and the automation
agent lists, reads, creates and updates workflows in it, and can start the server for you from a
command you saved. Read [the honest limit](#automation-what-it-can-and-cannot-do) before you rely
on this.

## Two doors, one app

One switch on the wordmark moves between **Assistant** ("Use, remember, and create") and **Code**
("Build, debug, and ship"). It is the same binary, the same data directory, the same settings,
memory and permissions — switching doors is not switching apps, and the app remembers which one
you were in. The door also scopes the chat list in SQL, so a run of coding sessions cannot starve
the other list.

|  | Assistant | Code |
|:---|:---|:---|
| **Where it works** | Your whole machine when no project is focused, or a project folder plus folders you add | The project folder you opened, plus folders you add |
| **Rooms** | Assistant · Projects · Agent team · Automation · Work | Code |
| **The right-hand panel** | Available | Available |

The doors separate what the *system* carries, never what the AI is willing to do. The assistant
has files and a shell and does software work with them; it does not hand a request back because
it involves code.

## The team

Six agents ship — `deck`, `doc`, `sheet`, `github`, `automation`, `research` — and hiring a
seventh is dropping a folder into `<DataRoot>/agents/`. No release, no plugin API, no restart.

An agent's folder is its whole identity: `AGENT.md` (who it is, what desk it sits at, which tools
it may narrow itself to, which model it pins), `MEMORY.md` (what it has learned), `STARTERS.md`
(how an empty chat with it opens, per language), and a private `skills/` folder no other agent
can see.

That folder is also where the difference between a clever assistant and a company sits. Each
agent pins **its own model**, so the one that opens twenty pricing pages can run on something
cheap while the one that has to weigh what it found runs on something strong, and the bill
follows the work instead of following the hardest task in it. Each keeps **its own memory**, so
what the research agent learned about a source does not leak into the deck agent's judgement
about slides. A single generalist has one model, one memory and one set of tools for every job
it will ever be handed — and no way for you to add a seventh colleague to it.

You can **delegate** to one — the assistant calls `task` and up to four run concurrently — or you
can **talk to one directly**, in a session bound to its tools, its memory and its prompt. `@name`
from the composer is the third door: your sentence arrives verbatim, mention included, because a
paraphrase is where the request goes wrong.

Agents never call each other. The star has one centre; multi-step work is a conveyor through the
assistant, and the baton is a file path rather than the content. Separately, three **sub-agents**
(`explore`, `plan`, `general`) are internal helpers — a fixed set, not extensible, deliberately.

## What it learns, and what you approve

Aetox remembers across sessions, and **nothing is written without you approving it.**

The `memory` tool cannot write. It queues a proposal. Separately, a summarizer reads the tool-run
log with no model call at all, clusters repeated failures by agent + tool + normalised error, and
proposes a lesson once the same mistake has happened three times.

Everything lands in one review queue in Settings, and each card shows the body, the agent's stated
reason, whose memory it would go into, and — for a replacement — the line it would overwrite.
Approve or discard. What is kept is plain markdown you can open, edit line by line, or forget in
place, and every decision is recorded permanently, so *"why does it think that?"* always has an
answer. One switch turns the whole thing off.

It takes effect from the next session, not this one — a mid-conversation prompt change would
invalidate the provider's prefix cache, which is the same reason the tool block never moves.

The other half is **standing instructions**: your own always-on markdown files that ride into
every desk, every project and every agent. What you wrote, and what it worked out, are kept apart
on purpose.

## How it works, and what it can reach

A **desk** is the tool ceiling of a session. Three ship — `assistant`, `coding`, `specialized` —
and a session's desk is fixed for its life. Desks are also what MCP servers and external
connections are placed on, which is how a tool installed for one kind of work stays out of the
others.

**Where it may go.** With a project focused, the workspace is that folder plus any folder you add
— added folders get read and write with no prompt, the same rights as the root, because a second
quieter tier would be a rule you never agreed to. With no project focused, the workspace is the
machine, and writes land under `output/<session>`. One function resolves every path, symlinks and
all, and there is deliberately no second check anywhere else.

**Approval.** Three levels, and one gate every tool call goes through — built-in tools, shell and
MCP alike.

| Level | What it asks about |
|:---|:---|
| **Ask** | Anything that is not a plain read inside the workspace |
| **Unsafe only** | Deletes, `git`, shell, and anything touching a path outside the workspace |
| **Full access** | Nothing. There is no carve-out. |

MCP tools confirm at every level regardless, because their behaviour is defined by somebody else's
server.

**Shell commands are path-contained**, not pattern-matched: a path hidden in a quoted argument,
behind a flag, behind a redirect, or behind `%VAR%` / `$VAR` / `~` is still resolved and checked,
and a command the scanner cannot read — `$(...)`, backticks, `-EncodedCommand`, `FromBase64String`
— is refused rather than guessed at. Every command run is appended to a 0600 audit log.

**Refused to every file tool, in every mode:** `.ssh` `.aws` `.gnupg` `.azure` `.kube` `.netrc`
`.git-credentials` `.config/gh` `.aetox`, the Windows Credentials and Protect stores, Chrome /
Edge / Firefox / Brave profiles, and Aetox's own `credentials.json`, `oauth.json`,
`mcp-servers.json` and browser profile. Folder-picking refuses them too, so it fails at the door
rather than as a confusing tool error later.

**Your data.**

|  | Where it stands |
|:---|:---|
| Chat history, tool runs, produced files | On your disk, in local SQLite and plain folders |
| Cutting the cloud off entirely | Run the model through LM Studio or Ollama — not one byte of the prompt leaves |
| API keys | Their own file, 0600, DPAPI-wrapped against your Windows account. Off Windows there is no encryption at rest — that is stated rather than implied |
| Secrets in logs | Stripped through one registry into all three sinks: debug log, shell audit log, and the buffer the bug-report form reads |
| MCP secrets | `${env:VAR}` indirection, so a key never lands in the settings file |
| Taking it with you | Export any chat to `.md` or `.json`, and import a `.json` back into any Aetox |
| Bug reports | The app transmits nothing. It prefills a GitHub issue, already scrubbed, and you read every line before sending it from your own account |

There is no server of ours in the middle and no analytics. Using a cloud provider means that
provider sees what its API normally sees, and nothing is routed through us.

## Everything it can do

A tool count is not a reason to use anything, which is why this is down here.

**40 tools reach the model on a fresh install**; a default assistant session carries fewer,
because a desk narrows the set. They cost about 9,844 tokens on every request before you have
typed anything, against a ceiling of 10,100 that a test enforces — eight slots of headroom, argued
for rather than spent.

| Group | Tools |
|:---|:---|
| **Files** | `read` `write` `edit` `apply_patch` `delete` `list` `glob` `grep` |
| **Running commands** | `shell` *(run · output · kill · list)* `git` `desk_terminal` |
| **Handing back files** | `doc_write` `sheet_write` `slides_write` |
| **Reading media** | `image_ocr` `video_ocr` `pdf_read` `audio_transcribe` |
| **Web and automation** | `browser` *(open · read · click · type)* `web_fetch` `web_search` |
| **Code work** | `diagnostics` `symbol` `github` *(repo_summary · search · read_file · list_files)* |
| **How the assistant works** | `task` `task_result` `task_answer` `ask_user` `todo_write` `suggest_task` `memory` `session_search` `calc` `time` `desk_list` `desk_open` `skills_list` `skill_view` `plugin_install` |

That table is generated from the registry the model is actually handed
(`go test ./desktop -run TestPrintReadmeToolTable -v`), because a hand-kept list of what a program
contains is a second source of truth for a question the program can answer — and this one drifted
for months, still naming tools that had been folded into `shell` and `github`.

Connecting an automation engine adds one more packed tool — `n8n` *(list · read · create · update ·
activate)* or `windmill` *(workspaces · list · read · create · update)* — and nothing until then:
a tool with no account behind it is withheld rather than shown and refused.

**Growth goes where it costs nothing.** A skill is a markdown document, not a tool: `skills_list`
returns one line each and `skill_view` returns one body, so installing three hundred leaves the
tool block exactly the same size. MCP servers are placed per desk and per agent, so a server added
for video work is absent from an ordinary conversation — not hidden from the model, absent. Office
writers reach only the specialized desk, so the assistant delegates for a `.pptx` rather than
carrying three tools it rarely needs.

**11 providers in the app** — OpenAI · Anthropic · Gemini · DeepSeek · Qwen · Z.ai · OpenRouter ·
Codex · LM Studio · Ollama · and the built-in `aetox`. OpenRouter and Codex sign in; the rest take
an API key or a local server address. The engine catalogue holds 18, and the remainder are not
drawn in the window.

Local models are treated as first-class: Aetox asks LM Studio and Ollama which model is *loaded*
rather than which exist, streams the answer and the reasoning, really calls tools, and counts
tokens into the same statistics. You can switch provider or model mid-conversation and the full
context follows — tool calls, tool results and compaction summaries, not just the visible text.
One provider is active at a time; Aetox never silently reroutes your turn to a different paid one.

### Automation: what it can and cannot do

Connect an n8n or Windmill instance you host and the automation agent can list, read, create,
update, and — n8n only — activate workflows, and start your server from a command you saved.

**It cannot run a workflow and see the result.** There is no execution API call anywhere in this
codebase; the closest thing is the agent clicking Execute in the vendor's own editor through the
browser tool, which is not a verified run. Windmill has no activate either, so a flow it creates
is saved and inert until you trigger it yourself. The agent says so out loud rather than implying
otherwise, and a test exists whose only job is to keep it saying so.

**There is no scheduler, and there will not be one.** Aetox has no cloud, so a schedule would
silently depend on your laptop never closing. n8n and Windmill are the clock; Aetox is the hands.

## Measured, not claimed

The rules are in [BENCHMARK.md](BENCHMARK.md), and its one standing rule is that a number which
has not passed them may not appear here or on the website.

> The dangerous number is the flattering one, because nobody audits a figure that makes them look
> good.

**Aetox.** The two size rows and the two test counts were re-measured 2026-08-18 on v1.2.4;
assembling a turn is from 2026-08-13, and the ⁽ᵈ⁾ rows from 2026-07-27 on v0.9.2.

| | |
|:---|---:|
| What you download | 20.9 MB installer |
| What ends up on disk | **47.5 MB**, one file |
| Assembling a turn | 0.32 ms · 174.9 KB allocated |
| Go tests | 1,797 across 38 packages, 0 failures |
| Frontend tests | 755 across 69 files, 0 failures |
| First launch (cold) | 1.77 s ⁽ᵈ⁾ |
| Every launch after | 0.53 s ⁽ᵈ⁾ |
| RAM committed | 252 MB ⁽ᵈ⁾ |
| Processes | 7 ⁽ᵈ⁾ |

⁽ᵈ⁾ Measured 2026-07-27 on v0.9.2 under the rules, and **not re-measured since**. They are dated
figures rather than current ones, and they are here rather than deleted because they did pass the
rules on the day — which is the whole difference between an old number and a bad one.

Two things that number honestly. Assembling a turn was 0.12 ms and 96.2 KB when the block held 27
tools; it is 0.32 ms and 174.9 KB now that it holds more. That is a real regression, and it is
still three ten-thousandths of a second — the time you wait is the model thinking. And the Go
suite is green on Windows; **CI on Linux and macOS is red**, six failures, one of them a genuine
sandbox hole rather than a bad test. Since 2026-08-15 those two jobs are reported rather than
gating — Windows is what ships, and one shared verdict meant every Windows push went red for a
port's unfinished edges until nobody read the colour at all. The failures are still on the run
page and still to be fixed; what changed is that they no longer hide the platform that is done.

**Against Zed**, the harder ruler — native Rust, with a reputation for being light.

| | Aetox | Zed |
|:---|:---|:---|
| First launch (cold) | 1.77 s | 2.12 s |
| Every launch after | 0.53 s | 0.53 s |
| RAM committed | 252 MB | 471 MB |
| Disk | **47.5 MB** | 419 MB |

Both columns except Aetox's disk figure were measured 2026-07-27 on the same machine under the same
rules, and neither has been re-measured — Zed is no longer installed here. A tie on warm launch with
a native Rust editor is the result worth having; treat the row as dated rather than current.

The rest of this category ships 240 MB to 1 GB because an Electron app brings its own copy of
Chromium. Aetox uses the WebView2 that Windows already has — and being straight about it, WebView2
*is* Chromium, so the memory it holds is not a win over Electron. The win is that you are not
handed a second browser to store.

<details>
<summary>How these were measured, and what does not qualify</summary>

**Disk** — download [the portable zip](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-windows-amd64-portable.zip),
unpack it, and read the size of the one `aetox.exe` inside: 49,768,960 bytes. Anyone can reproduce
it in a minute. It replaces the 41.5 MB figure measured on 2026-08-13, which was correct then and
is not now. Competitor sizes are measured after install from the install folder, never
taken from a download page, and never from a folder holding user profiles or caches.

**Launch, RAM and process count** — `bench.ps1 -Start`, empty project, median of 5 runs after
discarding the first, read after 60 seconds settled. A true cold launch needs a reboot first,
because Windows keeps the app's files in its file cache afterwards.

**Assembling a turn** — `bench.ps1 -Engine`, median of 3 rounds.

**What was removed from this section.** An earlier version of this README published "97% of input
tokens came from cache over six consecutive messages" and local first-token times of 1.42 s and
1.75 s. Neither has a source in this repository — no test, no log, no BENCHMARK entry — and the
machine those local numbers describe did not have LM Studio installed. They are gone rather than
date-stamped, because the rule above does not have an exception for numbers we would like to keep.

</details>

## Status — v1.2.4

The core is in place. [Release notes](docs/release-notes/v1.2.4.md) ·
[roadmap](ROADMAP.md) · [architecture](ARCHITECTURE.md).

Three things it does today that are worth knowing about:

- **Undo, and answer variants.** Every turn records the files it changed, so "Undo (3)" puts them
  back, and regenerating an answer tells you exactly which files it will restore first. Old
  answers are kept — arrows and "2 / 3" — and switching between them rewrites what the
  conversation continues from.
- **The model draws.** An SVG in an answer renders as a picture inside the reply, building itself
  shape by shape as it streams, with Copy and Save that bake the live theme's colours onto a clone
  and rasterise at 2×.
- **You can talk into a running turn.** Typing turns Stop back into Send and hands your sentence to
  the turn in flight; anything it could not fold in comes back as a queued bubble you can drop.

**Next** — agents working across turns rather than only inside one; a plan handed from the
assistant door to the code door; a code-door team with defined roles.

## Documentation

[Architecture](ARCHITECTURE.md) · [Every decision, and why](docs/DECISIONS.md) ·
[What this company is](COMPANY.md) · [How a screen is designed](DESIGN.md) ·
[Benchmark rules](BENCHMARK.md) ·
[Platform support](PLATFORM-SUPPORT.md) · [Roadmap](ROADMAP.md) ·
[Automation engines](docs/AUTOMATION-ENGINES.md)

## Reporting bugs

[Open an issue](https://github.com/Mike0165115321/Aetox/issues). The app has a door for this:
Settings prefills a GitHub issue with your version, install channel and OS, folds the recent
internal log into a `<details>` block with secrets already stripped, and hands it to you to read
before you send it from your own account. Nothing is transmitted by the app.

## Who makes this, and the licence

Aetox is written by one person. It exists because a model that can only produce text is half a
tool, and the missing half — hands, permission, and a place to put the result — is an application
problem rather than a model problem.

Code is under [Apache-2.0](LICENSE): use, modify, redistribute and sell it freely, including in
closed-source work. Two things asked in return — keep the [NOTICE](NOTICE) file and the copyright
notice, and say which files you changed.

The name **"Aetox"** and the logo are trademarks and are **not** covered by the licence
(Apache-2.0 clause 6). Fork freely; anything you redistribute carries your own name, so nobody is
confused about which one is the original. Version 0.7.1 and earlier were MIT, and those versions
stay MIT forever.

> Aetox was not born to compete with anyone. It exists to stand where the market has a gap — not
> to be one more agent framework, and not to lock anyone into anything.

📧 [phrmsawanachyphl@gmail.com](mailto:phrmsawanachyphl@gmail.com) ·
❤️ [Support the project](SPONSOR.md)

---

<p align="center">
  © 2026 Chayaphon Phromsawana · <a href="LICENSE">Apache-2.0</a> · "Aetox" is a trademark, not covered by the licence
</p>
