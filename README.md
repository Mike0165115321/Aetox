<p align="center">
  <img src="docs/assets/logo.png" alt="Aetox" width="110">
</p>

<h1 align="center">Aetox</h1>

<p align="center">
  <strong>A Windows desktop app that finishes the work on your machine — files, browser, shell, documents.</strong>
</p>

<p align="center">
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/Mike0165115321/Aetox?color=2f81f7"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-proprietary%20%C2%B7%20source%20available-blue"></a>
  <img alt="Tests" src="https://img.shields.io/badge/tests-2%2C295%20Go%20%2B%20953%20UI-brightgreen">
  <img alt="Platform" src="https://img.shields.io/badge/platform-Windows%2010%2B-lightgrey">
</p>

<p align="center">
  <a href="README.th.md">ภาษาไทย</a> ·
  <a href="https://mike0165115321.github.io/Aetox/">Website</a> ·
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe">Download</a> ·
  <a href="https://www.facebook.com/share/g/1BnXC5EiWg/">Community</a> ·
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

It is one self-contained 48.5 MB executable. There is no runtime to install alongside it, no
`node_modules`, no bundled copy of Chromium. It talks to whichever model you point it at —
a hosted API, a subscription you already pay for, or a 9B/35B running in LM Studio or Ollama on
your own GPU (your data never leaves your machine or country — hook it up to Ollama and not a single byte goes anywhere) — and the capability comes from the app rather than from the model's parameters.
That is why a small local model can still read a picture, transcribe a recording, and hand you a
deck that opens in PowerPoint: `image_ocr`, `audio_transcribe` and the slide exporter are the
app's, not the model's.

Two concrete jobs, to make that less abstract. *"Go through this folder of receipts and give me
one spreadsheet"* — it OCRs each image, works out the totals in a JavaScript interpreter
compiled into the binary and shows you the script beside the answer, then writes a real `.xlsx`
with live formulas. *"Find why the login test is flaky"* — it greps the repo, runs the suite in
a terminal tab you can watch, reads the failure, and edits the file.

**The interface ships in Thai and English, and Thai is the default.** Every string exists in
both; the language switch is in Settings and in the first-run wizard. This README is in English;
[ภาษาไทย is here](README.th.md).

## Highlights

- **Four rooms in the same window as the conversation** — slides, browser, files and terminal. The
  agent works in the room you are looking at, and you can reach in at any point. The Code door adds
  a fifth: Git.
- **It builds slide decks** — one self-contained `.html` file that is yours, editable by hand, and
  openable on any machine with a browser. Exports as `.pdf`, `.png` or `.jpg`.
- **The browser control layer is ours** — the window is WebView2; the layer that drives it we wrote.
  A model that cannot see images clicks the right control, with no guessing at coordinates.
- **It builds websites and systems, not just code in a chat box** — a file tree, a Monaco editor,
  unlimited real PTY terminal tabs, `git`, `grep` and `glob` over the whole tree, and language
  servers the app installs itself.
- **Capability comes from the app, not from model parameters** — Thai/English OCR and offline
  speech-to-text are tools the app drives, so a 9B/35B model on your own GPU does these jobs as
  well as a frontier one.
