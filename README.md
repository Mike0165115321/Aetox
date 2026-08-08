<p align="center">
  <img src="docs/assets/logo.png" alt="Aetox" width="110">
</p>

<h1 align="center">Aetox</h1>

<p align="center">
  <strong>The execution layer for AI.</strong><br>
  <sub>Models provide intelligence. Aetox provides capability.</sub>
</p>

<p align="center">
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/Mike0165115321/Aetox?color=2f81f7"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
  <img alt="Tests" src="https://img.shields.io/badge/tests-1%2C438%20Go%20%2B%20365%20UI-brightgreen">
  <img alt="Installer" src="https://img.shields.io/badge/installer-13%20MB-brightgreen">
  <img alt="Platform" src="https://img.shields.io/badge/platform-Windows-lightgrey">
</p>

<p align="center">
  <a href="https://aetox-puce.vercel.app/"><strong>Website</strong></a> ·
  <a href="https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe"><strong>Download</strong></a> ·
  <a href="COMPANY.md"><strong>What it is</strong></a> ·
  <a href="docs/DECISIONS.md"><strong>Why it is that way</strong></a>
</p>

---

**Tell Aetox what needs doing, and it does it on your computer.** A model on its own can only
produce text. Aetox is the layer underneath that turns an intention into work that actually
happened — files changed, a form filled in, a document produced, a program run.

```
                    MODEL          ← the brain: knows what should happen
                      ↓
                 ┌─────────┐
                 │  AETOX  │      ← the body: eyes, ears, hands, tools, permission
                 │Execution│
                 │  Layer  │
                 └────┬────┘
                      ↓
          ┌───────────┼───────────┐
          ↓           ↓           ↓
        Files      Browser      Shell
          ↓           ↓           ↓
      Documents    Websites    Programs
```

That is why **any** model works, including a 9B on your own graphics card: the capability
lives in the layer, not in the parameters. Aetox reads images, watches video, hears audio,
opens real web pages and clicks through them, and hands back real `.docx` / `.xlsx` / `.pptx`
files — with a model that can do none of those things by itself.

One 40.8 MB Windows executable. No runtime to install. Nothing leaves your machine.

> **Don't build an AI that answers more. Build an AI that does more.**

<p align="center">
  <img src="docs/assets/hero-app.png" alt="Aetox desktop" width="90%">
</p>

## Install

**Installer** — [aetox-amd64-installer.exe](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe)
· adds a Start Menu entry and sets up Tesseract, poppler and ffmpeg for you.

**Scoop**

```powershell
scoop install https://raw.githubusercontent.com/Mike0165115321/Aetox/main/scoop/aetox.json
```

**Portable** — [the zip](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-windows-amd64-portable.zip),
unpack and run `aetox.exe`.

**No API key needed to start.** A built-in `aetox` provider ships a mock model that exercises
every feature — real tool calls, long reasoning, image galleries — so you can try it before
signing up for anything.

> The installer is not code-signed yet. The first run shows a SmartScreen warning about an
> "unknown publisher" — More info → Run anyway.

<details>
<summary>Build it yourself</summary>

```powershell
cd desktop
wails build          # → desktop/build/bin/aetox.exe
wails build -nsis    # with the installer
```

</details>

## Two doors, one app

Most tools pick a side: a chat assistant for everyone, or a coding tool for developers. That
choice only has to be made if the product *is* the assistant. Here the product is the engine,
and a door is just a way of pointing it at a kind of work — so Aetox is both, without either
bleeding into the other. One switch on the logo moves between them; the app remembers which
one you were in.

```
                    AETOX ENGINE
                          │
           ┌──────────────┴──────────────┐
           ↓                             ↓
    AETOX ASSISTANT                 AETOX CODE
    ───────────────                 ──────────
    Everyday work                   Software
    Files · Browser                 Files · Shell
    Documents · Research            Git · LSP · Workbench
```

| | 🏠 **Aetox Assistant** | 🔧 **Aetox Code** |
|:---|:---|:---|
| **Who it is for** | Anyone — including someone who has never opened a folder | Developers |
| **Where it works** | Your whole machine (credential stores always refused) | The project folder you opened, plus folders you add |
| **What it holds** | Files, shell, web, browser, OCR, speech, PDF, Office output, a team of agents | Files, shell, git, language-server diagnostics, symbol lookup, GitHub, browser |
| **Rooms** | Assistant · Projects · Agent team · Output gallery · Automation *(coming)* | Code, with the Workbench: editor, terminal, browser, file tree |
| **What it will not do** | Developer tooling — it says so and points you next door | Slides and documents — that is the other door's work |

**One engine, one store.** Two doors are two shells over the same binary, the same data
directory, the same settings, memory and permissions. Switching doors is not switching apps.