- **22 model providers** — cloud (OpenAI, Anthropic, Gemini, DeepSeek, Groq, and more) and local
  (LM Studio, Ollama), switchable mid-conversation with context intact. Full list under
  [Everything it can do](#everything-it-can-do).

## Install

Windows 10 or later, x64. **You do not need an API key to start** — a built-in `aetox` provider
ships five test models that exercise the real machinery (real tool calls, a real delegation to a
sub-agent, a long reasoning stream), so you can see what the app does before signing up for
anything.

**Installer** — [aetox-amd64-installer.exe](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe) (21.3 MB)

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

Two different warnings, two different causes.

**"Windows protected your PC", unknown publisher.** The installer is not code-signed yet, so
Windows has no publisher name to show for it — **More info → Run anyway**.

**"Virus detected", or the name `Program:Win32/Wacapew.C!ml`.** A cloud machine-learning verdict,
not a signature: nobody analysed this file and judged it dangerous. It fires because the installer
is unsigned, brand new, and at install time fetches Tesseract, poppler, ffmpeg and a speech model,
which is the shape of a downloader. Those four are pinned to immutable release tags and every
download is verified against a SHA256 hardcoded in
[the installer script](desktop/build/windows/installer/project.nsi) before it is used; a mismatch
skips that component rather than proceeding. This build has been reported to Microsoft as a false
positive, but a new release carries a new hash, so it can come back until code signing exists. The
portable zip is the way past it in the meantime.

Releases *are* signed: an ed25519 public key is compiled into the binary and the updater verifies
the signature over `checksums.txt` before it trusts a single hash. An empty or wrong key refuses
the update rather than falling back.

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

**All of it happens on the same workbench you are watching.** One window holds four rooms —
slides, browser, files and terminal — and the Code door adds a fifth, Git, which lays out the
uncommitted working tree with a per-file diff. The agent does not work behind a curtain and hand
you a file at the end: it opens a room, works in that room, and you can reach in and change
something yourself without waiting for the turn to finish.

**It builds slide decks.** Give it the subject and what you want out of it, and the deck comes back
as one self-contained `.html` file. Converting it is the app's work rather than the model's.

The deck is delivered when it opens in the slides room, not when it is exported — you page through
it and present it full screen from there, and the export bar is on that same screen. It exports as
`.pdf`, `.png` or `.jpg`: the PDF is the deck file itself through the renderer that draws it on
screen, so it looks exactly like what you were just looking at, and images come out one file per
slide into a folder of their own, named `01`, `02`, so a ten-slide deck sorts correctly everywhere.
A slide's box is 1280x720, which is 13.333 x 7.5in at 96dpi — exactly PowerPoint's widescreen
page.

**Watch it work in a real browser.** Not a headless scrape: a WebView2 window composited into
the app, with an address bar, back/forward, DevTools, and eight device presets that resize the
native window and zoom the page so CSS media queries genuinely fire.

The layer that drives it is ours. One read stamps a number on every interactive element on the
page; click and type aim at that number. The agent hits the right control without a vision model
and without guessing at coordinates, in the same tab you are watching.

<img src="docs/assets/cap-browser.png" alt="The agent driving a page in the workbench browser" width="100%">

**Build websites and systems.** Point it at a folder and the files room becomes a real place to
work: file tree, Monaco editor, unlimited real PTY terminal tabs, `git`, `grep` and `glob` over
the whole tree, plus `diagnostics` and `symbol` backed by language servers the app installs on
first use (gopls, typescript-language-server, svelteserver). Write a page, then open it and look
at the real thing in the browser room next door, without leaving the app to find somewhere to run
it.

**Hand over a folder and get a file back.** Point it at a directory of images, PDFs or
recordings and ask for the thing you actually want. OCR (Thai and English), PDF text with the
layout intact, and offline speech-to-text all feed the same conversation.

**Read what the model cannot see.** `image_ocr` runs Tesseract with Thai and English, so a
screenshot, a scan or a photographed form becomes text a 9B/35B model can reason about — no vision
model required, and the model that *can* see gets the image itself instead.

<img src="docs/assets/cap-image-ocr.png" alt="OCR pulling Thai text out of an image" width="100%">

**Delegate to a specialist.** Type `@doc`, `@sheet`, `@github`, `@automation` or
`@research` and your sentence reaches that agent word for word — not a paraphrase. Each is a folder on disk with its
own prompt, its own memory, optionally its own model, and its own private skills.

**Give it a job, not a step.** Work that takes twenty moves is planned before it is worked, and
`todo_write` puts that plan on screen while it runs, so what you watch is the order it chose
rather than a spinner. Up to four specialists run at once: `task` hands work out and returns
immediately, `task collect` picks it up, so three jobs cost the time of the slowest rather than
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

Five agents ship — `doc`, `sheet`, `github`, `automation`, `research` — and hiring a
sixth is dropping a folder into `<DataRoot>/agents/`. No release, no plugin API, no restart.

An agent's folder is its whole identity: `AGENT.md` (who it is, what desk it sits at, which tools
it may narrow itself to, which model it pins), `MEMORY.md` (what it has learned), `STARTERS.md`
(how an empty chat with it opens, per language), and a private `skills/` folder no other agent
can see.

That folder is also where the difference between a clever assistant and a company sits. Each
agent pins **its own model**, so the one that opens twenty pricing pages can run on something
cheap while the one that has to weigh what it found runs on something strong, and the bill
follows the work instead of following the hardest task in it. Each keeps **its own memory**, so
what the research agent learned about a source does not leak into the document agent's judgement
about a contract. A single generalist has one model, one memory and one set of tools for every job
it will ever be handed — and no way for you to add a sixth colleague to it.

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
| Browser data (history, cookies, session) | Stays on your machine only — no server of ours sits in between |
| Cutting the cloud off entirely | Your data stays on your machine and in your country — run through LM Studio or Ollama and not a single byte leaves |
| API keys | Their own file, 0600, DPAPI-wrapped against your Windows account. Off Windows there is no encryption at rest — that is stated rather than implied |
| Secrets in logs | Stripped through one registry into all three sinks: debug log, shell audit log, and the buffer the bug-report form reads |
| MCP secrets | `${env:VAR}` indirection, so a key never lands in the settings file |
| Taking it with you | Export any chat to `.md` or `.json`, and import a `.json` back into any Aetox |
| Bug reports | The app transmits nothing. It prefills a GitHub issue, already scrubbed, and you read every line before sending it from your own account |

There is no server of ours in the middle and no analytics. Using a cloud provider means that
provider sees what its API normally sees, and nothing is routed through us.

## Everything it can do

A tool count is not a reason to use anything, which is why this is down here.

**35 tools reach the model on a fresh install**; a default assistant session carries fewer,
because a desk narrows the set. They cost about 7,763 tokens on every request before you have
typed anything, against a ceiling of 10,400 tokens and 48 tools that a test enforces.

| Group | Tools |
|:---|:---|
| **Files** | `apply_patch` `delete` `edit` `glob` `grep` `list` `read` `write` |
| **Running commands** | `desk_terminal` `git` `shell` *(run · output · kill · list)* |
| **Handing back files** | `doc_write` `sheet_write` |
| **Reading media** | `audio_transcribe` `image_ocr` `pdf_read` `video_ocr` |
| **Web and automation** | `browser` *(open · read · click · type · wait · back · scroll · capture · tabs · dialog · console · network)* `web_fetch` `web_search` |
| **Code work** | `diagnostics` `github` *(search · repo_summary · list_files · read_file)* `symbol` |
| **How the assistant works** | `ask_user` `calc` `desk` *(open · list · close)* `memory` `plugin_install` `session_search` `skill_view` `skills_list` `suggest_task` `task` *(start · collect · answer · plan)* `time` `todo_write` |

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

**22 providers, and the window shows every one** — OpenAI · Anthropic · Gemini · DeepSeek ·
Qwen · Z.ai · OpenRouter · Codex · Groq · Mistral · Kimi · MiniMax · xAI · ThaiLLM ·
ModelScope · NVIDIA · Ollama Cloud · OpenCode Zen · OpenCode Go · LM Studio · Ollama · and the built-in `aetox`. OpenRouter and Codex sign in; the rest take an
API key or a local server address. The catalogue and the picker used to disagree; they no longer
do, because a provider the engine knows and the window hides is one nobody can reach.

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

## When a turn goes wrong

**An answer cut off by the output-token limit is continued.** A reply that hits the ceiling used to
reach you stopped mid-word, with nothing anywhere asking for the rest. The turn now carries on up to
three times and appends to what is already on screen, so one answer is watched being written rather
than vanishing and starting over.

**A tool call that names the same argument twice is refused.** A model asking for three searches
sometimes writes two of them into one object — `{"query":"A","query":"B"}`. That is valid JSON, so a
parser keeps the last and drops the first without a word: one search never runs, and the answer
reports on it anyway. Aetox rejects the call and tells the model what was actually wrong with it,
rather than letting a silent loss reach the answer.