## What it does that other agents do not

### It gives a small model senses it does not have

A 9B model on your own machine cannot see, hear, or act on a page. Through Aetox it can —
and **none of this needs a vision model.**

| The model cannot… | Aetox supplies | Result |
|:---|:---|:---|
| See images | `image_ocr` (Tesseract, Thai + English) | Any model reads the text in a picture |
| Watch video | `video_ocr` (ffmpeg + OCR) | Text with `[m:ss]` timestamps |
| Hear audio | `audio_transcribe` (whisper.cpp, offline) | Speech to text from audio *and* video |
| Open a PDF | `pdf_read` (poppler) | Attach it and ask; Thai comes out intact |
| Act on the web | `browser_open` / `read` / `click` / `type` | Reads the real page and clicks by reference |

One clip can go through `video_ocr` and `audio_transcribe` together — both emit the same
`[m:ss]` format, so the two read as a single transcript. A model that *can* see gets the
image itself; OCR is the fallback for models without eyes, not the only road in.

### It hands back files, not descriptions of files

`sheet_write` → `.xlsx`, `slides_write` → `.pptx`, `doc_write` → `.docx`. Real files Excel,
PowerPoint and Word open — numbers you can `SUM`, dates that sort, Thai vowels that sit
where they belong. Built by hand from `archive/zip` + `encoding/xml`, **with no added
dependency at all.** Click an `.xlsx` in the app and it renders as a table.

### A number is worked out, not remembered

A wrong sum is the only mistake that never announces itself — it arrives in the same
confident sentence as a right one. `calc` runs a short JavaScript program in an interpreter
compiled into the app, and **you see the script beside the result**, so a wrong answer is a
line of arithmetic you can point at rather than a number you had to trust.

### A team you hire by dropping in a file

The assistant delegates to agents that work in their own memory space without blocking your
turn. Each one is a folder: `agents/<name>/` holds what it is and what it has learned.
Hiring one is adding a file — no release, no plugin API. Ask for a deck and the `deck` agent
makes it; the main assistant never carries those tools, so you do not pay for them on every
question.

### It knows what it is made of

A bundled skill tells the assistant where its own data, skills, agents and settings live —
so it stops guessing about its own system, and stops searching the web for something that is
on your disk. It costs nothing until something opens it.

## Why it is this small and this fast

Not a claim about care taken — a list of decisions, each with the number it bought.

**One static Go binary, no runtime.** Go + Wails + Svelte 5 compile to a single
self-contained `aetox.exe`. Nothing to install alongside it, no `node_modules` to disagree
about versions on update, no Node runtime bundled inside the app. **40.8 MB, one file.**

**The window is WebView2, which Windows already has.** Electron apps ship their own copy of
Chromium; Aetox uses the shared one. Straight about this: WebView2 *is* Chromium, so the
252 MB it holds is not a memory win over Electron — the win is that you are not handed a
second copy of a browser to store.

**The request prefix never moves.** Tools are assembled in a fixed order and serialised once,
so the head of every request is byte-for-byte identical and providers that cache recognise
it. **97% of input tokens came from cache** over six consecutive messages. Shift the tool
order by one and the whole cache breaks silently on every request — no error, just a larger
bill — which is why the order is pinned by a test.

**Skills load in three levels, so the tool list never grows.** A skill costs nothing until
the model opens it: `skills_list` returns one line each, `skill_view` returns one body.
Install three hundred and **the tool block stays the same size** — 48 definitions,
~9,600 tokens, with a test that fails the build if it grows.

**Search costs no model call.** History lives in local SQLite with FTS5, so `session_search`
across every past conversation and every tool run is a query, not an inference. **Zero
tokens per search**, Thai and English alike.

**OCR instead of a vision model.** `image_ocr`, `video_ocr` and `browser_read` turn pixels
and pages into text, so capability does not depend on the model having eyes — which is what
makes a **9B on a consumer GPU** enough.

**The interpreter is compiled in.** `calc` runs JavaScript in an embedded goja runtime with
a **5-second clock** (set by measurement: ~7M loop iterations/second, so one second would
have refused any real calculation) and a **256 MB heap watch**. Nothing to install, and it
can reach no file, socket or process — which is why it needs no approval prompt.

**Assembling a turn takes 0.12 ms.** The heaviest thing the app does before sending is
building the whole tool list and turning it into JSON: **96.2 KB allocated, one
eight-thousandth of a second.** The time you wait is the model thinking.

## Measured against the alternatives

**Disk** — same machine, Windows 11. Smaller is better.