**A provider that returns nothing is an error, not an empty answer.** A turn 350 seconds in with
eighteen tool results behind it once died on a round that came back without a single frame of text.
That round is replayed twice — with whatever streamed taken back first — and only then does the turn
change the question instead of asking a fourth time. Everything the turn had already done stays in
context either way.

**What a model can do only ever narrows the toolset, never widens it.** A model the catalogue has
never described keeps every tool; one the catalogue says cannot call tools is narrowed. Wrongly
withholding tools turns an agent into a chat window, so doubt is resolved in one direction only.

## Measured, not claimed

The rules are in [BENCHMARK.md](BENCHMARK.md), and its one standing rule is that a number which
has not passed them may not appear here or on the website.

> The dangerous number is the flattering one, because nobody audits a figure that makes them look
> good.

**Aetox.** The two size rows and the two test counts were re-measured 2026-08-25 on v1.5.7;
assembling a turn is from 2026-08-13, and the ⁽ᵈ⁾ rows from 2026-07-27 on v0.9.2.

| | |
|:---|---:|
| What you download | 21.3 MB installer |
| What ends up on disk | **48.5 MB**, one file |
| Assembling a turn | 0.32 ms · 174.9 KB allocated |
| Go tests | 2,295 across 42 packages, 0 failures |
| Frontend tests | 953 across 91 files, 0 failures |
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
| Disk | **48.5 MB** | 419 MB |

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
unpack it, and read the size of the one `aetox.exe` inside: 50,818,560 bytes. Anyone can reproduce
it in a minute. It replaces the 47.5 MB figure measured on 2026-08-18, which was correct then and
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

## Status — v1.5.7

The core is in place. [Release notes](docs/release-notes/v1.5.7.md) ·
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
[Where every published number lives](docs/PUBLISHED-NUMBERS.md) ·
[Platform support](PLATFORM-SUPPORT.md) · [Roadmap](ROADMAP.md) ·
[Automation engines](docs/AUTOMATION-ENGINES.md)

## Community

There is a Facebook group for questions, ideas, and the kind of half-formed problem that does not
fit in an issue yet: [the Aetox group](https://www.facebook.com/share/g/1BnXC5EiWg/). Bugs are
still better filed as issues, because an issue carries the version and the log with it.

## Reporting bugs

[Open an issue](https://github.com/Mike0165115321/Aetox/issues). The app has a door for this:
Settings prefills a GitHub issue with your version, install channel and OS, folds the recent
internal log into a `<details>` block with secrets already stripped, and hands it to you to read
before you send it from your own account. Nothing is transmitted by the app.

## Who makes this, and the licence

Aetox is written by one person. It exists because a model that can only produce text is half a
tool, and the missing half — hands, permission, and a place to put the result — is an application
problem rather than a model problem.

**Aetox is free to use and is not open source.** From v1.3.0 it is under a
[proprietary licence](LICENSE): install it on as many machines as you like, use it for commercial
work, read the whole source and audit it — but do not modify it, redistribute it, rebrand it, or
sell it. The source is published to be *read*, not to be built on.

**Your own extensions are yours.** Skills, agents, prompts, configuration and MCP servers you
write are your property, and selling them is expressly permitted ([LICENSE](LICENSE) §4). That is
what the extension points are for.

The name **"Aetox"** and the logo are trademarks and are not licensed to anyone else. Third-party
components keep their own terms and are listed in
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) — none of them is GPL or AGPL.

Earlier releases keep the licence they shipped under, permanently: v0.7.1 and earlier are MIT,
v0.8.0 through v1.2.4 are Apache-2.0.

> Aetox was not born to compete with anyone. It exists to stand where the market has a gap — not
> to be one more agent framework, and not to lock anyone into anything.

📧 [phrmsawanachyphl@gmail.com](mailto:phrmsawanachyphl@gmail.com) ·
❤️ [Support the project](SPONSOR.md)

---

<p align="center">
  © 2026 Chayaphon Phromsawana · All rights reserved · <a href="LICENSE">Licence</a> · <a href="THIRD-PARTY-NOTICES.md">Third-party notices</a>
</p>