```
Aetox         █                                            40.8 MB
Copilot CLI   █████                                         137 MB
Claude Code   ████████                                      236 MB
Zed           ██████████████                                419 MB
OpenCode      █████████████████                             498 MB
Codex         ████████████████████████                      698 MB
Cursor        ██████████████████████████████                874 MB
VS Code       ████████████████████████████████████████    1,171 MB
```

Aetox carries a full UI — Monaco editor, terminal, file tree, agent-driven browser, LSP
diagnostics — and is still **5.8× smaller than Claude Code** and **21× smaller than Cursor**.
Every terminal-only tool in that list is larger than all of Aetox.

> If the reason you still use a CLI is "desktop apps are heavy", that reason has expired.
> Aetox has a CLI in the source too, and it is deliberately **not shipped** — the desktop
> does more in less space.

**In use** — measured against apps with a UI (a prompt has no window to time). Zed is native
Rust with a reputation for being light, which makes it the harder ruler.

| | Aetox | Zed | Cursor |
|:---|:---|:---|:---|
| First launch (cold) | **1.77 s** | 2.12 s | — |
| Every launch after | **0.53 s** | 0.53 s | — |
| RAM committed | **252 MB** | 471 MB | 1,750 MB |
| Processes | **7** | 4 | 10 |
| Disk | **40.8 MB** | 419 MB | 874 MB |

**Local models keep up** — a 9B on a consumer GPU with the full tool set attached
(~3,000 tokens per request): LM Studio starts answering in **1.42 s**, Ollama in **1.75 s**,
both with live streaming, visible reasoning, real tool calls and correct token accounting.

**Stability** — `go test ./...` passes **1,438 tests across 34 packages, 0 failures**. The
frontend suite adds **365** more.

<details>
<summary>How these were measured, and what does not qualify</summary>

The rules are in [BENCHMARK.md](BENCHMARK.md), and its single standing rule is that a number
which has not passed them may not appear here or on the website. The dangerous number is the
flattering one, because nobody audits a figure that makes them look good.

**Disk (40.8 MB)** — re-measured for v0.9.2 from the shipped portable artifact: download
[the zip](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-windows-amd64-portable.zip),
unpack it, and that is one self-contained `aetox.exe`. Anyone can reproduce it in a minute.
It replaces the 33 MB figure logged on 2026-07-27, which was correct then and is not now —
the binary grew as tools went from 20 to 48. The installer additionally sets up Tesseract,
poppler and ffmpeg, which are separate programs and not counted here.

**Launch, RAM and process count** — measured 2026-07-27 under the BENCHMARK rules (empty
project, settled, median of 5 runs). These have not been re-measured on the v0.9.2 build; a
check on 2026-07-28 saw launch unchanged at 0.53 s and RAM at 276 MB, but it ran 3 rounds
instead of 5 and settled for 25 s instead of 60, so it broke the rules and is recorded only
as a no-regression check, never as a published figure.

**Comparisons** — Cursor was measured after about six minutes of real use with the
extensions this machine's owner had installed: a real working state, not the clean one Aetox
and Zed were measured in, which is why its launch time is absent. VS Code was running a job
that could not be interrupted, so only its size is listed. Sizes are measured after install,
never taken from a vendor's download page — Aetox's own installer is 12.6 MB against 40.8 MB
installed, and mixing the two kinds of number breaks the whole table.

</details>

## Your data

Aetox sends nothing off your machine except what you ask it to. There is no server of ours
and no analytics in the middle.

| | Where it stands |
|:---|:---|
| **Chat history, files, output** | On your disk, in local SQLite |
| **Cutting the cloud off entirely** | Run the model through LM Studio or Ollama — not one byte of the prompt leaves |
| **What the agent may touch** | The project folder plus folders you added; or, with no project focused, the machine minus the credential stores below. Every shell command is logged |
| **Never reachable, in any mode** | `.ssh` `.aws` `.gnupg` `.azure` `.kube` `.netrc` `.git-credentials`, browser profiles, and Aetox's own key files — refused by every file tool whatever folders you added |
| **API keys** | Encrypted at rest against your Windows account (DPAPI), in their own file apart from settings, and stripped from logs, the audit trail and tool history automatically |

Using a cloud provider means that provider sees what its API normally sees — nothing more,
and nothing routed through us.

## What is actually behind it

The sections above are the claim. This is the evidence — and it is deliberately down here,
because a tool count is not a reason to use anything. A car is not sold on having 47
bearings; it is sold on getting you there.

**48 tools reach the model**, grouped by what they are for:

| Group | Tools |
|:---|:---|
| **Files** | `read` `write` `edit` `apply_patch` `notebook_edit` `delete` `list` `glob` `grep` |
| **Code** | `diagnostics` `symbol` `shell` `shell_output` `shell_kill` `git` |
| **Office output** | `sheet_write` `slides_write` `doc_write` |
| **Senses** | `image_ocr` `video_ocr` `pdf_read` `audio_transcribe` |
| **Web** | `web_fetch` `web_search` `browser_open` `browser_read` `browser_click` `browser_type` |
| **GitHub** | `github_repo_summary` `github_search` `github_read_file` `github_list_files` `plugin_install` |
| **Thinking** | `calc` `memory` `session_search` `todo_write` `ask_user` `suggest_task` `time` |
| **Delegation** | `task` `task_result` `task_answer` `desk_list` `desk_open` `desk_terminal` |
| **Skills** | `skills_list` `skill_view` |

**That number is capped on purpose.** 48 definitions cost about 9,600 tokens on every single
request, before you have typed anything — so a test fails the build when the tool block grows,
and the count is currently full: the next tool has to displace one.

Everything else expands where it costs nothing until used. A skill document is not a tool —
install three hundred and the tool list is the same size, because the agent opens one only
when it needs it. MCP servers are attached per desk and per agent, so a server installed for
video work does not take up room in an ordinary conversation. **Growth goes there, never into
the block every request pays for** ([COMPANY.md §1.1](COMPANY.md)).

**18 providers** — OpenAI · Anthropic · DeepSeek · Gemini · Groq · Mistral · Together ·
Perplexity · Cohere · OpenRouter · Codex · Z.ai · Qwen · Kimi · MiniMax · LM Studio ·
Ollama · and the built-in `aetox`. Sign in where sign-in exists, or bring an API key.
Local models get everything a cloud one does: they pick up the models you already downloaded,
stream the answer and the reasoning, really call tools, and count tokens into the same stats.

## Status — v0.9.3

The core is in place and the next layer is going up. [Release notes](docs/release-notes/v0.9.2.md)
· [architecture](ARCHITECTURE.md) · [every decision and why](docs/DECISIONS.md).

**Working today** — everything above, plus: multi-provider in one session, model switching
without losing context, live streaming, three approval levels, full-text searchable history
in Thai and English, an agent that can search its own past work, undo for files a turn
changed, projects that carry context files into every chat inside them, and an update check
that matches how you installed it.

**Next** — agents in parallel; then a team with defined roles handing work to each other;
then automation, where you describe a repeating job and Aetox writes and schedules it.

The foundation is already laid for that: every agent is self-contained with its own model
and memory, all tools share one interface, and every permission question goes through a
single gate — so adding a team is building on top, not tearing down.

**Further out** — a plugin ecosystem, a knowledge base over your notes and code, and
ensemble reasoning where several models argue toward the best answer.

## Philosophy

| | |
|:---|:---|
| **Architecture > Parameters** | Good architecture beats a trillion parameters |
| **Freedom > Convenience** | No lock-in is worth more than convenience that binds you |
| **You Own It** | The data, the models, the settings — all yours |
| **Direction > Execution** | Know where you are going before you start |
| **Pattern > Ad-hoc** | Do it once, automate it for good |
| **Simplicity > Complexity** | The simplest solution, not another layer |

**What gets built, and what does not.** A feature has to make the AI *do* more — either
something it could not do at all, or the same thing faster, cheaper or more safely, or make
it easier for a person to hand work over. "A competitor has it" is not a reason. The rule
lives in one place, [COMPANY.md §1.2](COMPANY.md), so it cannot drift into two.

---

> Aetox was not born to compete with anyone. It exists to stand where the market has a gap —
> not to be one more agent framework, and not to lock anyone into anything.
>
> — Mike (Chayaphon Phromsawana)

## License and brand

Code under [Apache-2.0](LICENSE): use, modify, redistribute and sell it freely, including in
closed-source work. Two things asked in return — keep the [NOTICE](NOTICE) file and the
copyright notice, and say which files you changed.

The name **"Aetox"** and the logo are trademarks and are **not** covered by the license
(Apache-2.0 clause 6). Fork freely; anything you redistribute carries your own name, so
nobody is confused about which one is the original. Version 0.7.1 and earlier were MIT, and
those versions stay MIT forever.

## Contact

Open to conversations about business, partnership and investment. The long-term goal is to
train our own model, so the architecture and the model are designed for each other from the
start.

📧 [phrmsawanachyphl@gmail.com](mailto:phrmsawanachyphl@gmail.com) ·
🐙 [Open an issue](https://github.com/Mike0165115321/Aetox/issues) ·
❤️ [Support the project](SPONSOR.md)

---

<p align="center">
  © 2026 Chayaphon Phromsawana · <a href="LICENSE">Apache-2.0</a> · "Aetox" is a trademark, not covered by the license
</p>
