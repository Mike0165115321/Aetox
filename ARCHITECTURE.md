> **Pass level:** Full Mode
> **Trigger:** whole-project documentation requested at root; 3+ interacting modules (root/`internal`, `engine`, `providers`, `cli`, `desktop`), local persistence (SQLite), 11+ external provider integrations, remote code execution path (`plugin_install`).
> **Scope:** entire repository (`e:\Aetox\Aetox`), last updated 2026-07-24.
> **Evidence:** file tree, `go.work`/`go.mod`×4, `README.md`, `AETOX.md`, `docs/adr/0001`, `docs/adr/0002`, `docs/architecture/module-split-2026-07-21.md`, `docs/architecture/browser-security-2026-07-21.md`, `TEST-REPORT.md`, `MCP-SUPPORT-PLAN.md`, and direct reads of `cmd/aetox/main.go`, `internal/app/app.go`, `internal/cognitive/agent.go`, `internal/turn/executor.go`, `internal/skill/{skill,dispatcher,github_tools}.go`, `internal/safety/safety.go`, `internal/config/config.go`, `desktop/{app,browser,terminal,db,sessions,workbench}.go` and their `_test.go` files, `desktop/frontend/src/{App.svelte,style.css,lib/workbench/*}`.
> **Skipped:** Svelte component internals, provider-by-provider implementation detail (`internal/model/*.go` bodies), test file contents (existence noted, not read line-by-line).
> **Status labels used below:** `Direct` = confirmed by reading the file. `Inferred` = derived from evidence but not line-verified. `Proposed` = design intent, not yet built — never presented as existing.

This document is an evidence-first architecture map, distinct from [README.md](README.md) and [AETOX.md](AETOX.md), which are product vision/pitch documents and mix shipped state with roadmap in the same tables. Where they conflict with the code, this document follows the code.

---

## Document Map — this file is the hub

**Owner's law (2026-07-24): this is the master architecture document. Every separate architecture doc must be referenced here — a new doc gets its row in this table (and, if it records a decision, a numbered Decision section) in the same commit that creates it.**

| Doc | What it is |
|---|---|
| **This file** | Evidence-first whole-system map + the numbered Decision log (§10–§28). Start here; everything below is a spoke. |
| [README.md](README.md) · [AETOX.md](AETOX.md) · [Aetox Desktop.md](Aetox%20Desktop.md) | Product vision/pitch documents — mix shipped state with roadmap; this file wins on conflicts. |
| [docs/architecture/module-split-2026-07-21.md](docs/architecture/module-split-2026-07-21.md) | Why an `engine/`/`providers/`/`cli/` split was proposed and the migration plan (§4). ⚠️ The scaffold directories it describes were deleted in §28 — the rationale stands, the on-disk structure is gone. |
| [docs/architecture/browser-security-2026-07-21.md](docs/architecture/browser-security-2026-07-21.md) | Browser tab `postMessage` bridge — threat model, 3-check defense, residual risk (§6.6). |
| [docs/architecture/desktop-app-2026-07-22.md](docs/architecture/desktop-app-2026-07-22.md) | Layer-5 deep dive: every `desktop/` Go file + workbench frontend, read in full. |
| [docs/architecture/model-control-layer-2026-07-22.md](docs/architecture/model-control-layer-2026-07-22.md) | Layer-2 deep dive (`turn`/`cognitive`/`skill`/`safety`). ⚠️ Executor sections superseded by §17 — the doc says so itself. |
| [docs/architecture/tesseract-ocr-bundling-2026-07-22.md](docs/architecture/tesseract-ocr-bundling-2026-07-22.md) | How `image_ocr`'s Tesseract dependency reaches the user's machine per OS. |
| [docs/architecture/native-browser-embedding-2026-07-24.md](docs/architecture/native-browser-embedding-2026-07-24.md) | Native browser embedding: architecture, 7-entry failure catalog, macOS/Linux port blueprint (§18). |
| [docs/adr/0001-native-tool-calling-foundation.md](docs/adr/0001-native-tool-calling-foundation.md) | ADR, Accepted 2026-06-07 — native tool calling as the agentic foundation. |
| [docs/adr/0002-directional-cognition-engine.md](docs/adr/0002-directional-cognition-engine.md) | ADR, Proposed 2026-07-10 — long-term multi-AI orchestration vision (ensemble/routing/consensus). |
| [MCP-SUPPORT-PLAN.md](MCP-SUPPORT-PLAN.md) | MCP integration plan (skill.Tool is already MCP-shaped; staged rollout). |
| [PLATFORM-SUPPORT.md](PLATFORM-SUPPORT.md) | What actually runs where: Windows is the only real platform, CLI/engine cross-compile but have never been executed on Unix, desktop is hard-blocked. Deliberately a record, not a plan (§29). |
| [SETTINGS-PARITY-PLAN.md](SETTINGS-PARITY-PLAN.md) | Settings-parity roadmap vs ZCode (Skills/Plugins → Onboarding → Usage → Commands → Preview → Subagents; Indexing deliberately skipped) — decisions recorded in §24. |
| [third_party/go-webview2/AETOX-PATCH.md](third_party/go-webview2/AETOX-PATCH.md) | Why go-webview2 is vendored+patched: stop a single browser tab's WebView2 error from `os.Exit`-crashing the whole app (§26). |
| [TEST-REPORT.md](TEST-REPORT.md) | Module-by-module test coverage and known untestable seams. CI that runs it all: [.github/workflows/ci.yml](.github/workflows/ci.yml) (§28). |
| [LICENSE](LICENSE) | MIT (§28) — the license README and `scoop/aetox.json` both reference. |
| [docs/opencode-study/](docs/opencode-study/README.md) | Source-level reading of opencode at a pinned commit (agents, MCP, permissions, plugin hooks, snapshot). |
| [docs/architecture-reference-opencode.md](docs/architecture-reference-opencode.md) · [docs/competitor-research.md](docs/competitor-research.md) | Package/feature-level comparisons that motivated the deep study above. |
| [docs/architecture-review-aetox-cli.md](docs/architecture-review-aetox-cli.md) | **Superseded** (predates `desktop/`); kept for history. |
| Tier-1 module READMEs | `internal/{app,turn,skill,model,grammar,prompt,rtk}/README.md`, `cmd/aetox/README.md`, `desktop/README.md` — hub-and-spoke rule per §12: meaningful change to a module updates its README in the same commit. |

---

## Reader's Map — the "5 layers" mental model vs. actual code

A working mental model used for planning this project splits responsibility into 5 layers: **model management**, **model-control (skills/MCP)**, **orchestrator (multi-agent)**, **UI/CLI front ends**, and **desktop app**. This section states plainly how much of that is real, separate code today, so the mental model isn't mistaken for the module boundaries in §4.

| Layer | What exists today | Where |
|---|---|---|
| 1. Model management | Real behavior, but **not its own module** — lives inside `internal/model` (interface + all 11 provider clients + factory/bootstrap) and `internal/provider` (catalog), both part of the same flat root module. `providers/` (scaffold, §4) is where this is *meant* to move — zero code there yet. |
| 2. Model-control (skill dispatch, tool-calling loop) | Real, but **three cooperating packages, not one**: `internal/skill` (Registry/Dispatcher/15 tools), `internal/cognitive` (Agent), `internal/turn` (Executor) — see the flow in §5. MCP itself is not built (`MCP-SUPPORT-PLAN.md`). |
| 3. Orchestrator (multi-agent) | **Scaffold only.** `internal/orchestrator` exists (§10) but nothing calls it — both front ends still construct exactly one `cognitive.Agent` each. |
| 4. UI/CLI front ends | Real. Two front ends — `cmd/aetox` (CLI) and `desktop/` (GUI) — both driving the same engine through `internal/app`. |
| 5. Desktop app (browser/terminal/extension surface) | Real, and the most independently-developed layer as of this session: session persistence (SQLite), native browser tab (WebView2), embedded terminal (ConPTY) — see §4.2, `TEST-REPORT.md` Module 5, and `docs/architecture/browser-security-2026-07-21.md`. |

**Answering "is it 5 clean modules now": no.** Only layer 5 (`desktop/`) is a genuinely separate Go package boundary from the rest. Layers 1–3 are conceptual groupings *within* the same flat `internal/` module (15 packages) — there is no compiler-enforced boundary stopping "layer 1" code from reaching into "layer 2" today. Treat the 5-layer split as a **planning/reading aid**, not as enforced architecture, until the `engine/providers/cli` migration (§4) actually happens.

---

## 1. Synthesis (read this first)

- **Two systems currently share the root Go module without a boundary.** `cmd/aetox` (CLI) and `desktop/` (Wails GUI) both import `internal/app`, but the GUI only uses ~2 of that package's ~35 exported methods (`NewApp`, `RunOnce`) — the rest is CLI terminal presentation (banner, status bar, spinner, Thai-language approval prompts). See [§6.1](#61-internalapp-mixes-orchestration-with-cli-terminal-presentation). `Medium`, `Direct`.
- **`internal/model` imports `internal/provider`**, so the abstraction (Provider interface, Message types) depends on the implementation catalog — already identified by the project's own [module-split proposal](docs/architecture/module-split-2026-07-21.md). Confirmed still true by direct import scan. `Medium`, `Direct`. This was the stated reason `engine/`, `providers/`, `cli/` existed as scaffolds; the coupling outlived them.
- ~~**The 3-module split (`engine/`, `providers/`, `cli/`) is an empty scaffold** — `go.mod` files only, zero source~~ — **deleted 2026-07-25 (§28)** along with `go.work`/`go.work.sum`. Four days of empty directories only obscured where the running code lives: `internal/` and `cmd/`. The migration rationale is preserved in the module-split doc; re-scaffold when migration actually starts.
- ~~`Registry.Register()` silently overwrites on name collision, and there is no built-in-vs-user-added distinction~~ — **fixed 2026-07-21**, see §6.4 below. `Register` now takes a `Source` and rejects collisions instead of overwriting.
- ~~`SearchSessions` (Desktop session search) doesn't work at all — silently returns nothing on every call~~ — **fixed 2026-07-22**, see §6.7 below. Root cause was narrower than first recorded: `snippet()` combined with `GROUP BY` in one query (not the whole query shape). Fixed with a `MATERIALIZED` CTE, single query, no two-step needed.
- **Browser tab Z-order, `postMessage` origin/replay forgery, and two frontend resize/layout bugs — all fixed 2026-07-21/22.** See §6.6 and §6.8 below.
- ~~`README.md`/`AETOX.md` advertise Anthropic support that doesn't exist in the actual provider catalog~~ — **fixed 2026-07-22**, see §6.9 below. `internal/model/anthropic.go` now implements a real client against Anthropic's Messages API (`/v1/messages`, `x-api-key`/`anthropic-version` headers, content-block request/response shape — not OpenAI-compatible, so it's its own file, not a reuse of `openai_compatible.go`), registered in the catalog and wired into the desktop provider picker (`desktop/app.go`'s `desktopProviders`). `zai` is still undocumented in both docs — that half of §6.9 is still open.

**Primary recommendation:** ~~apply the `SearchSessions` fix (§6.7)~~ — applied 2026-07-22, suite green. Next: the `Registry` Source gap (§6.4, closed) unblocks deciding what `plugin_install`'s loader (§6.5) or an MCP client should set as `Source` — `SourceExternal` alone doesn't yet distinguish them. Independently, `zai` (Z.AI/GLM) is fully implemented but still absent from `README.md`/`AETOX.md`'s provider tables — same category of doc-drift as the now-fixed Anthropic gap, worth a follow-up pass.

---

## 2. System Overview (confirmed)

Aetox is a single-user, local-first AI coding/personal-assistant agent with two front ends sharing one Go engine:

- **CLI** (`cmd/aetox`) — interactive REPL or one-shot invocation, Thai-language terminal UI.
- **Desktop** (`desktop/`) — Wails v2 + Svelte 5 GUI with chat, a tabbed workbench (Review/Terminal/Files/Browser), persistent SQLite-backed sessions, and an agent-controlled native browser (WebView2).

Both front ends drive the same tool-calling agent loop: user message → `cognitive.Agent` → provider API (model-driven tool calls) → `turn.Executor` dispatches to one of 15 built-in skills → result folded back into the conversation.

No backend server, no cloud database, no multi-tenant concerns — everything runs on the user's machine against third-party model provider APIs.

---

## 3. System Boundary

```mermaid
flowchart LR
    User(("User"))
    CLI["CLI — cmd/aetox\nThai-language REPL / one-shot"]

    subgraph DesktopApp["Desktop GUI — desktop/ (Wails v2 · Windows-only today, §18)"]
        direction TB
        FE["Svelte 5 frontend\nchat · composer (project-focus + model pickers, §19)\nworkbench tabs — layout persisted per session (§19.2)"]
        Bindings["Wails-bound App\ndesktop/app.go\n(bindings return [] never nil — §19.3)"]
        BrowserHost["Native browser host — desktop/browser.go\nWebView2 child windows on one STA thread\nparent found by PID, never title (§18)"]
        Term["Embedded terminals\ndesktop/terminal.go (ConPTY)"]
        Sessions["Session persistence + FTS5 search\ndesktop/sessions.go · db.go\nunfocused chats → home-dir bucket (§19.1)"]
        FE <--> Bindings
        Bindings --> BrowserHost
        Bindings --> Term
        Bindings --> Sessions
    end

    subgraph Engine["Engine — internal/* (one shared engine, both front ends)"]
        direction TB
        Grammar["grammar (+command facade)\nexplicit tokens only — no NL inference (§17)"]
        Turn["turn.Executor\nexplicit skill → direct dispatch\nelse model tool loop, else streaming chat"]
        Cog["cognitive.Agent\nunbounded tool loop — brakes are\napproval gate + ctx cancel"]
        Skills["skill.Registry — 15 built-ins\n+ browser_open/read/click/type (SourceExternal)\n+ MCP tools via mcp.Manager"]
        Safety["safety — ask / unsafe-only / full-access\nper-command risk rules"]
        Prompt["prompt — system prompt layers\n(identity · environment · user · project)"]
        RTK["rtk (optional)\nsqueezes tool receipts"]
        Model["model — Provider interface\n+ all provider clients + backstops (§17)"]
        Grammar --> Turn
        Turn --> Cog
        Cog --> Model
        Turn --> Skills
        Skills --> Safety
        Turn --> RTK
        Prompt -->|bootstrap| Cog
    end

    User --> FE
    User --> CLI
    CLI --> Engine
    Bindings --> Engine

    Providers[["13 Provider APIs (verified in code)\nOpenRouter, OpenAI, Anthropic, DeepSeek, Z.AI,\nGemini, Groq, Mistral, Together,\nPerplexity, Cohere, LM Studio,\nOllama (local)"]]
    GH[["GitHub API\ngithub_repo_summary · plugin_install · rtk self-install"]]
    MCPExt[["Configured MCP servers"]]
    Web[["Live web pages\n(inside native browser tabs)"]]
    FS[("Local filesystem + shell + git\nrooted at project, or home when unfocused (§19.1)")]

    Model --> Providers
    Skills --> GH
    Skills --> FS
    Skills -.-> MCPExt
    BrowserHost --> Web

    subgraph Data["DataRoot() — every Aetox-owned file, one directory (§14)"]
        DB[("SQLite + FTS5\nsessions per project bucket")]
        Profiles[("WebView2 profiles\nwebview/app · webview/browser")]
        Misc[("model prefs · permissions\ndebug logs · audit log · rtk binary")]
    end

    Sessions --> DB
    BrowserHost --> Profiles
    Engine --> Misc
```

- **Confirmed external dependencies:** 12 named provider HTTP APIs, verified against the actual catalog map in `internal/provider/catalog.go` (not README's list — see §6.9, they differ), GitHub REST API (`internal/skill/github_tools.go`), local shell (`internal/skill/shell.go`), local filesystem, SQLite (confirmed `modernc.org/sqlite`, `Direct` — `desktop/db.go:17`), Windows WebView2 (`desktop/browser.go`, Win32 syscalls — Windows-only, `Direct`).
- **Not observed:** no outbound telemetry/analytics code found in `internal/` or `desktop/` during inspection (`Inferred`, `Verify first: Yes` — not exhaustively grepped for every HTTP client call).

---

## 4. Module Map

```mermaid
flowchart TB
    subgraph rootmod["root module: github.com/Mike0165115321/Aetox"]
        direction TB
        subgraph fronts["Front ends"]
            cmd["cmd/aetox\nCLI entry"]
            dfe["desktop/frontend — Svelte 5\nstores: cockpit · workbench\n(session-bound layout, §19.2)"]
            dgo["desktop/ — Wails Go side\napp · browser · terminal\nsessions · db · workbench"]
            dfe -->|generated wails bindings| dgo
        end
        subgraph engcore["Engine core (internal/)"]
            app["app\nCLI loop + engine wiring"]
            turn["turn\nExecutor — §17 pipeline"]
            cognitive["cognitive\nAgent tool loop"]
            skill["skill\nRegistry + 15 built-ins"]
            safety["safety\napproval + risk"]
            grammar["grammar (+command)\nexplicit-token parsing"]
            prompt["prompt\nsystem prompt layers"]
            memory["memory\nconversation context"]
        end
        subgraph modellayer["Model layer"]
            model["model\nProvider iface + clients + think normalization"]
            provider["provider\nruntime catalog"]
        end
        subgraph infra["Infra"]
            config["config\nDataRoot() · prefs · permissions"]
            mcp["mcp\nMCP server manager"]
            rtk["rtk\noptional receipt squeeze"]
            audit["audit"]
            debuglog["debuglog"]
        end
        orch["internal/orchestrator\nbuilt, no caller yet — §10"]

        cmd --> app
        dgo --> app
        app --> turn
        turn --> grammar
        turn --> cognitive
        turn --> skill
        turn --> rtk
        skill --> safety
        cognitive --> model
        cognitive --> memory
        model --> provider
        app --> prompt
        dgo --> mcp
        mcp -.->|tools registered into| skill
        app -.->|not wired in| orch
    end

```

`rootmod` is the whole picture — one module, no workspace. The `engine/`/`providers/`/`cli/` scaffold that used to sit beside it was deleted in §28; the split it was meant to become is still [proposed](docs/architecture/module-split-2026-07-21.md), just no longer half-present on disk.

### 4.1 `internal/` packages (root module, `Direct`)

Per-module docs (hub-and-spoke, §12): Tier-1 modules carry a `README.md` in their own folder — linked in the **Docs** column. Front-end modules: [cmd/aetox/README.md](cmd/aetox/README.md), [desktop/README.md](desktop/README.md) (§4.2).

| Package | Files | Role | Docs |
|---|---|---|---|
| `app` | 4 | CLI interactive loop + orchestration wiring (`NewApp`, `RunOnce`, `RunInteractive`, banner/status bar, approval-mode picker). Shared with desktop only via `NewApp`/`RunOnce` — see [§6.1](#61-internalapp-mixes-orchestration-with-cli-terminal-presentation). | [README](internal/app/README.md) |
| `cognitive` | 2 | `Agent` — builds provider requests, runs the (unbounded, §11 "related cleanup") tool-call loop, streams responses. | [model-control deep dive](docs/architecture/model-control-layer-2026-07-22.md) |
| `turn` | 3 | `Executor` — turn pipeline (§17): explicit skill command → direct dispatch, else model-driven tool loop, else streaming chat; approval gate on every tool path. No NL intent inference — that layer was deleted 2026-07-23. | [README](internal/turn/README.md) |
| `skill` | 19 | `Registry`/`Dispatcher` + all 15 built-in tools (read/write/edit/delete/list/fs/grep/shell/git/echo/time/help/github_repo_summary/plugin_install/image_ocr). | [README](internal/skill/README.md) |
| `model` | 14 | `Provider` interface, `Message`/`Request`/`Response` types, factory, bootstrap, and **all 11 provider client implementations** in the same package. Imports `internal/provider` (see [§6.2](#62-modelprovider-imports-providers-catalog)). | [README](internal/model/README.md) |
| `provider` | 2 | Provider runtime catalog (names, capabilities) — separate from `model`'s own `provider_catalog.go`, which is a second source for similar data (`Inferred`, `Verify first: Yes` — not diffed line-by-line). | — |
| `safety` | 2 | 3-tier approval (`ask`/`unsafe-only`/`full-access`), per-command risk assessment (`AssessCommand`, git/fs-specific rules). | [model-control deep dive](docs/architecture/model-control-layer-2026-07-22.md) |
| `command` | 3 | Input intent parsing facade — thin aliases delegating to `grammar` (the real implementation). | see `grammar` |
| `config` | 2 | Config loading, `.env`, model-preference persistence (JSON on disk). Owns `DataRoot()` — the single directory every Aetox-owned file lives under, see [§14](#14-decision--unified-data-root-2026-07-23-cleaning-up-where-aetox-writes-its-own-data). | — |
| `think` | 2 | Thinking-level normalization per provider. | — |
| `plan` | 2 | Execution planning (conversation vs. skill classification, per ADR 0001). | — |
| `memory` | 1 | Context/conversation memory. | — |
| `audit` | 2 | Execution audit log. | — |
| `debuglog` | 1 | Debug logging. | — |
| `grammar` | 2 | Input classification rules engine (Kind/Intent/slash parsing) behind the `command` facade. | [README](internal/grammar/README.md) |
| `orchestrator` | 2 | Multi-`cognitive.Agent` lifecycle tracker (`Spawn`/`Get`/`Stop`/`List`). Built this session, **not called by `cmd/aetox` or `desktop/app.go` yet** — see [§10](#10-decision--agent-orchestrator-layer-proposed-approved-2026-07-21) for scope and naming rationale. | §10 |
| `prompt` | 2 | System prompt assembly (identity/environment/user-global/project layers) — both front ends call it, replacing two near-duplicate `buildSystemPrompt` copies. Built 2026-07-22, see [§11](#11-decision--promptcontext-layer-proposed-being-settled-section-by-section-2026-07-22). | [README](internal/prompt/README.md) |
| `rtk` | 4 | Optional bridge to the owner's `rtk` CLI — shrinks tool-call output before it's wrapped into the model's receipt (`turn.modelToolReceipt`). v1: `git`+`shell` only. Auto-installs itself once from GitHub if missing (`install.go`), no NSIS changes. Built 2026-07-22, see [§13](#13-decision--rtk-integration-proposed-being-settled-section-by-section-2026-07-22). | [README](internal/rtk/README.md) |

### 4.2 `desktop/` (root module, `Direct`)

Module doc: [desktop/README.md](desktop/README.md) (replaced the Wails template boilerplate 2026-07-22).

| File | Role |
|---|---|
| `app.go` (665 lines) | Wails-bound `App` struct — the GUI's own type, distinct from `internal/app.App`. Owns `bootstrapFromConfig` (wires `internal/app.App` + `cognitive.Agent` + extra skills), provider/model switching, project tree/file read-write for the Files pane. |
| `sessions.go` | Per-project session persistence: `ListSessions`, `SearchSessions` (FTS5 — fixed 2026-07-22, see §6.7 below), `LoadSession`, transcript ↔ `model.Message` conversion. |
| `db.go` (77 lines) | SQLite connection + schema. `App.dbDir` overrides the default `<UserConfigDir>/aetox` directory (empty = production default) — a test seam added this session, not a behavior change. |
| `browser.go` (~470 lines) | Native WebView2 tab host via raw Win32 syscalls (`wndClassExW`, message loop) — Windows-specific, no build-tag isolation observed (`Inferred`, `Verify first: Yes`). Z-order and `postMessage`-forgery issues fixed this session — see §6.6 below and `docs/architecture/browser-security-2026-07-21.md`. |
| `workbench.go` | `browser_open`/`browser_read`/`browser_click`/`browser_type` implemented as `skill.Tool` — the agent drives the browser itself, distinct from the user-facing `BrowserOpen`/`BrowserNavigate` etc. in `browser.go`. `browser_read` tags interactive elements with `data-aetox-ref` (see `textScript`, `browser.go`) so `browser_click`/`browser_type` can target one by number — same ref-based pattern as Playwright MCP's accessibility tree and browser-use's element index. This is the pattern [MCP-SUPPORT-PLAN.md](MCP-SUPPORT-PLAN.md) recommends reusing for an MCP adapter. |
| `terminal.go` | Embedded shell session lifecycle (`TerminalStart`/`Write`/`Resize`/`Close`), independent of `internal/skill/shell.go` (that one is the agent's `shell` tool; this is the user-facing terminal pane). |
| `main.go` (39 lines) | Wails bootstrap. |

Test files added this session (`Direct`, all passing except the one noted bug): `app_test.go`, `browser_test.go`, `db_test.go`, `sessions_test.go`, `terminal_test.go` — per-file coverage detail in `TEST-REPORT.md` Module 5. `TerminalStart`/`TerminalClose`/`browser.go`'s Win32 window plumbing remain **not** unit-testable (documented there): `wailsruntime.EventsEmit` calls `log.Fatalf`/`os.Exit(1)` when the context isn't a real Wails-bound one, which a test never has.

`desktop/frontend/src/lib/`: `stores/` (Svelte 5 runes: `cockpit.svelte.ts`, `workbench.svelte.ts`), `services/cockpit.ts` (Go binding wrappers), `workbench/*.svelte` (Review/Files/Browser panes). Not read in detail — structure only (`Direct` for existence, `Inferred` for internal behavior), except `App.svelte` (resize-handle logic, read in full — see §6.8) and `workbench/Workbench.svelte`/`style.css` (address-bar + blank-state layout, read in full — see §6.8).

---

## 5. Primary Workflow — one chat turn

```mermaid
sequenceDiagram
    participant U as User
    participant Front as CLI or Desktop
    participant App as internal/app.App
    participant Turn as turn.Executor
    participant Agent as cognitive.Agent
    participant Provider as Provider API
    participant Skill as skill.Registry

    U->>Front: message
    Front->>App: RunOnce / RunInteractive
    App->>Turn: Execute(input)
    Turn->>Turn: command.Parse (skill vs conversation)
    alt known skill command
        Turn->>Skill: dispatchBySkill
        Skill-->>Turn: Output
    else model-driven tool call
        Turn->>Agent: RespondWithTools
        Agent->>Provider: Request (messages + tool defs)
        Provider-->>Agent: tool_call or final text
        Agent->>Turn: ToolCall
        Turn->>Turn: safety.AssessCommand + approval gate
        Turn->>Skill: ExecuteTool
        Skill-->>Turn: Output
        Turn->>Agent: tool result appended, loop continues
    end
    Turn-->>App: Result
    App-->>Front: reply
    Front-->>U: reply
    opt Desktop only
        Front->>Front: sessions.appendTurn (SQLite)
    end
```

Desktop-specific additions confirmed in `desktop/app.go`/`sessions.go`: every turn is persisted via `appendTurn` after `SendMessage` returns; sessions are keyed per project root (`projectKey`); full-text search is FTS5-backed.

---

## 6. Debt Register

### 6.1 `internal/app` mixes orchestration with CLI terminal presentation

- **Evidence:** `internal/app/app.go` exports `NewApp`, `RunOnce`, `RunInteractive`, `PrintBanner`, `printStatusBar`, `printPromptLine`, `pickApprovalMode` (Thai-language console prompts), `showHelp`, `showSkillPalette` — all in one package/type. `desktop/app.go:275-289` (`SendMessage`) calls only `a.chat.RunOnce`; no desktop code path reaches `RunInteractive`, `PrintBanner`, or the approval-mode console picker.
- **Impact:** the GUI binary compiles and links terminal-rendering code it never calls. Any change to CLI presentation (banner, status bar, Thai approval prompts) risks breaking desktop's build even though desktop doesn't use those methods. The project's own [module-split plan](docs/architecture/module-split-2026-07-21.md) intends to move this whole package to `engine/app/` labeled "shared by CLI + desktop" — migrating as-is carries the same mixing forward.
- **Severity:** `Medium` (taxes the planned migration; doesn't break anything today).
- **Confidence:** `Direct`.
- **Direction (proposed, needs approval):** split `internal/app` into an orchestration piece (`NewApp`, `RunOnce`, turn-executor wiring — genuinely shared) and a CLI-presentation piece (banner, status bar, interactive loop, approval picker) before or during the `engine/` migration, so `engine/app/` doesn't inherit terminal-only code.

### 6.2 `model` imports `provider` — abstraction depends on implementation

- **Evidence:** `internal/model/factory.go`, `provider_catalog.go`, `thinking_capabilities.go` import `internal/provider`. Documented and accepted as the reason for the 3-module split in [module-split-2026-07-21.md:8-16](docs/architecture/module-split-2026-07-21.md).
- **Impact:** any consumer of `model.Provider`/`model.Message` transitively pulls in the full provider catalog; can't depend on the interface alone.
- **Severity:** `Medium`.
- **Confidence:** `Direct`.
- **Direction:** already proposed (`engine/model` interface-only, `providers/` for implementations) — no new recommendation, just confirming the existing plan matches the evidence. The empty scaffold that used to stand in for it is gone (§28); the plan is a plan again, not a half-built directory.

### 6.3 Two provider catalogs

- **Evidence:** `internal/model/provider_catalog.go` and `internal/provider/catalog.go` both exist; not diffed for exact overlap.
- **Impact:** unclear without deeper read — possibly one is the canonical list and the other is a thin re-export, or they've drifted.
- **Severity:** `Low`–`Medium` (`Unverified` which).
- **Confidence:** `Inferred`, `Verify first: Yes`.
- **Direction:** none proposed — needs verification first, listed as an open question (§7).

### 6.4 Skill registry has no core/user-added boundary — FIXED 2026-07-21

- **Original evidence:** `internal/skill/skill.go:44` `Registry.Register()` overwrote on key collision with no warning. `internal/skill/defaults.go` registered all 17 built-ins and `plugin_install` into the same flat map. Documented in detail in [MCP-SUPPORT-PLAN.md:37-51](MCP-SUPPORT-PLAN.md).
- **Impact (was):** couldn't gate trust levels differently (built-in vs. third-party MCP/plugin tool), couldn't show "core" vs. "installed" separately in the Settings UI, and a user-installed skill could silently shadow a built-in tool by name.
- **Severity:** was `High` (blocked safe MCP support).
- **Fix applied:** [internal/skill/skill.go](internal/skill/skill.go) — added `Source` type (`SourceBuiltin`/`SourceExternal`), `Registry` now stores `{skill, source}` pairs and exposes `SourceOf(name)`. `Register(skill, source)` returns an error instead of silently overwriting on name collision.
  - [internal/skill/defaults.go](internal/skill/defaults.go) — all 12 built-ins register with `SourceBuiltin`; a collision among built-ins now panics at startup (programmer error, not a runtime condition).
  - [desktop/app.go](desktop/app.go) `bootstrapFromConfig` — `extraSkills` (the `workbenchTools`: `browser_open`/`browser_read`/`browser_click`/`browser_type`) register with `SourceExternal`; a collision is logged via `debuglog.Msg` and the skill is skipped rather than silently overwriting a built-in.
  - Tests: [internal/skill/skill_test.go](internal/skill/skill_test.go) (`TestRegisterTracksSource`, `TestRegisterRefusesCollision`).
- **Still open:** `SourceExternal` is one bucket for everything non-built-in — MCP and `plugin_install`-loaded skills don't yet have their own distinct source values, and nothing consumes `SourceOf` yet (no per-source permission gating, no Settings UI grouping). Add `SourceMCP`/`SourcePlugin` when those integrations are actually built, not before.
- **Confidence:** `Direct` — `go build ./...` and `go test ./internal/skill/...` pass.

### 6.5 `plugin_install` downloads files but nothing loads them back

- **Evidence:** `internal/skill/github_tools.go` implements `plugin_install` fully (fetches manifest, path-traversal-checked, writes to `~/.agents/skills/<name>/`) but `internal/skill/defaults.go` only registers compiled-in Go skills — no bootstrap-time scan of the install directory. Confirmed by direct read of `validatePluginManifest`/`normalizeManifestRelativePath` (traversal guards present) and by `MCP-SUPPORT-PLAN.md:17-23`.
- **Impact:** "installing a plugin" via this tool currently has no observable effect after download completes.
- **Severity:** `Medium` (feature is inert, not unsafe — path traversal is guarded and manifest type is restricted to `"skill-bundle"`).
- **Confidence:** `Direct`.
- **Direction:** proposed in `MCP-SUPPORT-PLAN.md` — write a loader that scans the install directory at `bootstrapFromConfig` time; decide execution model for downloaded (non-compiled) skills first.

### 6.6 Browser tab Z-order + `postMessage` forgery — FIXED 2026-07-21/22

- **Evidence (Z-order):** `desktop/browser.go`'s tab window used only `MoveWindow`/`ShowWindow` — neither changes Z order. Two independent WebView2 controllers in one top-level window each composite via DirectComposition; without an explicit Z-order call the tab's pixels could render behind the app's own webview — confirmed live (page navigated successfully, per-page title updated, but the tab area stayed solid black, the app's own background color).
- **Evidence (`postMessage` forgery):** `onMessage` trusted a page's `postMessage({__aetox:...})` envelope with no check that the page was the one it claimed to be, or that a `"text"` response corresponded to a specific `BrowserGetText` call. `args.GetSource()` (real origin, page can't forge) was available on `ICoreWebView2WebMessageReceivedEventArgs` but unused.
- **Impact:** Z-order bug made the entire browser tab feature appear non-functional (page loads, nothing visible). The `postMessage` gap allowed a malicious page to (a) claim an arbitrary `url` in a `"meta"` message — address-bar spoofing, a phishing enabler — and (b) inject fabricated `"text"` content into the agent's `BrowserGetText` read path — a prompt-injection vector distinct from the page's own real (untrusted) DOM content.
- **Severity:** was `High` for the `postMessage` gap (security), `Medium` for Z-order (functionality, not security).
- **Fix applied:** `procSetWindowPos` + `HWND_TOP` on tab creation/resize/show ([browser.go](desktop/browser.go)). `onMessage` now requires `sameOrigin(source, m.URL)` for `"meta"`, and a matching per-request nonce (`textToken`, minted by `BrowserGetText`, `crypto/rand`) for `"text"`. Full threat model and residual risk in [docs/architecture/browser-security-2026-07-21.md](docs/architecture/browser-security-2026-07-21.md).
- **Confidence:** `Direct` — 12 new tests in `browser_test.go`, `go build`/`go vet` clean.

### 6.7 `SearchSessions` returned nothing, ever — FIXED 2026-07-22

- **Evidence:** `desktop/sessions.go`'s `SearchSessions` joined `messages_fts` (FTS5) with `messages`/`sessions` in one query and called `snippet(messages_fts, ...)` in the same statement. That shape returns SQL error `"unable to use function snippet in the requested context"` from `modernc.org/sqlite` every time — and `SearchSessions` swallows the error (`if err != nil { return out }`), so the function always silently returned zero results instead of surfacing a failure. That silent swallow is why the bug was invisible outside the failing test.
- **Root cause (narrowed from the original record):** not the whole join shape — specifically `snippet()` + `GROUP BY` in one statement. Probing each variant against `modernc.org/sqlite v1.54.0` showed the same query passes with either `snippet()` or `GROUP BY` removed, and a plain subquery doesn't help (the planner flattens it back into the failing shape).
- **Fix applied:** `snippet()` moved into a `WITH f AS MATERIALIZED (...)` CTE, which the planner may not flatten; the outer query keeps the joins, `GROUP BY s.id` dedupe, and ordering. Single query — the originally proposed two-step rewrite wasn't needed.
- **Confidence:** `Direct` — [`TestSearchSessionsFindsMatch`](desktop/sessions_test.go) now passes (it verified both a hit and a miss), full `desktop` suite green.

### 6.8 Frontend layout bugs — FIXED 2026-07-22

Three distinct issues surfaced while verifying the Z-order fix (§6.6) visually, all in `desktop/frontend/src/`:

- **Blank-state text clipped instead of wrapping** — [style.css](desktop/frontend/src/style.css) `.insp-blank-title`/`.insp-blank-sub` sat in a `align-items:center` flex column with no `max-width`, so they sized to their own text's natural width instead of the pane's — at a narrow pane width the text overflowed and was hard-clipped by the ancestor's `overflow:hidden` instead of wrapping. **Fixed:** added `max-width:100%` + `text-align:center` + container padding.
- **Resize drag could get stuck and grow a panel forever** — dragging the sidebar/inspector resize handle across the native WebView2 browser window (§6.6) let the OS deliver the drag-ending `pointerup` to that separate native window instead of the DOM, so the drag state never cleared and any later mouse movement kept growing the panel. **Fixed:** `setPointerCapture` on the handle at drag start ([App.svelte](desktop/frontend/src/App.svelte)), with `pointercancel`/`blur` as backstops.
- **Panel width had no maximum at all** — `clampSize` was `Math.max(min, px)`, deliberately unbounded per a prior code comment ("neither side panel can squeeze [main] into the overlap bug from before"). Confirmed by the user that unbounded dragging itself is undesirable (pushes chat content off-screen, `.app`'s `overflow-x:auto` masking it as a horizontal scrollbar rather than a visible error). **Fixed:** max is now computed at drag time as `window.innerWidth − (other panel's current width) − 360 (main's own grid floor) − 12 (two 6px handles)` — keeps main's floor guarantee intact (the thing the original unbounded design was protecting) while capping runaway growth.
- **Confidence:** `Direct` — `svelte-check`: 0 errors after each change; visually confirmed against the user's screenshots for the first and third.

### 6.9 README.md/AETOX.md advertised Anthropic support that didn't exist; still omit a real provider (Z.AI) that does

- **Status: Anthropic half fixed 2026-07-22.** `internal/provider/catalog.go` now registers `anthropic` (aliases `anthropic`/`claude`, `ANTHROPIC_API_KEY`) and `internal/model/anthropic.go` implements a real client against Anthropic's Messages API — its own file, not a reuse of `openai_compatible.go`, since Anthropic's wire format differs on every axis that matters (`system` is a top-level field not a message, `x-api-key`/`anthropic-version` headers instead of `Authorization: Bearer`, content-block request/response shape instead of a flat string, different SSE event types for streaming). Wired through the existing `Provider`/`StreamingProvider`/`ReasoningProvider` interfaces via `internal/model/factory.go`, so `BootstrapProvider`'s fallback-to-noop path (still described below) no longer triggers for `--model-provider anthropic`. The desktop GUI's provider picker (`desktop/app.go`'s `desktopProviders` allowlist) was updated in the same pass — the engine catalog alone wasn't enough to surface it in the Settings dropdown.
- **What's still open:** `zai` (Z.AI / GLM) is fully registered and implemented but is **still not mentioned anywhere** in `README.md` or `AETOX.md`. Same category of issue as the Anthropic gap was — a real provider missing from user-facing docs — just not yet addressed.
- **`BootstrapProvider`'s silent-fallback behavior is unchanged** — `internal/model/bootstrap.go:21-54` still catches any `NewProvider` error (e.g. a genuinely unknown/mistyped `--model-provider` value) and falls back to the `noop` stub with only a non-fatal warning string, no hard error. This was flagged independently of the Anthropic question and is still true; worth a second look regardless.
- **Confidence:** `Direct` — Anthropic fix verified by `go build ./...`, `go vet ./...`, and `go test ./internal/...` all passing, plus a direct read of the new client, factory wiring, and catalog entry.

### 6.10 Permission per-tool (pattern-based) + skill auto-discovery — FIXED 2026-07-22

- **Original gap:** per `MCP-SUPPORT-PLAN.md` and `docs/architecture-reference-opencode.md` §5, Aetox only had a 3-tier `ApprovalMode` (ask/unsafe-only/full-access) with no per-tool pattern override, and no scan of `~/.agents/skills/`/`~/.claude/skills/` for externally authored skills — two of the four gaps opencode's own comparison doc named.
- **Fix applied — permissions:** [internal/safety/safety.go](internal/safety/safety.go) adds `PermissionConfig`/`PermissionRule` (`Tool`+`Pattern` glob match, `Action` allow/ask/deny, last-match-wins — opencode's own semantics). `turn.Executor.resolveApproval` (new, [internal/turn/executor.go](internal/turn/executor.go)) checks it before falling back to `ApprovalMode`/`ShouldPrompt`, replacing the `if safety.ShouldPrompt(...)` gate at all 3 call sites that used to call it directly. A matched `deny`/`allow` rule skips the prompt entirely (including under `full-access`, where `ask` now still forces a prompt). Persisted via `config.LoadPermissions`/`SavePermissions` (`~/.config/aetox/permissions.json`, same shape as `model-preference.json`) and threaded through `internal/app.Options.Permissions` into all 4 `turn.NewExecutor` call sites, and wired at both bootstrap points (`cmd/aetox/main.go`, `desktop/app.go`'s `bootstrapFromConfig`).
- **Fix applied — auto-discovery:** [internal/skill/discovery.go](internal/skill/discovery.go) adds `DiscoverSkills`/`RegisterDiscovered`, scanning `~/.agents/skills/` then `~/.claude/skills/` (`DefaultDiscoveryPaths`, opencode's own scan order) for `<dir>/*/SKILL.md`, parsing `name`/`description` frontmatter + a markdown body into a `markdownSkill` (`skill.Tool` — invoking it just returns the body as tool output, the model follows the instructions itself, same shape as opencode/Claude Code skills). Registered as `SourceExternal`; a name collision with a built-in is reported and skipped, not fatal (mirrors the existing `extraSkills` collision handling). Wired into both `cmd/aetox/main.go` and `desktop/app.go`'s bootstrap.
- **Still open:** `plugin_install`'s downloaded bundles ([internal/skill/github_tools.go](internal/skill/github_tools.go), §6.5) are only picked back up by the new discovery scan if the bundle happens to contain a `SKILL.md` — the manifest format doesn't guarantee one, so §6.5's loader gap is only partially closed by this. No Settings UI exists yet to edit `permissions.json` rules (JSON-only for now). MCP itself is still not built — see `MCP-SUPPORT-PLAN.md`, which this closes 2 of the 4 originally-named gaps for (auto-discovery, permission pattern); MCP client and plugin hook system remain.
- **Confidence:** `Direct` — `go build ./...`/`go vet ./...` clean in both modules, new tests in `internal/safety/safety_test.go`, `internal/turn/executor_test.go`, `internal/config/config_test.go`, `internal/skill/discovery_test.go` all pass; full suite otherwise unchanged (`desktop`'s pre-existing `TestSearchSessionsFindsMatch` failure, §6.7, is unrelated and still open).

---

## 7. Open Questions

- Are `internal/model/provider_catalog.go` and `internal/provider/catalog.go` duplicates, or does one wrap the other? (§6.3) — affects how cleanly `providers/` can absorb both during migration.
- Is `desktop/browser.go`'s direct Win32 syscall usage guarded by a build tag for non-Windows, or is desktop Windows-only by design? Not confirmed either way.
- ~~`go.work.sum`: commit it or gitignore it?~~ — moot since §28 deleted the workspace; there is one module and no `go.work`.

## 8. Risks

- ~~**Migration drift risk:** `engine/providers/cli` scaffolds exist with zero code~~ — removed at the source (§28): the scaffolds are deleted, so there is no stale dependency graph left to drift. The `internal/model` → `internal/provider` coupling that motivated the split is still open (§6.3).
- **MCP readiness risk:** per `MCP-SUPPORT-PLAN.md`, adding an MCP client before resolving §6.4 (registry core/user-added split) means third-party tool code would run under the same 3-tier safety model designed for trusted, self-written built-ins.

## 9. AI Agent Notes

- **Documentation discipline (owner-set, 2026-07-22):** this repo's architecture documentation follows the `senior-architect-agent` skill's discipline — evidence-first (`Direct`/`Inferred`/`Proposed`, `Verify first: Yes`), describe-then-judge findings (evidence + impact + severity + confidence, never bare style opinions), and numbered Decision sections for new design before implementation. This was a deliberate choice, not a retrofit: §§2–10 of this file already matched the skill's Full Mode template set (overview/boundary/module-map/workflow/debt-register/open-questions/risks/agent-notes/decision-record) before the skill was ever invoked — confirmed 2026-07-22. Module-level `README.md` files (§12) are the skill's "file responsibility map," kept plain/descriptive rather than evidence-tagged line-by-line — tagging is for claims under dispute (debt, risk, inferred behavior), not for code the writer read directly.
- **Documentation rule (owner-set, 2026-07-22):** docs live with their module, not as loose root files. A change that meaningfully alters a Tier-1 module (§12) updates that module's `README.md` in the same commit; if it changes the architecture picture, update the relevant ARCHITECTURE.md section too. New design discussions become numbered `Decision` sections here (§10/§11/§12 style) before implementation — do not create new standalone `.md` files at the repo root.
- Start reading at `cmd/aetox/main.go` (CLI) or `desktop/app.go:bootstrapFromConfig` (Desktop) — both converge on the same `internal/app.NewApp` + `cognitive.Agent` + `turn.Executor` wiring. Each Tier-1 module's `README.md` (§4 Docs column) is the fast map of its seams.
- **One Go module, no workspace** (since §28). All running code is `internal/`, `cmd/aetox` and `desktop/`. If you ever re-introduce a `go.work`, it must list every module whose directory tree you run workspace-aware commands (`go mod tidy`, `wails dev`) from, *including* the root module — Go activates workspace mode for any subdirectory under a `go.work` ancestor regardless of intent, and forgetting this is what broke `wails dev` in 2026-07-21.
- For skill/tool changes, `internal/skill/dispatcher.go` and `skill.go`'s `Tool` interface are the seam — already MCP-shaped per `MCP-SUPPORT-PLAN.md`.
- Per-module test status (what's covered, what's structurally untestable and why) lives in `TEST-REPORT.md`, organized by the same 5-layer reading grouped above — don't re-derive it from scratch, read it first.
- `desktop/browser.go`'s `postMessage` security model (threat model, the 3-layer defense, residual risk) is documented separately in `docs/architecture/browser-security-2026-07-21.md` — read it before touching `onMessage`/`metaScript`/`textScript`.
- Two deep-dive docs expand the layers with the most independent complexity — read them before making non-trivial changes in either: `docs/architecture/model-control-layer-2026-07-22.md` (layer 2: `skill`+`cognitive`+`turn`, the exact 4-phase turn pipeline, the safety-gate chokepoint, where MCP plugs in) and `docs/architecture/desktop-app-2026-07-22.md` (layer 5: `desktop/`, the Svelte↔native-window bridge, known issues).
- `internal/skill/image_ocr.go` shells out to a `tesseract` binary the user doesn't install manually — on Windows it's silently downloaded and installed by the NSIS installer itself (checksummed, install-time, not vendored in git); on macOS/Linux (no packaging pipeline exists for either yet) `image_ocr.go` itself does a lightweight runtime fallback instead (auto `brew install` on macOS, a copy-paste `apt`/`dnf`/`pacman` hint on Linux). Full mechanism + pinned-version bump instructions: `docs/architecture/tesseract-ocr-bundling-2026-07-22.md`.

---

## 10. Decision — Agent Orchestrator Layer (Proposed, approved 2026-07-21)

Discussed with the project owner: CLI, Desktop, and future UI front ends currently each embed the engine as a Go library in their own process (§2, §4) — there is no gateway process and no shared live agent/session state across front ends. The owner asked whether a background layer for multi-agent orchestration should exist, and whether front ends should share one running agent.

**Decision:** keep the embedded-library model (no new standalone gateway process — rejected as premature; no front end today needs to observe another front end's live session). Add a new orchestrator responsible for multi-agent lifecycle, built so it can be wrapped by local RPC later without a rewrite if that need becomes concrete.

```mermaid
flowchart TB
    subgraph today["Today — proposed, not built"]
        CLIp["CLI process"] --> Orch1["orchestrator\n(in-process)"]
        Deskp["Desktop process"] --> Orch2["orchestrator\n(in-process)"]
        Orch1 --> A1["cognitive.Agent ×N"]
        Orch2 --> A2["cognitive.Agent ×N"]
    end
```

- **What it is:** a new package, [internal/orchestrator/orchestrator.go](internal/orchestrator/orchestrator.go) (moves to `engine/orchestrator` once the module split migrates), that spawns/tracks/stops multiple `cognitive.Agent` instances per process via `Spawn`/`Get`/`Stop`/`List`, replacing the current one-`Agent`-per-front-end model. Distinct from `internal/app` (§6.1), which wires exactly one agent today. Not yet wired into `cmd/aetox` or `desktop/app.go` — those still construct a single `cognitive.Agent` directly via `bootstrapFromConfig`.
- **Interface constraint (the point of the decision):** operates on agent **IDs and a serializable `Info` snapshot** (`ID`, `Model`, `CreatedAt`), not Go closures or channels owned by a specific front end — so a future local-RPC wrapper (front end as thin client, Desktop or a dedicated process as host) can sit on top without redesigning the orchestrator itself.
- **Explicitly deferred, not built:** any IPC/RPC transport, any "Desktop as host for CLI" wiring, any shared-state protocol, and wiring into the existing front ends. Build only when a front end has a concrete requirement to observe or drive another front end's session, or to run more than one agent concurrently.
- **Status:** `Direct` (package exists, `go test ./internal/orchestrator/...` passes) but **unused** — no caller yet. Not to be confused with existing `internal/cognitive.Agent` (single-agent, already built and wired) or the roadmap "MAIN + sub-agent" description in `AETOX.md`, which this scaffolds toward but does not yet implement (no sub-agent spawning logic, no profile/tool-filtering, no MAIN-vs-sub-agent role distinction).

**Naming — settled, don't rename without checking this first:**

| Rejected name | Why it was rejected |
|---|---|
| `gateway` | Implies a network/process boundary (something external connects *into*) — there is none yet (§10 decision explicitly defers IPC/RPC). If a local-RPC layer is ever built in front of this package, `gateway` belongs to *that* layer, not to the in-process tracker. |
| `router` | Already claimed in `README.md`'s architecture diagram — "Multi-Provider Orchestration: **Router** \| Comparator \| Consensus" means routing a request to which *provider/model*, an unrelated concept. Reusing it for agent lifecycle tracking would collide with that already-planned component. |

`orchestrator` was kept because (a) it doesn't collide with any name already used in `README.md`/`AETOX.md`, and (b) `AETOX.md`'s own "Multi-Agent Layer" description ("MAIN → spawn sub-agent") already implies exactly this responsibility — lifecycle, not routing or transport.

---

## 11. Decision — Prompt/Context Layer (Proposed, being settled section-by-section, 2026-07-22)

Discussed with the project owner, who asked whether it is time to build the layer that manages system prompts and per-project context files (`AETOX.md`), and requested the design be settled here in ARCHITECTURE.md piece-by-piece rather than approved wholesale. Findings that motivated it (all `Direct`):

- The system prompt is a hardcoded string duplicated in **three places**: `cmd/aetox/main.go:buildSystemPrompt`, `desktop/app.go:buildSystemPrompt` (95% identical, one sentence differs), and a fallback in `internal/cognitive/agent.go:NewAgent`.
- **The desktop `GovernanceLoaded` badge lies.** `desktop/app.go:projectStatus` only `os.Stat`s `Aetox.md` — no code anywhere reads its contents into the prompt. A user who writes rules in `Aetox.md` sees "loaded ✓" while the model never sees a byte of it. The CLI ignores the file entirely.
- There is no user-level (cross-project) instruction file at all.
- Adjacent parts that are **fine and out of scope**: model management (`config.ModelPreference`, already centralized, CLI/desktop share one preference file) and conversation memory/truncation (`internal/memory.Context`).

**Where it sits:** one new engine-side package, `internal/prompt` (moves to `engine/prompt` when the §4 module split migrates). Both front ends call it from their existing bootstrap paths (`cmd/aetox/main.go`, `desktop/app.go:bootstrapFromConfig`), deleting both `buildSystemPrompt` copies. In the 5-layer reader's map this feeds layer 2 (what the model sees) but is a leaf package — no new dependencies, no new process, no front-end code beyond the call site.

**Assembly order (settled 2026-07-22):** four layers concatenated, most-specific last, because models weight later context higher on conflict — so project rules beat personal rules:

| # | Layer | Source | Notes |
|---|---|---|---|
| 1 | Identity | hardcoded in Go, `surface` param (`"cli"`/`"desktop"`) | the only per-surface difference today is one sentence |
| 2 | Environment | sandbox root (+ existing don't-leak-path rule) | |
| 3 | User global | every `*.md` file in `<UserConfigDir>/aetox/identity/` | new capability; cross-project personal rules — a directory, not one file, see 2026-07-23 addendum below |
| 4 | Project | `<root>/AETOX.md`, fallback `<root>/AGENTS.md` | wins on conflict |

Missing files are skipped silently; per-file size cap (~16KB) to keep the prompt bounded. Content format is free markdown — no schema, the model reads it as-is.

**Project file naming (settled 2026-07-22):** `AETOX.md` uppercase — matches this repo's own root file. Falls back to `AGENTS.md` because the ecosystem is converging on it (OpenCode, Codex, Gemini CLI), so any repo that already has one works with Aetox without creating a new file. The desktop badge should switch from stat-ing the file to reporting what the prompt layer actually loaded, making it honest.

**Reload timing (settled 2026-07-22, owner: "ตอนรีสตาร์ท หรือตอนเริ่ม ... หลายๆที่ทำก็แบบนั้น"):** **Option A, bootstrap-only.** The file is read where the agent is created — app start, project switch, model/provider switch. Editing `AETOX.md` mid-session has no effect until one of those happens. Matches convention elsewhere; zero new mechanism (no context-reset seam, no per-turn stat cost). An upgrade path to per-turn mtime-checking (previously sketched as "Option B") is still possible later without an API change, if it's ever needed — not built now (YAGNI).

**Explicitly deferred, not part of this layer:** per-turn dynamic context (git status / open files injected every turn, OpenCode-style), and sub-agent profile files (`.aetox/agents/*.md`) — the latter waits until the orchestrator (§10) has a real caller; per `docs/opencode-study/agents.md`, a profile is just model override + prompt override + permission ruleset (+ `steps`), so it can be layered on top of this package later without redesign.

**Related cleanup (done 2026-07-22):** the tool loop is now unbounded engine-wide (OpenCode-style; brakes = approval layer + ctx cancel — CLI Ctrl+C, desktop Stop button → `App.CancelTurn`). The CLI's leftover `defaultAgentMaxToolCalls = 16` was removed — it was only applied at startup and silently lost on `/model` switch anyway. Details: `docs/architecture/model-control-layer-2026-07-22.md` §3.

**Status:** `Approved & done 2026-07-22.` [internal/prompt](internal/prompt/README.md) built and wired into both front ends: both copies of `buildSystemPrompt` (`cmd/aetox/main.go`, `desktop/app.go`) deleted, both now call `prompt.Build`/`prompt.BuildWithReport`. `desktop/app.go`'s `projectStatus` no longer stats a hardcoded `Aetox.md` — it calls `prompt.ProjectContextFile`, so the `GovernanceLoaded` badge now reports the same file the prompt layer actually loaded (the "badge lies" gap this section opened with is closed). New: `config.UserGlobalContextPath()` (same `<UserConfigDir>/aetox/*` pattern as `PreferencePath`/`PermissionsPath`). Tests: `internal/prompt/prompt_test.go` (layering, fallback priority, size cap). **Incidental fix while wiring the CLI call site:** the CLI's leftover `defaultAgentMaxToolCalls = 16` (only applied at startup, silently lost on `/model` switch) was removed in the same pass as the "related cleanup" note above — see `docs/architecture/model-control-layer-2026-07-22.md` §3.

**Addendum 2026-07-23 — user global layer became a directory, not one file (owner: "ตัวตน เอไอ หมายถึง พวกไฟล์ root ที่ติดตัว เอไอ ไปตลอดอ่ะ ไฟล์ context.md ไฟล์ สกิล ไฟล์ พวกนี้อ่ะ").** The single `AETOX.md` blob from row 3 above didn't match the owner's mental model — they picture the "AI Identity" layer as a small set of always-attached root files (`context.md`, a skills note, etc.), not one text box. Changed: `config.IdentityDir()` (`<UserConfigDir>/aetox/identity/`) replaces `UserGlobalContextPath` as the layer 3 source; `prompt.BuildWithReport` now folds in every `*.md` file found there (sorted by filename, same per-file 16KB cap), each as its own `Personal instructions — <name>` block, most-specific-last ordering unaffected since this whole layer still sits before the project layer. `Loaded.UserGlobalPath` (single string) became `Loaded.UserGlobalPaths` (`[]string`) — not consumed by any caller outside `internal/prompt` yet, so this was a safe rename. `desktop/app.go` gained `ListIdentityFiles`/`ReadIdentityFile`/`SaveIdentityFile`/`DeleteIdentityFile` (replacing the short-lived single-file `ReadGlobalInstructions`/`SaveGlobalInstructions` pair from the same day), with a one-time migration: the old `AETOX.md` at `DataRoot` is moved into `identity/context.md` the first time the identity directory is touched. `UserGlobalContextPath` itself is kept only as that migration's source path, not for new code. Desktop sidebar's "ตัวตน AI" section is now a small file list (open/create/delete) over a single-file editor — see `desktop/frontend/src/lib/identity.svelte.ts`.

**Addendum 2026-07-24 — identity-layer conventions (settled in discussion with owner):**

- **Suggested identity files (convention, not schema):** `identity.md` (who the AI is), `thinking.md` (thinking discipline), `context.md` (about the user), `skills.md` (always-on notes). Surfaced as one-click templates in the desktop sidebar (`identityTemplates` in `identity.svelte.ts`) shown for any not-yet-created name. The engine still treats every `*.md` in the identity dir identically — **zero magic filenames**; the differentiation is UI nudge only.
- **`thinking.md` is discipline, not steps — and goes to every model.** Owner initially proposed skipping it for models without native reasoning; inverted after discussion: non-reasoning models benefit *most* from prose thinking discipline (that's what CoT prompting is), while step-by-step "think like this" instructions are what provider guidance (DeepSeek R1, OpenAI o-series) warns can degrade native-reasoning models. So the template is values-style (evidence-first, don't guess, say when unsure), safe for all models. Upgrade path if a specific model measurably degrades: per-file frontmatter condition (e.g. `when: reasoning|no-reasoning`), declared in the file — never hardcoded filenames in Go.
- **Guiding rule for what goes where:** habit/style → prompt (identity file); hard prohibition → code (`internal/safety` permission layer, prose can't enforce); thinking *effort* → `think.Level` (already a real API knob via `model.ReasoningConfig`, not prose).
- **Project fallback chain extended:** `AETOX.md` → `AGENTS.md` → `CLAUDE.md` (Claude Code compat — same drop-in motivation as `AGENTS.md`, mirrors OpenCode reading Claude Code's files).
- **Agent profile files (`agents/*.md`) remain deferred** per this section's original deferral — owner confirmed they are a separate topic and will never be merged into `AETOX.md`; format to be settled when `internal/orchestrator` (§10) gets a real caller.

---

## 12. Decision — Per-Module Documentation, Hub-and-Spoke (Proposed 2026-07-22)

Owner proposal: each module owns its documentation inside its own folder (complex modules first), ARCHITECTURE.md stays the central overview/hub, and every module doc links back here (and the hub's §4 module map links out). This section is the survey of how many modules that actually means, so it can be approved with real numbers instead of "น่าจะเยอะ".

**Survey (2026-07-22, non-test LOC):** 19 modules total — 17 `internal/` packages + `desktop/` + `cmd/aetox`. Tiered by what documentation they actually warrant:

| Tier | Rule | Modules (LOC) | Action |
|---|---|---|---|
| **1 — complex, gets a `README.md` in its folder** | >1000 LOC or many files or a real subsystem | `internal/model` (3852, 10 files), `internal/skill` (3391, 18 files), `internal/turn` (3367), `desktop/` (3345 Go + Svelte frontend), `internal/app` (1440), `internal/grammar` (1032 — **single file, zero docs today**), `cmd/aetox` (1010) | **7 READMEs to write.** `desktop/README.md` exists but is Wails template boilerplate — replace, don't append. |
| **2 — medium, hub row + package doc comment** | 300–800 LOC, already covered elsewhere or too simple for a file | `internal/provider` (771), `internal/mcp` (681), `internal/config` (606), `internal/cognitive` (517 — already covered by `docs/architecture/model-control-layer-2026-07-22.md` + ADR 0002), `internal/safety` (481 — same doc), `internal/command` (298) | No new files until one grows. |
| **3 — small, doc comment at top of file only** | <300 LOC, single concern | `internal/audit` (283), `internal/memory` (194), `internal/think` (166), `internal/orchestrator` (131 — §10 covers the design), `internal/plan` (106), `internal/debuglog` (100) | Nothing to do beyond existing comments. |

**Conventions (proposed):**

- A module `README.md` answers: what this package is, its key seams (the 2–3 types/functions everything else hangs off), links to its dated deep-dive docs, and one link back to `ARCHITECTURE.md` — a map, not a novel. Aim under ~80 lines each.
- **Dated deep-dive docs stay where they are** (`docs/architecture/*-2026-*.md`, `docs/adr/`) — they are session records/history, not living module docs. Module READMEs link to them; nothing gets moved, so existing links and git history stay intact.
- The hub embeds the index: §4's module map gains a **Docs** column linking each Tier-1 README (done when the READMEs land, not before — no dead links).
- New rule going forward: a change that meaningfully alters a Tier-1 module updates that module's README in the same commit.

**Suggested write order (follows active work, not size):** `internal/app` + `cmd/aetox` + `desktop/` first (the prompt layer §11 touches exactly these three), then `internal/turn`/`internal/skill`/`internal/model`, `internal/grammar` last.

**Addendum (2026-07-22):** `internal/prompt` was added after this survey (§11). By LOC alone (~110 lines) it would be Tier 3 (doc comment only), but it got a README anyway — a new module that implements an approved Decision section earns one regardless of size, so the design doesn't only live in ARCHITECTURE.md. Tier boundaries above are for *existing, undocumented* modules; a freshly-built module tied to a Decision section is a standing exception, not a tier-4.

**Status:** `Approved & done 2026-07-22.` All 7 Tier-1 READMEs written ([internal/app](internal/app/README.md), [cmd/aetox](cmd/aetox/README.md), [internal/turn](internal/turn/README.md), [internal/skill](internal/skill/README.md), [internal/model](internal/model/README.md), [internal/grammar](internal/grammar/README.md), [desktop](desktop/README.md) — boilerplate replaced); §4 map carries the Docs column; the update-docs-with-changes rule is recorded in §9. The 5-layer reader's map stays as the top-level reading aid — module READMEs are the finer breakdown beneath it.

---

## 13. Decision — RTK Integration (Proposed, being settled section-by-section, 2026-07-22)

Owner asked to integrate `rtk` (Rust Token Killer — the owner's own CLI output-filtering tool, already used as a Claude Code hook) into Aetox itself. This section also closes a documentation gap the owner flagged in the same conversation: the model-call path (what exactly gets sent to/from the provider each turn) was explained in chat but never written down — closing it here because RTK's insertion point *is* that exact seam.

### 13.1 The call path this plugs into (closes the doc gap)

**Moved to its permanent home:** [docs/architecture/model-control-layer-2026-07-22.md §6](docs/architecture/model-control-layer-2026-07-22.md) ("The exact call path — what a 'call' to the provider actually contains") — full diagram, `model.Request` field breakdown, and why `memory.Context.enforceLimits` is the only *other* token-budget mechanism. §7 of that same doc explains, with evidence, why the RTK hook is **not** inside `Agent.buildRequest` (a question re-raised and settled in this same 2026-07-22 session): by the time `buildRequest` runs, tool output is already a flat, un-typed message string — the tool name/args needed to pick an `rtk pipe -f <filter>` only exist earlier, at `modelToolReceipt`. A second hook at `buildRequest` would be redundant (re-filtering already-filtered text) and risk corrupting already-correct content by guessing wrong.

### 13.2 What RTK is (evidence from direct inspection, not the owner's description alone)

- Installed at `~/.cargo/bin/rtk`, v0.34.3 — a real, already-installed Rust binary, not vendored into this repo. `Direct`.
- Its own description: "a high-performance CLI proxy designed to filter and summarize system outputs before they reach your LLM context." Subcommands overlap directly with existing Aetox skills: `rtk git {status,diff,log,show}` ↔ `internal/skill/git.go`, `rtk grep` ↔ a grep-style tool, `rtk read --level {minimal,aggressive}` ↔ `internal/skill/read.go`, `rtk ls`/`rtk tree`/`rtk find` ↔ `internal/skill/list.go`/`fs.go`.
- Demonstrated on this repo (`Direct`, ran live): `rtk git status` on a clean tree returned `clean — nothing to commit` (one line) instead of porcelain output.
- Demonstrated risk (`Direct`, ran live): `rtk read internal/prompt/prompt.go --level minimal` **silently dropped every doc comment**, including the package comment and the one on `type Surface`. For a tool whose output the agent might use to *edit* a file afterward, or whose commentary explains a non-obvious constraint, this is a correctness risk, not just a cosmetic change.
- Today has no library/API mode found in `--help` output — integration means shelling out to the `rtk` binary per call, same as Aetox's skills already shell out to `git`/native commands.
- No hook currently installed for this machine's `rtk` (`rtk config` reports "No hook installed"); Aetox integrating it would be independent of whatever Claude Code hook setup exists on this machine.

### 13.3 Corrected design — the actual hook point (found only after tracing the real data path)

The owner's own framing ("สอดตอนที่ระบบสร้าง call ก่อนส่งให้ provider" — insert where the system builds the call, before sending to the provider) pointed at `internal/cognitive/agent.go`'s tool-loop as the first guess. Tracing the actual data (`Direct`, not assumed) showed that's the wrong seam: by the time `cognitive.Agent.RespondWithTools` has a tool's output, it's *already* the JSON-wrapped receipt string from `turn.executeToolCallWithOutcome` → `modelToolReceipt` (`internal/turn/executor.go:661`), which bundles `{"tool", "status", "output", "stderr", ...}`. Piping a JSON envelope through `rtk pipe -f git-status` would just fail to parse. **The real seam is one line earlier, inside `modelToolReceipt` itself** — where `result := output.RawOutput` is still a plain string, before it's wrapped into JSON and before the existing secret-redaction pass (`sanitizeAndTrimOutput`, `internal/turn/result.go:158` — redacts api key/token/password patterns, enforces `summaryLimit`).

Also discovered live (`rtk pipe -f <bogus-name>` lists its own valid filters in the error): `rtk pipe -f <filter>` needs a **named, format-aware filter** — `cargo-test, pytest, go-test, go-build, tsc, vitest, grep, rg, find, fd, git-log, git-diff, git-status, log, mypy, ruff-check, ruff-format, prettier` — it is not a blind text compactor. This confirms the §13.4 read.go concern independently: there is no filter name for "arbitrary file content," so file reads were never a fit for this mechanism at all (they'd need the separate `rtk read --level` mechanism, not attempted here).

### 13.4 Settled design

- **Hook location:** `internal/turn/executor.go:modelToolReceipt`, immediately after `result` is computed from `output.RawOutput`/`output.Content`, **before** `sanitizeAndTrimOutput` — so RTK sees genuine unredacted command output (needed to match its filter formats) but the existing secret-redaction/length-cap safety net still runs unconditionally afterward on whatever comes out, RTK-filtered or not.
- **New package:** [internal/rtk](internal/rtk/README.md) — `Available()` (cached `exec.LookPath` check), `FilterForTool(name, args)` (tool → filter-name mapping), `Filter(filter, content)` (runs `rtk pipe -f <filter>`, 5s timeout, any failure returns the original content untouched).
- **Optional, never required.** No installer bundling (unlike tesseract's precedent in `docs/architecture/tesseract-ocr-bundling-2026-07-22.md`) — a token-cost optimization doesn't earn download-on-first-use the way an OCR capability did.
- **Approval untouched.** `safety.AssessCommand`/`turn.resolveApproval` see the real command (`git status`) always — RTK only touches the result string, after execution, never what's presented for approval.
- **v1 scope, decided from evidence rather than re-asked:** `git` (status→`git-status`, diff/show→`git-diff`, log→`git-log`) and `shell`. `read.go` excluded (§13.3's mechanism mismatch, plus the demonstrated comment-stripping risk). Every other tool (`write`/`delete`/`list`/`fs`/`echo`/`time`/`help`/`input`/`output`/`github_repo_summary`/`plugin_install`/`image_ocr`) has no mapping and passes through exactly as before.
- **Failure behavior:** any error (missing binary, unknown filter, subprocess failure, empty output) falls back to the original content silently — never surfaced as a tool error, since RTK is strictly an optimization layer, not a capability.

**Status:** `Approved & done 2026-07-22`, refined `2026-07-23` (below). Full repo `go build`/`go vet`/`go test ./...` green throughout. List/fs and a wider filter set are natural v2 candidates once v1 usage confirms real savings — not built now (YAGNI).

### 13.5 Refinement (2026-07-23) — `shell` switched from a guessed prefix-list to `Rewrite`, matching rtk's own OpenCode plugin

Owner's own instruction: check how OpenCode's actual RTK integration is configured before expanding ours, and "เอาแค่คำสั่งที่จำเป็น...อย่ายัดทุกอย่างเข้า" (take only what's necessary, don't stuff everything in). Ran `rtk init -g --opencode --dry-run -v` (`Direct`, live) — it previews the exact plugin rtk would install for OpenCode:

```ts
export const RtkOpenCodePlugin: Plugin = async ({ $ }) => {
  // ...checks `which rtk`...
  return {
    "tool.execute.before": async (input, output) => {
      // ...only for tool === "bash"/"shell"...
      const result = await $`rtk rewrite ${command}`.quiet().nothrow()
      // ...substitutes args.command with the rewritten result if different...
    },
  }
}
```

The real plugin is a **single hook, single delegated call** (`rtk rewrite`) — no hand-maintained command-to-filter table at all. Aetox's v1 `shell` case (a hardcoded `strings.Contains`/`HasPrefix` guess-list: `go test`, `go build`, `grep`, `rg`, `find`, `fd`) was strictly inferior: incomplete (misses anything not on the list, e.g. `cat`→`rtk read`, confirmed live), and would silently drift out of date as rtk's own registry grows.

**Change made:** `shell.go` now calls `rtk.Rewrite(commandLine)` directly (after approval, before `exec.Command`) — if rtk has an equivalent, Aetox runs *that* instead of the original command line; same underlying side effects (rtk actually runs the real command), just captured pre-compacted. `FilterForTool`'s `shell` case was deleted entirely (dead code once `Rewrite` covers it — keeping both would have double-processed shell output). `git` was deliberately **left unchanged** — it already parses its exact subcommand (`internal/skill/git.go`'s `allowedGitReadActions`), so reconstructing a command string just to ask `Rewrite` would be strictly more roundabout than the existing direct name→filter mapping; not "everything," only where it actually simplifies something.

**Bug found and fixed during this change:** `rtk rewrite`'s own `--help` claims "exits 0 and prints the rewritten command if supported" — a live check showed a *successful* rewrite (`git status` → `rtk git status`) exiting **3**, not 0. `internal/rtk.Rewrite` originally used `exec.Cmd.Output()` (which errors on any non-zero exit, discarding stdout) and so treated every successful call as a failure. Fixed by capturing stdout unconditionally and judging success by content, not exit code — same resilient pattern `Filter` already used correctly. Caught by `TestRewriteRealBinary`, a live integration test against the real installed binary (same pattern as `TestFilterRealBinary`) — this is exactly the kind of gap a mocked test would have missed.

Updated: [internal/rtk/README.md](internal/rtk/README.md) (seam table split by which mechanism each tool uses and why), `rtk_test.go` (shell prefix-list tests replaced with `Rewrite` tests, including the live one that caught the exit-code bug). No change needed to `internal/turn/executor.go` beyond what §13.4 already describes — `FilterForTool` shrinking to git-only doesn't change its call site.

**Test coverage gap closed (2026-07-23):** `internal/rtk`'s own 9 tests cover the package's logic in isolation, but neither integration seam had a direct test — added two: `internal/turn/rtk_hook_test.go` (`modelToolReceipt` calling into `rtk.FilterForTool`/`Filter` for real, plus a regression check that an unmapped tool like `read` passes through byte-for-byte unfiltered) and a new case in `internal/skill/shell_test.go` (`shellSkill.Execute` actually substitutes the rtk-rewritten command — asserted by the *output shape* difference: rtk's compact `git status` reads `clean — nothing to commit`, plain git's doesn't, so passing proves substitution happened, not just that `Rewrite()` works standalone — and confirms the recorded `Command` field stays the original, never the rtk one). All three tests skip gracefully when `rtk` isn't installed, same pattern as `internal/rtk`'s own live tests. Full repo `go build`/`go vet`/`go test ./...` green.

### 13.6 Runtime auto-install (2026-07-23) — RTK downloads itself once if missing, no installer changes

Owner's direction: Aetox is meant to become a program other people download and run, so should `rtk` be part of the installer? Asked, then explicitly chose **runtime fallback instead of touching the NSIS installer** — see the three options put to the owner (installer bundling / runtime fallback / defer) and the choice made.

**Evidence gathered before building anything (`Direct`, all live):**
- `rtk` on this machine was installed via `cargo install --git https://github.com/rtk-ai/rtk` — a real, specific, identifiable public repo, not the owner's private/undisclosed tool.
- `gh api repos/rtk-ai/rtk` confirms: public, **72,590 stars**, license **Apache 2.0**, not archived — permissively redistributable, no licensing blocker for bundling or auto-downloading it.
- Its latest release (`v0.43.0`) publishes a portable, checksummed asset per platform: `rtk-x86_64-pc-windows-msvc.zip`, `rtk-{x86_64,aarch64}-apple-darwin.tar.gz`, `rtk-x86_64-unknown-linux-musl.tar.gz`, `rtk-aarch64-unknown-linux-gnu.tar.gz` — each with a `sha256:` digest **published directly in the GitHub release API response** (no separately-computed/pinned hash needed, unlike the Tesseract installer's case where the digest field was `null`).
- Downloaded and inspected both archive shapes directly: the Windows zip contains a single bare `rtk.exe` at its root (no subfolder); the Linux tar.gz contains a single bare `rtk` file, already executable. Both confirmed by actually unzipping/untarring them, not assumed from convention.

**Design, mirroring the Tesseract precedent's judgment (`docs/architecture/tesseract-ocr-bundling-2026-07-22.md` §3 — macOS's Homebrew auto-install, "no elevation needed, so one automatic attempt is safe") but applicable on every OS here** since rtk ships a portable archive with no installer wizard to script around, unlike Tesseract's Windows story which needed a real install-time hook:

- New file [internal/rtk/install.go](internal/rtk/install.go): `resolve()` (package-private, `sync.Once`-cached) tries, in order: (1) PATH, (2) a previously-downloaded copy at `<UserConfigDir>/aetox/bin/rtk[.exe]`, (3) one download-verify-extract attempt from the GitHub release matching `GOOS`/`GOARCH`. `Available()` now calls this instead of a bare `exec.LookPath`; `Rewrite`/`Filter` invoke the resolved path instead of a literal `"rtk"` string, so a downloaded-but-not-on-PATH binary actually gets used.
- Any failure at any step (offline, unsupported OS/arch, checksum mismatch, GitHub rate-limited) leaves `Available()` returning `false` — identical fallback behavior to rtk never having been installed at all. This is strictly additive to §13's original "optional, never required" principle, not a change to it.
- **Explicitly not done:** no change to `project.nsi`/the NSIS installer. This is the owner's deliberate scope boundary for this round — an install-time mechanism remains a live option later if the runtime fallback proves insufficient (e.g. if GitHub API rate-limiting or offline-install scenarios turn out to matter in practice), same "revisit later if needed" posture the Tesseract doc already takes for macOS/Linux packaging.

**Testing:** `internal/rtk/install_test.go` — fast, deterministic, no-network tests for the pure logic (`assetNameFor` across all 5 supported platforms + one unsupported, `parseSha256Digest`, zip/tar.gz extraction against synthetic in-memory archives) plus light real-network checks against the actual GitHub API using the small `checksums.txt` asset (838 bytes) rather than the multi-MB platform binaries, so the routine `go test ./...` sweep stays fast and skips cleanly if offline. One additional test, `TestTryAutoInstallEndToEnd`, actually downloads the full real binary, extracts it, and runs `--version` to confirm the pieces compose correctly end to end — gated behind `AETOX_TEST_RTK_E2E=1` (skipped by default) since a full multi-MB download on every test run would be disproportionate; run manually once during this work and confirmed passing.

**Status:** `Approved & done 2026-07-23.` Full repo `go build`/`go vet`/`go test ./...` green (including the gated end-to-end check, run manually and passing).

---

## 14. Decision — Unified Data Root (2026-07-23) — cleaning up where Aetox writes its own data

Owner, continuing straight from §13.6: "ทำระบบของเราให้คลีนเลยไม่ไปสร้างเครสมั่วๆ อีกได้ไหมครับ... ข้อมูลใน WebView เราจะฝังในระบบของเราเลยดีสุดปลอดภัยสุดครับ" — make the system clean, stop scattering files, and WebView2 data specifically should be embedded in Aetox's own design rather than left to an external library's default. Also raised as a general principle: things Aetox designs itself and doesn't need to share with other tools should live inside Aetox's own structure, not wherever an OS/library convention happens to put them.

### 14.1 Evidence — re-surveyed every OS-directory resolution point (`Direct`, re-grepped after §13's rtk work)

| Location found | Consistent with the others? |
|---|---|
| `internal/config`'s 5 path functions (preferences, permissions, MCP servers, `.env`, `AETOX.md`) | ✅ all `<UserConfigDir>/aetox/*`, but each one duplicated the same 6-line OS-fallback block inline |
| `desktop/db.go` (session SQLite) | ✅ same folder, but its own separate copy of the same fallback logic |
| `internal/rtk/install.go` (downloaded `rtk` binary, built in §13.6) | ✅ already correct, reused `UserGlobalContextPath`'s directory |
| `internal/audit/audit.go` (shell command audit log) | ❌ used `os.UserHomeDir()` + `.aetox` — a **second, different "Aetox home"** (`~/.aetox`) that had nothing to do with the `%AppData%\aetox` folder everything else uses. Flagged in an earlier session pass, left pending owner approval — approved and fixed in this pass. |
| `desktop/main.go`/`browser.go` (WebView2 profiles) | ❌ left empty by default → Wails/go-webview2's own silent convention (`%AppData%\<exe-name>`), which is *why* `aetox-desktop.exe`, `aetox-desktop-dev.exe`, and `aetox-browser` were three unrelated folder names for what's conceptually one thing — not a location Aetox itself ever decided on |
| `internal/skill/discovery.go` + `github_tools.go` (`~/.agents/skills`, `~/.claude/skills`) | **Deliberately not unified** — see §14.3 |

### 14.2 Design — one function, everything else derives from it

`config.DataRoot()` (new, `internal/config/config.go`): resolves an `AETOX_DATA_ROOT` env var override first, else `<UserConfigDir>/aetox` (the production default, unchanged). Every one of the ✅ and ❌ rows above except the intentional exception now calls this one function instead of repeating the OS-fallback dance or inventing its own convention:

- `PreferencePath`/`PermissionsPath`/`MCPServersPath`/`EnvFilePath`/`UserGlobalContextPath` — refactored onto `DataRoot()`, deleting 5 copies of identical fallback logic (`LegacyPreferencePath` deliberately left untouched — it represents a fixed historical location for one-time migration, not a place to redirect).
- `desktop/db.go` — same, keeping its existing `dbDir` test-seam override (a *different*, test-only mechanism from `AETOX_DATA_ROOT`, both still work together).
- `internal/rtk/install.go`'s `privateBinaryPath` — simplified to call `DataRoot()` directly instead of borrowing another function's directory.
- `internal/audit/audit.go` — **fixed**: `ShellAuditLogPath` now uses `DataRoot()`, closing the `~/.aetox` vs `%AppData%\aetox` inconsistency flagged earlier.
- `desktop/main.go`'s `webviewUserDataDir` — **now always** returns `<DataRoot()>/webview/<name>`, explicitly, every time (empty return is only a last-resort fallback if `DataRoot()` itself errors) — never Wails'/go-webview2's silent exe-name-based default again, for dev *or* production. `browser.go` already called this same helper, so it picked up the fix automatically.
- `desktop/wails-dev.bat` — the dev-only override env var is renamed from the narrower `AETOX_WEBVIEW_DATA_DIR` (introduced in §13's disk-cleanup pass, webview-only) to the general `AETOX_DATA_ROOT`, now covering *all* of Aetox's data during dev, not just WebView2. `desktop/.gitignore`'s `.webview2-data` entry renamed to `.aetox-data` to match.

This reconciles the two asks that were briefly in tension: "keep dev data off C:" (§13, still true — `wails-dev.bat` still redirects everything to a project-local folder) and "WebView data should be embedded in our own design" (this section — the *shape* of where it lives is now something Aetox explicitly decides, `<DataRoot>/webview/*`, not a location any external library chose for it; the drive it happens to sit on during dev is a separate, already-solved concern).

### 14.3 Explicit exception — skill discovery/`plugin_install` stay pointed at shared, external locations

`~/.agents/skills` and `~/.claude/skills` (`internal/skill/discovery.go`) are deliberately **not** brought under `DataRoot()`. These are ecosystem-wide conventions — the same paths OpenCode and Claude Code use — and the entire value of scanning them is that *other tools'* skills/plugins get auto-discovered, and anything `plugin_install` writes into `~/.agents/skills` becomes visible to those other tools too. "Don't share what doesn't need to be shared" (the owner's own stated principle) cuts the other way here: this is a location that specifically *needs* to stay shared to keep working. Unifying it under Aetox's own private folder would quietly break interop for no benefit.

### 14.4 Test-isolation bug found and fixed along the way

`internal/audit/audit_test.go` isolated itself by overriding `HOME`/`USERPROFILE` — which redirected the *old* `os.UserHomeDir()`-based path, but `DataRoot()` resolves via `os.UserConfigDir()` (`%AppData%` on Windows), which those env vars don't affect. Running the suite after the refactor made this obvious immediately: tests started reading/writing the real `%AppData%\aetox\shell-audit.log` on this machine, accumulating real entries across runs (`want 2 got 5`-style failures). Fixed by isolating via `AETOX_DATA_ROOT` instead — dogfooding the same override mechanism `wails-dev.bat` uses, rather than the audit package inventing its own isolation trick.

Also found while fixing it: `internal/skill/shell_test.go`'s tests call `shellSkill.Execute`, which unconditionally calls `audit.WriteShell` — and had **never** isolated the audit log at all, on any prior session. Every `go test ./...` run before this fix was silently appending real entries to the real, machine-wide audit log. Fixed the same way (`isolateAuditLog` helper, `AETOX_DATA_ROOT`). The real log file this had already polluted was deleted by hand as part of this cleanup.

### 14.5 Deferred, not designed now — easier installation (npm or otherwise)

Owner mentioned wanting Aetox installable via `npm` or similar "someday," explicitly framed as a future direction, not a current ask. Noted here so it isn't lost, not designed: Aetox is a Go+Wails binary, not a Node package, so "npm install" would mean a thin wrapper package (the pattern several Go/Rust CLI tools use — an npm package whose `postinstall` fetches the real platform binary, similar in spirit to how `internal/rtk/install.go` already fetches `rtk`'s binary). Revisit when actual distribution work starts; not blocking anything today.

**Status:** `Approved & done 2026-07-23.` Full repo `go build`/`go vet`/`go test ./...` green.

---

## 15. Decision — Coding Loop Tools: `edit` + `grep` (2026-07-23)

Owner brought external feedback proposing three workstreams (browser mastery, total tool command, full coding capability). Checked against the actual code before building anything:

### 15.1 Evidence — most of the feedback was already built or already decided

| Proposed | Reality |
|---|---|
| "Tool chaining" | Already exists — `cognitive.Agent.RespondWithTools` is the model-driven multi-tool loop (§10/§3 of the model-control doc). |
| "Self-healing execution" | Not a feature — `turn.modelToolReceipt` already returns `stderr`/`success` to the model; retrying is the model reading the receipt. No code needed. |
| "Max-retry guard against infinite loops" | Already decided — `AgentConfig.MaxToolCalls` is an opt-in cap; the deliberate default is unbounded with permission gate + ctx cancel as the brakes (comment at `cognitive/agent.go:71`). Not re-litigated. |
| "Coding loop: Explore→Read→Edit→Verify" | Read (`read`), Verify (`shell`) existed. **Explore and Edit were the real gaps** — no content search, and `write` (whole-file overwrite) was the only mutation tool. |
| "Browser control" | Skeleton already exists (`desktop/workbench.go` `browser_open/read/click/type`, `SourceExternal`). Deeper design deferred until the coding loop is proven. |

### 15.2 Design — two new built-ins, nothing else

- **`edit`** (`internal/skill/edit.go`) — exact search & replace: model sends `path`/`old_string`/`new_string`; Go verifies `old_string` matches **exactly once** (0 → "not found, re-read the file"; >1 → "add surrounding lines"), then replaces. **Rejected the feedback's suggestion of LLM-generated unified diffs + `go-diff`**: models drift on line numbers, and uniqueness-checked literal replace is both safer and dependency-free (`strings.Count` + `strings.Replace`). `ExecuteTool` deliberately bypasses `stringSlice` (it trims/drops empties, which corrupts whitespace-significant match strings). Classified `RiskHigh`/`EffectWriteWorkspace` in `safety.go` — critical because *unknown* skill names default to `RiskLow`/no-effects there, which would have made `edit` an unprompted file writer.
- **`grep`** (`internal/skill/grep.go`) — stdlib `regexp` + `filepath.WalkDir` content search returning `path:line:text`, capped (200 results, 1 MB/file, skips dot-dirs and binaries). **Rejected shelling out to ripgrep**: not guaranteed installed, and the stdlib walk is ~100 lines with no dependency. `RiskLow`/`EffectReadWorkspace`.

Both registered in `defaults.go`, path-arg extraction added to `turn.toolCallToArgs` so permission patterns (`{Tool: "edit", Pattern: "docs/*"}`) can match. Tests follow the existing per-skill pattern; the full suite also caught that `stringSlice` trimming issue before it shipped.

**Status:** `Done 2026-07-23.` `go build`/`go test ./...` green. Next in this thread when owner resumes it: browser-control design (§15.1 last row) as its own Decision section.

---

## 16. Decision — Dead-Code Sweep (2026-07-23)

Ran `golang.org/x/tools/cmd/deadcode ./...` (trustworthy here: Wails-bound desktop methods register as reachable). 47 findings, triaged three ways:

**Deleted (~430 lines):** CLI-era and module-split leftovers — `model/provider_catalog.go` 4 delegate funcs (note the trap: dead `ModelChoicesWithEndpointAndAPIKEY` vs live `...APIKey`), `DefaultThinkingLevel` + `BuildCapabilityCatalog`/`ModelCapability`, `provider.CapabilitiesFor`/`EnvKeys`, duplicated `SlashMetaLegend`/`SlashSuggestions` in both `command` and `grammar` (+ `grammar.New`), `turn/infer.go`'s regex-inference helpers superseded by the model-driven loop (`inferToolFromConversation`, `parseWriteIntent`, `parseListPath`, `applyExtensionHint`, `splitIntentParts`), `turn.executeToolCall`, `app.go`'s `dispatchBySkill`/`showSkillPalette`/`printAvailableSkills`, `config.SavePermissions` (its round-trip test rewritten to cover the live `LoadPermissions` directly), `safety.ApprovalModeFromLegacy`, `debuglog.IsEnabled`/`LogDir`. Tests referencing deleted funcs removed with them; two tests that actually pinned live code were kept and repointed instead of deleted.

**Deliberately kept despite being flagged:** `internal/orchestrator` (§10's unwired layer — the future task/subagent tool) and `rtk.Available` (live test-infrastructure: skip-guard in `shell_test.go`/`rtk_hook_test.go`; deadcode doesn't count test binaries).

**Pending manual delete (session tooling can't remove files):** `internal/plan/` (whole package, zero callers), `internal/model/openrouter.go` + `_test.go` (duplicate of the live OpenRouter spec in `internal/provider`), `internal/turn/record.go` (aspirational audit-record type, never constructed anywhere).

**Status:** `Done 2026-07-23` except the four files above. `go build`/`go vet`/`go test ./...` green; `deadcode` re-run clean apart from the kept/pending items.

---

## 17. Decision — Kill the Regex Intent Layer (Proposed 2026-07-23)

**Trigger (real failure, 2026-07-23):** user asked `ornith:9b` "คุณทำอะไรได้อีก เอาเนื้อหาในเว็บมา ทำเป็นไฟล์ html ให้ผมได้ไหม". Phase 1's regex (`internal/turn/infer.go`, `reWriteIntent`) matched the bare verb "ทำ", hijacked the turn into a direct `write` with a garbage path, and the model never saw the message. The per-regex patch shipped that day treats one symptom; the architecture invites the next one.

**The debt, mapped (audit 2026-07-23):** `turn.Executor.Execute` runs *four* phases, three of which second-guess the model with NL keyword regexes:
- Phase 1 (`executor.go:178`, `shouldExecuteInferredBeforeAgent` :255): regex candidates for write/read/delete/github/plugin_install execute **before the model is ever called**.
- Phase 2 (:190): the real model-driven `RespondWithTools` loop — but if the model doesn't call a tool, or the tool fails, it **silently re-enters the regex loop** (:196-209).
- Phase 3 (:213): regex-only path for non-tool agents.
- `infer.go` is 746 lines / 22 NL regexes (Thai + English), the only NL-inference file left in the repo (§16's sweep already deleted its dead half).
- Compounding it: `ollama.SupportsToolCalling()` is hardcoded `true` (`internal/model/ollama.go:56`) — never checked against the actual model, so weak local models "pass" the Phase 2 gate, fail to tool-call, and land in the regex layer anyway.

**Reference points:** Claude Code does zero NL inference — pre-model parsing is explicit syntax only (`/slash`, `!bash`, `@file`); everything else goes to the model with tool definitions and the model decides every call. A weak model's failure mode is a plain text answer, never a hijacked tool. OpenCode is the same shape: a per-model capability catalog declares `tool_call` true/false; non-tool models simply get no tools (chat-only). Neither has a keyword layer, in any language.

**Decision:**
1. **Delete the layer.** `internal/turn/infer.go` entirely; executor Phases 1 and 3; both silent regex fallbacks inside Phase 2; `shouldExecuteInferredBeforeAgent` / `shouldUseInferredToolPath` / `executeInferredToolCandidatesLoop` / `executeInferredTool` and the inferred-path test suites (executor_test.go is 1,387 lines mostly pinning this layer). Net ≈ −2,000 lines.
2. **Pipeline becomes CC-shaped:** `grammar.Parse` (kept — explicit slash/meta/known-command tokens, the analog of CC's slash commands) → explicit `KindSkill` command → `executeSkillTurn` direct dispatch (kept — analog of `!`) → everything else: model tool loop when the provider truly supports tools, else plain streaming chat. The model's answer is final; nothing re-guesses it.
3. **Make Ollama capability honest:** replace the hardcoded `true` with a bootstrap probe of Ollama's `/api/show` capabilities list (`"tools"`, Ollama ≥ 0.4), plus a runtime backstop — on the API's "does not support tools" error, mark the session chat-only. Surface the result in `ModelInfo` so the desktop UI can show "tools unavailable for this model" instead of pretending.
4. **Accepted regression:** non-tool-capable local models lose the "สร้างไฟล์ x.txt" magic and become chat-only — same behavior as OpenCode, and honest. The fix for a weak model is a tool-capable model, not a regex.

**Status:** `Done 2026-07-23.` Owner approved same day, with one amendment to point 3: **no preemptive capability gating** — tools are served to every model normally, no size/capability prejudgment (owner's call: "ระบบเราจะเสิร์ฟให้โมเดลตามปกติ... แต่เผื่อทางไว้ก็ดี"). The `/api/show` probe was dropped; only the runtime backstop shipped — `ollama.Complete` retries a tools request as plain chat when Ollama itself answers "does not support tools" (`internal/model/ollama.go`, pinned by `TestOllamaComplete_RetriesWithoutToolsWhenModelRejectsThem`). Demolition tally: `infer.go` (−746), `record.go` (−36, §16's pending item), `executor.go` 876→620, `executor_test.go` 1387→560 — net ≈ −2,050 lines. New §17 regression pin: `TestExecute_ConversationTextNeverTriggersToolsDirectly` (the exact Thai sentence that triggered this decision). `go build`/`go test ./...` green.

---

## 18. Decision — Browser Embedding: Never Find Your Own Window by Title (2026-07-24)

**Trigger (real failure, 2026-07-24):** every browser tab rendered blank — `file://` pages AND https — surviving app restarts, with zero errors anywhere. §6.6's z-order fix was live and correct, but no tab webview process was even being created.

**Evidence (instrumented `open()`/`start()`, read from `debuglog`):** `CreateWindowExW FAILED: Access is denied.` on every tab, and the one-line diagnosis: `parent hwnd=… parentPid=1464 selfPid=9148` — `browserHost.start()` located the main window via `FindWindowW(nil, "Aetox Desktop")`, i.e. **by title**, and matched a *foreign process's* window carrying the same text (an `explorer.exe` taskbar-thumbnail host; a dev-URL browser tab titled by our own `<title>` can collide the same way). A cross-process parent makes `WS_CHILD` creation fail with `ERROR_ACCESS_DENIED` — silently, because `open()` swallowed every failure path.

**Fix applied ([desktop/browser.go](desktop/browser.go)):**
1. `findOwnMainWindow()` — enumerate top-level windows, match by `GetWindowThreadProcessId == os.Getpid()` + visible. Title-based lookup is banned in this subsystem.
2. Every native failure path now logs to `debuglog` with tab id — the silent `return`s are what turned a one-line bug into a half-night hunt.
3. Visibility hardened while in there: `SetWindowPos(HWND_TOP)` moved into `NavigationCompletedCallback` natively (§6.6's meta-event-driven re-glue depended on page JS surviving the origin check — never true for `file://`, whose URLs have no host; `sameOrigin` now special-cases file↔file, which also restores title/URL sync for local pages).
4. Address-bar `normalizeUrl` (frontend): drive paths → `file:///`, schemes pass through — blind `https://` prefixing had produced dead `https://file:///…` URLs.

**Full write-up:** [docs/architecture/native-browser-embedding-2026-07-24.md](docs/architecture/native-browser-embedding-2026-07-24.md) — architecture, the complete failure catalog (7 entries), and the macOS (WKWebView subview) / Linux (WebKitGTK widget) port blueprint; both platforms structurally avoid the window-parenting and z-order classes entirely. Also settles §7's open question in passing: `desktop/browser.go` has no build tag — desktop is Windows-only today by construction, and the port blueprint is the plan for changing that.

**Status:** `Done 2026-07-24.` Owner verified tabs load (file:// + web). `go build`/`go test ./desktop` green (file↔file origin cases added to `TestSameOrigin`).

---

## 19. Decision — Desktop: No-Project-Focus Default, Session-Bound Workbench, Non-Nil Binding Contract (2026-07-24)

Three same-day desktop decisions, recorded together; all owner-driven, all shipped.

### 19.1 No-project-focus is the startup default

**Trigger:** the composer chip showed "desktop" — the engine silently adopted whatever cwd it was launched from (`config.go`'s `RootPath == "" → os.Getwd()`; `wails dev` runs from `desktop/`). Owner: never bind to the launch folder; focus must be an explicit choice (Claude Desktop's เลือกโปรเจกต์ picker as the reference), and unfocused chat must keep **full tool access** — "เหมือนเปิด Claude/Codex มาคุยดื้อๆ แต่ยังรันอะไรในเครื่องเราได้."

**Design ([desktop/app.go](desktop/app.go), [desktop/sessions.go](desktop/sessions.go)):** `App.projectFocused` flag; startup calls `focusNone()` — engine re-roots at the user's **home dir** (tools all work, rooted neutrally), no `touchProject`, so launch cwd never pollutes recent projects. `ClearProjectFocus` binding + a focus picker on the composer chip (no-focus / recent projects / open folder). Unfocused sessions live under the home-dir `projectKey` bucket — no schema change — and `LoadSessionAnyProject` special-cases that bucket back into unfocused mode instead of erroring "project not found". `ProjectTree`/`GitChangedFiles` return empty when unfocused (never eagerly walk a home dir). `ProjectStatus.Focused` drives the UI.

### 19.2 Workbench (right pane) state is per-session

**Trigger:** owner: "แต่ละเซสชันต้องผูกกับฝั่งขวาที่เปิดไว้ด้วย" — switching sessions must bring back what was open.

**Design ([workbench.svelte.ts](desktop/frontend/src/lib/stores/workbench.svelte.ts)):** layout snapshots (browser URLs, file paths, singleton panes, active tab) persist to localStorage keyed `aetox-workbench:<sessionId>` — the Go session store never learns about UI layout. Two entry paths: `switchWorkbenchSession` (explicit switch: save old, restore new) and `adoptWorkbenchSession` (passive id change: first sighting restores, a mid-chat re-key migrates the open tabs). **Terminals are excluded** — a dead PTY restored as an empty shell is more confusing than a closed tab.

### 19.3 Bindings never return nil slices

**Trigger (real failure):** the model-picker button "didn't work" — a Go `nil` slice serialized to JSON `null`, and the menu's render crashed on `thinkLevels.length` mid-open. Same class hit `ListModelsForProvider` and `ListIdentityFiles` (sidebar startup error).

**Rule:** any Wails binding returning a slice returns `[]T{}`, never nil. Fixed in `SupportedThinkLevels`/`ListModelsForProvider`/`ListIdentityFiles`; treat this as the standing contract for every future binding (frontend code is allowed to assume arrays).

**Status:** `Done 2026-07-24.` All three verified live (screenshots + `go test ./desktop` + `svelte-check` green). **Open item from the same session:** DeepSeek V4's DSML tool-call markup leaking into chat as raw text — backstop parser written ([internal/model/dsml.go](internal/model/dsml.go), grammar from DeepSeek-V4-Flash's encoding README) but not yet wired into `openai_compatible.go`'s `Complete()`; §17's backstop-not-gate convention applies. A latent sibling bug is noted for the same file: `StreamComplete` silently drops structured tool-call deltas (currently harmless — no streaming path passes tools).

---

## 20. Decision — Tool-Loop Hardening, Context Truth, Summarizing Compaction (2026-07-24)

One session, three engine layers fixed; OpenCode/Claude Code are the confirmed reference implementations for all of it (owner: "ใช้ OpenCode / Claude Code นี่แหละครับ เป็นแนวทางในการพัฒนาระบบโค้ดดิ้งและคอร์หลัก").

### 20.1 Tool-loop doom loops from truncated tool calls (`7017324`)

**Trigger (real failure):** desktop chat looped forever writing a landing page — `write` failed 7 times with `open C:\Users\...\:` before accidentally succeeding.

**Root cause chain (from `debuglog`):** the tool loop hardcoded `max_tokens: 4096` → long write payloads got cut mid-JSON (`finish_reason: "length"`, never checked) → the salvage parser (`salvageWriteArgs`) had an index bug that turned *every* truncated write into path `":"` → the model was told "path is wrong", so it retried new paths forever (the loop is deliberately uncapped, §17-style; the brake was missing). OpenCode hit the identical class — sst/opencode#18108.

**Fixes ([internal/cognitive/agent.go](internal/cognitive/agent.go), [internal/model](internal/model), [internal/turn/executor.go](internal/turn/executor.go)):** per-provider explicit `max_tokens` (OpenCode's 32k ceiling, clamped where an API 400s: deepseek-v4* 32000, V3-era 8192, openai 16384, anthropic/gemini/zai 32000, others 8192; Ollama now maps `MaxTokens → options.num_predict`); `Response.FinishReason` surfaced from all providers (`length`/`max_tokens`/`done_reason`), and a truncated tool call is **not executed** — the model gets a truthful "arguments truncated, shorten or split" receipt; `salvageWriteArgs` deleted (half-written files reported as ✓ are worse than loud failure); doom-loop brake à la OpenCode PR #3445 (identical consecutive call: warn at 3, hard-stop at 5, reset on any different call). §19's open DSML item closed the same day (`3c9c7e1`): the backstop parser is wired into `Complete()`.

### 20.2 Context truth: per-model windows, no message cap (`fa3bb61`)

**Trigger:** the new context meter showed 32k for deepseek-v4-flash — a 1M-token model. The 32k was `memory.Context`'s internal 128k-char buffer, unrelated to the model.

**Fix:** [internal/model/context_window.go](internal/model/context_window.go) — curated per-model windows (thinking_capabilities.go pattern; openrouter resolves `vendor/model` through the vendor; unknown → 0, caller falls back); the agent's char budget scales to the window (`tokens×4`) in CLI and desktop; `MaxTurns=80` deleted outright — nobody set it, the token/char budget is the only brake, same as the references. User override (`ModelContextTokens`) still wins everywhere.

### 20.3 Summarizing compaction (`65fea12`)

**Trigger:** with real windows in place, the remaining honesty gap: at budget the engine dropped whole old turns verbatim — long coding sessions forgot early decisions.

**Design ([internal/memory/context.go](internal/memory/context.go), [internal/cognitive/agent.go](internal/cognitive/agent.go)):** before each turn, if usage > 80% of budget, the model summarizes everything between the system prompt and the last ~6 messages into one dense message (goals, decisions, files touched, unresolved tasks, user's language) which replaces the span. The boundary always lands on a user turn (assistant+tool blocks never split); failure/empty summary is non-fatal (turn proceeds, char trim still guards); the fresh user message is added *after* compaction so it is never summarized. Known ceilings, deliberate: no mid-tool-loop compaction, and the chars/4 estimate over-counts Thai (3 bytes/char) so Thai sessions compact ~3× earlier — revisit only if it hurts.

**Verified live (2026-07-24):** real DeepSeek run — a secret fact stated in turn 1, four padding turns to 8,881/9,000 chars, compaction produced a 1,401-char summary replacing the original turns, and the model recalled the secret *from the summary*. Unit coverage: truncation/doom-loop/finish-reason (cognitive, model, turn), compaction trigger/boundary/failure (memory, cognitive).

---

## 21. Decision — Research Tool Suite: web_search, web_fetch, GitHub read, images end-to-end (2026-07-24)

**Trigger:** owner use case — "ค้นเว็บหาข้อมูล (มือถือ ฯลฯ) พร้อมรูปภาพ". Audit verdict: the browser suite could limp through search-by-hand, but image URLs never reached the model, and multi-page research meant driving visible tabs one at a time.

**Shipped (each its own commit):**
- `browser_read` now surfaces page images (rendered ≥64px, deduped, max 20, URL+alt) with a hint to embed via `![alt](url)` ([desktop/browser.go](desktop/browser.go) textScript, [desktop/workbench.go](desktop/workbench.go)).
- [internal/skill/web_fetch.go](internal/skill/web_fetch.go) — headless page reading: `x/net/html` walk (already a dependency), scripts/styles stripped, links + images collected as absolute URLs, 40k text cap, http(s) only, output flagged as untrusted data (prompt-injection first line).
- [internal/skill/web_search.go](internal/skill/web_search.go) — DuckDuckGo HTML endpoint, no API key; `uddg` redirect decoding; engine-level, so CLI gets it too. Known ceiling: DDG markup drift degrades to "(no results)", selectors live in one place.
- [internal/skill/github_tools.go](internal/skill/github_tools.go) — `github_search` / `github_list_files` / `github_read_file` on the existing `githubRepoClient`; `GITHUB_TOKEN`/`GH_TOKEN` honored on every GitHub request (rate limit 60→5000/h, private repos); path traversal rejected before any request; all read-only network in `safety` (github_* prefix), `plugin_install` keeps its stricter assessment.
- Composer attachments now carry `[attachment: <type>]` source tags (user image vs dragged file-tab vs dragged browser-tab) — the model can no longer confuse provenance ([cockpit.svelte.ts](desktop/frontend/src/lib/stores/cockpit.svelte.ts)).
- Chat rendering: `.markdown-body img` rules (single image capped 240px; multiple images in one paragraph flow as a wrapping gallery at ~⅓ bubble width); fenced code got the standard AI-chat chrome — language label + copy button + highlight.js (delegated clicks; DOMPurify still sanitizes).
- [internal/model/noop.go](internal/model/noop.go) — noop is the UI test harness: picker models `aetox-image:test` / `aetox-think:test` / `aetox-markdown:test` (plus `img1/img5/imgbig/imgbroken` keyword cases) exercise every rendering path with no API key; `aetox-think:test` streams reasoning through `onReasoningChunk` before the answer.

**Status:** `Done 2026-07-24.` Full suite green; rendering verified live by owner (noop screenshots). Not yet built, deliberately: web_search fallback engine, per-domain fetch rate limiting, browser screenshots (WebView2 `CapturePreview`).

---

## 22. Decision — Capability Extension: video_ocr + computer, and the empty-reply nudge (2026-07-24, `computer` removed 2026-07-25)

**Trigger:** owner positioning — "โมเดลที่อ่านภาพ/วิดีโอไม่ได้ มาใช้ผ่านเราแล้วทำได้ เพราะสถาปัตยกรรมขยายศักยภาพให้โมเดล" — plus a live failure: ornith:9b returned an empty reply after a skill payload and the UI showed a bare `(empty response)`.

**Shipped:**
- Empty-reply recovery ([internal/cognitive/agent.go](internal/cognitive/agent.go)): a blank model reply triggers ONE nudge. In the tool loop the nudge stays **inside the loop with tools intact** — a model missing a capability is told to reach for a tool that covers it, never to refuse; wording is tools-first. Only a second blank reply falls back to the fixed bilingual "เกินขีดจำกัดของโมเดลปัจจุบัน" line. Streaming path returns `streamed=false` on recovery so the UI renders the reply. Language handling is model-mirrored (reply in the user's language), no i18n table.
- [internal/skill/video_ocr.go](internal/skill/video_ocr.go) — ffmpeg samples a frame every N seconds (default 5, cap 120 frames), each frame through the existing `runTesseract`, output as `[m:ss] text` with consecutive duplicates collapsed. Live test synthesizes a two-scene video and asserts text, order, timestamps, dedupe; Thai OCR verified exact on clean rendered text.
- `internal/skill/computer.go` (+ `_windows`/`_other`) — real mouse/keyboard/screen via Win32 SendInput, screenshot through PowerShell `CopyFromScreen`, RiskHigh in safety so every action prompted. **Removed 2026-07-25, see below.**
- Browser form-filling completed (owner: "คลิกในเบราว์เซอร์แม่นยำ กรอกแทนได้หมด"): `browser_type` now handles `<select>` (option matched by value/label via the native setter — previously the else-branch would have destroyed the options with textContent) and takes `enter=true` (synthetic keydown, then `requestSubmit` only if the page didn't preventDefault — untrusted key events never trigger implicit submission); `browser_read` lists each select's `[options: ...]`. JS logic proven by executing the generated script against a stub DOM (select match, no-clobber on miss, no double-submit). RTK-for-web idea assessed and rejected: rtk's filters are per-CLI-format ([internal/rtk/rtk.go](internal/rtk/rtk.go) pipeFilters), web compression already lives in web_fetch.

**Status:** `Done 2026-07-24.` Live-verified on the dev box: cursor round-trip, real-screen screenshot → OCR read actual VS Code content. Not built, deliberately: scene-detection sampling and audio transcription for video_ocr (`ponytail:` comments mark the ceilings).

**Amendment 2026-07-25 — `computer` removed.** In real use the tool did not work from the desktop app (owner: "computer ใช้งานไม่ได้จริง"): calls like `screen_info` and `key win+d` came back failed while every other tool in the same turn succeeded. Deleted rather than debugged — the capability it sold (act on the real screen) is covered inside the app by `browser_*` for web work and by `shell` for the machine, and a tool that fails in front of the user is worse than one that was never offered. Gone: the five `computer*.go` files, its `RegisterDefaults` entry, its `safety.Assess` branch, and its `toolCallToArgs` case. Docs that sold it ([README.md](README.md), [docs/index.html](docs/index.html), [PLATFORM-SUPPORT.md](PLATFORM-SUPPORT.md)) now lead with `browser_*` for the "hands" half of the story. If desktop control comes back, it comes back with a live end-to-end test on a real desktop, not just unit tests over the input-buffer builders — that was the gap that let a broken tool ship.

---

## 23. Decision — Windows Distribution: GitHub Releases → winget → Scoop; npm rejected (2026-07-24)

**Trigger:** owner — "เตรียมทำให้ติดตั้งได้ ผ่าน npm หรืออะไร เสนอมา อันไหนพร้อมและสะดวก วินโดวส์ก่อน".

**Channel ranking:**
1. **GitHub Releases + NSIS installer** — the foundation every other channel points at. The installer ([desktop/build/windows/installer/project.nsi](desktop/build/windows/installer/project.nsi)) was already excellent: WebView2 runtime + Tesseract silently installed with pinned SHA256 + Thai traineddata. What was missing was purely the pipeline.
2. **winget** — widest reach (built into Win 10/11); needs one published release first, then `wingetcreate new <installer-url>` generates the manifest PR. Blocked on the LICENSE decision (winget requires a license field).
3. **Scoop** — zero-gatekeeper channel; manifest lives in this repo ([scoop/aetox.json](scoop/aetox.json), `checkver: github` + autoupdate), users run `scoop install https://raw.githubusercontent.com/Mike0165115321/Aetox/main/scoop/aetox.json`.
4. **npm — rejected:** wrong audience and wrong shape for a desktop app (per-platform binary wrapper packages, no Start-Menu integration). Revisit only if the CLI ever targets JS-ecosystem devs specifically.

**Shipped:**
- [.github/workflows/release.yml](.github/workflows/release.yml) — tag `v*` → windows-latest builds `wails build -nsis` (wails pinned to go.mod's version, NSIS via choco), portable zip, `checksums.txt`, attaches all three to a **draft** release (owner publishes manually). The CLI exe was dropped from the release in §30.
- [desktop/wails.json](desktop/wails.json) `info` block (product name/version/copyright); CI re-stamps `productVersion` from the tag, local builds use the committed value.
- Proven locally end-to-end 2026-07-24: NSIS via `scoop install nsis`, `wails build -nsis` → `aetox-desktop-amd64-installer.exe` (12.4 MB) with correct version metadata (ProductVersion 0.4.0).
- [docs/index.html](docs/index.html) — Thai landing page for GitHub Pages (Settings → Pages → main, `/docs`): download CTA on the stable `releases/latest/download/...` URL, scoop one-liner, capability-extension pitch with a terminal mock of the blind-model loop, honest SmartScreen note. Single self-contained file, no build step, one accent color, gridgeist-reviewed.

**Release checklist (owner):**
1. Bump `info.productVersion` in [desktop/wails.json](desktop/wails.json) — that is the shipped version. `appVersion` in [cmd/aetox/main.go](cmd/aetox/main.go) is the unshipped CLI's own string (§30); keep it in step with the product or leave it, nothing user-facing reads it.
2. `git tag v0.4.0 && git push origin v0.4.0` → CI drafts the release → review + publish.
3. First release only: copy the portable zip's SHA256 from `checksums.txt` into [scoop/aetox.json](scoop/aetox.json) `hash` (autoupdate maintains it afterwards).
4. When LICENSE lands: `wingetcreate new <installer asset URL>` → PR to microsoft/winget-pkgs.

~~**Open decision (owner):** LICENSE file~~ — **closed 2026-07-25 (§28):** MIT, matching what README already claimed; `scoop/aetox.json` updated from `TBD`. The winget step above is now unblocked.

**Not built, deliberately:** code signing (unsigned exe = SmartScreen "unknown publisher" warning on first run — Azure Trusted Signing ~$10/mo when distribution volume justifies it), macOS/Linux packaging (desktop is Win32-only today, §22).

---

## 24. Decision — Settings Parity Roadmap + Process-Tree Lifetime via Job Object (2026-07-24)

**Trigger:** owner — "เรามาทำให้มันพร้อมจริงๆกันดีกว่า ... ผมจะทำทั้งหมดครับ เอาให้เข้ากับบริบทเรา" หลังเทียบ Settings sidebar ของ ZCode กับของจริงในโค้ด (ผลสำรวจอยู่ใน [SETTINGS-PARITY-PLAN.md](SETTINGS-PARITY-PLAN.md) ซึ่งเป็นเอกสารแผนของ decision นี้).

**Decisions:**
1. **ไม่ก็อป sidebar ของ ZCode 1:1** — เอาเฉพาะหัวข้อที่มีของจริงให้ต่อยอดหรือมีความต้องการจริง เรียงถูก→แพง (Skills/Plugins → Onboarding → Usage stats → Commands → Code preview → Subagents) ทุก phase จบแล้ว ship ได้เป็น commit แยก.
2. **Skills + Plugins = หน้าเดียว** — สองหัวข้อของ ZCode คือกลไกเดียวกันของเรา (โฟลเดอร์ `SKILL.md`; `plugin_install` ก็เขียนลง `~/.agents/skills/` อยู่แล้ว) การรวมยังปิด half-finished loop ของ `plugin_install` (ติดตั้งแล้ว re-discover ทันที ไม่ต้อง restart).
3. **Indexing page ตัดทิ้ง** — FTS5 (§ desktop/db.go) เป็น implementation detail ไม่มี knob ให้ผู้ใช้; ZCode มีหน้านี้เพราะทำ repo-wide RAG indexing ซึ่งเราไม่ได้ทำ. เพิ่มเมื่อทำ code indexing จริงเท่านั้น.
4. **Subagents ต้องผ่าน design ก่อนเขียนโค้ด** — `internal/orchestrator` (§10) คือ scaffold ที่รอเรื่องนี้; จะได้ Decision section ของตัวเองก่อน implement (walking skeleton: `task` tool, depth 1) — ไม่แตะ ensemble/routing ของ ADR 0002 ในรอบนี้.
5. **Child-process lifetime แก้ที่ root ด้วย Windows Job Object** — พบ orphan `node.exe`/`cmd.exe` (MCP servers) ค้างหลังปิดแอปจริงในเครื่อง owner คืนนี้. แทนที่จะไล่ทำ process-group cleanup รายทาง (upgrade path เดิมที่มาร์ก `ponytail:` ใน `internal/mcp/client.go`), ตอน boot ใส่ process ตัวเองเข้า Job Object ที่ตั้ง `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` — ลูกหลานทุกตัวจากทุกเส้น spawn (MCP, ConPTY shell, git, rtk, ffmpeg) อยู่ใน job โดยอัตโนมัติและถูกฆ่าทั้งต้นไม้เมื่อ process หลักตาย ไม่ว่าจะปิดดีๆ หรือโดน force-kill. จุดเดียวจบใน [internal/proc](internal/proc) (แพ็กเกจเดียวกับ `HideConsole` ของ §fix `917b550`) เรียกจาก main ของทั้ง desktop และ CLI; no-op นอก Windows.

---

## 25. Decision — Subagents: the `task` tool (Proposed 2026-07-24, awaiting owner approval)

**Trigger:** [SETTINGS-PARITY-PLAN.md](SETTINGS-PARITY-PLAN.md) Phase 6 — the last parity item, and the first real caller for the `internal/orchestrator` scaffold (§10) that has sat unused since it was built.

**Proposed design (walking skeleton, nothing more):**
1. **One new built-in tool `task`** — schema `{description: string, prompt: string}`. When the MAIN agent calls it, the executor spawns a sub-agent via `orchestrator.Spawn` with the same provider/model, a **fresh context** (no conversation history — the prompt must carry everything, same rule as every task-tool system), and the same skill registry **minus `task` itself** — depth 1 is enforced structurally, not by a counter.
2. **Lifecycle:** the sub-agent runs its own bounded tool loop; ctx cancellation propagates (Stop button kills the whole chain); its final text returns as the tool output; the orchestrator entry is removed on completion. Token usage flows through the same `SetUsageReporter` hook (§ Phase 3), so Usage stats absorb subagent spend automatically.
3. **Safety:** the sub-agent inherits the session's `ApprovalMode` and permission rules unchanged — approval prompts surface to the same UI. No silent privilege widening.
4. **UI:** `task` shows up as a normal tool step in the chat timeline (label = description). No new panel in v1.
5. **Settings:** single knob — enable/disable the `task` tool (default off until proven). Lives on the existing General page, not a new section.

**Explicitly out of scope (this decision):** ADR 0002's ensemble/routing/consensus, per-task model override (phase 2 if wanted), parallel fan-out, persistence of sub-agent transcripts, and any cross-process orchestration.

**Status: Proposed.** Implementation starts only after the owner approves this section.

---

## 26. Decision — Browser Tab Errors Must Never Crash the App (2026-07-24)

**Trigger:** owner hit a live crash — `browser tab error: The group or resource is not in the correct state to perform the requested operation` (`ERROR_INVALID_STATE`, 0x8007139F) that took the whole app down ("เบราว์เซอร์ชอบมีปัญหา มันพาแอปเด้งหลุด กลัวคนอื่นจะมีปัญหาด้วย").

**Root cause:** `github.com/wailsapp/go-webview2`'s `Chromium.errorCallback` always calls `os.Exit(1)` after the error callback. `desktop/browser.go` installs a `SetErrorCallback` to log-and-continue, but that only swaps the inner callback — the `os.Exit(1)` fires regardless. Aetox embeds one WebView2 per browser tab, so any single tab's transient failure (RTSS DLL injection — the same RivaTuner that crashed the app's own webview in the install session, GPU driver, low memory) crashed the entire process. `os.Exit` can't be recovered in-process (skips defers, defeats recover).

**Decision — vendor + patch, matching the `conpty` precedent (§ `917b550`), owner-approved:** fork go-webview2 into [third_party/go-webview2](third_party/go-webview2) via a `go.mod replace`. Four surgical `AETOX PATCH` edits (documented in [third_party/go-webview2/AETOX-PATCH.md](third_party/go-webview2/AETOX-PATCH.md)): a caller-installed `SetErrorCallback` now owns the exit decision (default handler still exits, so the **wails main window is unaffected** — it never sets a custom callback); the controller-failure path early-returns instead of nil-dereferencing, unblocks `Embed`'s message loop, and makes `Embed` report `false` so `browser.go` tears down the orphan child window. Net effect: a browser tab that fails to embed just logs and disappears; the app lives.

**Rejected alternative:** per-tab WebView2 user-data folders (would remove *this* `ERROR_INVALID_STATE` trigger but not the class — other machines hit other transient webview errors that all route through the same `os.Exit`). The vendor+patch is trigger-independent.

**Not done:** upgrading past v1.0.22 (re-apply the four patch blocks when doing so); process-isolating the browser host (heavier, unnecessary now that errors are non-fatal).

---

## 27. Decision — Engine Parity Batch + Interactive UI Tools (2026-07-25)

One session, three fronts; OpenCode/Claude Code remain the reference implementations (per §20's owner directive). Everything below is unit-tested; the suite is green across `go test ./...` + vitest.

### 27.1 Engine parity fixes (OpenCode/Claude Code alignment audit)

An audit of model selection / tool loop / long-run behavior against the references found five deviations; all fixed in [internal/cognitive/agent.go](internal/cognitive/agent.go), [internal/turn](internal/turn), [desktop/app.go](desktop/app.go):

1. **Ephemeral tool summaries** — `summarizeToolExecution` used to route through `Respond`, which wrote the summary prompt (kilobytes of tool output, as a fake user message) into conversation history forever. New `Agent.RespondEphemeral` completes over the context without writing anything — the references never let meta-work pollute the transcript.
2. **No duplicate user message** — the tool loop's first-call-failure fallback re-added the user message via `Respond(msg)`; extracted `respondFromContext` responds over history as-is.
3. **Per-provider output ceiling for plain conversation** — `Respond`/`RespondStream` were flat-capped at 768 tokens (truncating long answers for non-tool-calling models); now they use `toolLoopMaxTokens()` like everything else. A ceiling costs nothing until used.
4. **Mid-loop compaction** — §20.3's known ceiling closed: `compactIfNeeded` now runs every tool-loop round (OpenCode checks per step). `SplitForCompaction`'s user-turn boundary guarantees the in-flight turn's tool results are never summarized away.
5. **Model switch keeps tool history** — `applyConfig` now replays the *outgoing agent's real context* (tool calls, results, compaction summaries) into the fresh agent instead of the text-only transcript; falls back to transcript when no live agent exists. Project-switch paths are safe: every `reload` caller immediately `startNewSession`/`LoadSession`s, which resets context.

**Slow-tool guard (owner-requested):** a single chokepoint in [internal/turn/executor.go](internal/turn/executor.go) `executeTool` runs every model-driven tool call under a 60s deadline; overrun returns a truthful "abnormally slow … abandoned, retry with a narrower scope" receipt to the model instead of hanging the turn (the 270s `grep "(?i)aetox" .` case). FS-walking tools ignore ctx and finish in a stray goroutine whose result is discarded — `ponytail:` marked, plumb ctx if the CPU leak ever bites. `interactiveTools` (ask_user) are exempt: waiting on a human is their job.

### 27.2 Wire-format latent bugs (the 401 restart bug)

`c49ec3b`'s wire-format support had two endpoint-resolution bugs that only surfaced when the app finally restarted (preferences normalize the catalog-default BaseURL to `""` on save):

- [internal/model/anthropic.go](internal/model/anthropic.go): empty BaseURL defaulted to `DefaultBaseURL("anthropic")` — the real api.anthropic.com — ignoring `cfg.Provider`. A restarted app sent the DeepSeek key to Anthropic → `401 invalid x-api-key`. Now defaults to the *provider's own* endpoint first.
- [internal/model/factory.go](internal/model/factory.go): switching wire formats kept a stale other-format BaseURL (OpenAI requests aimed at `/anthropic/v1` and vice versa). The default-format URL now swaps to the alt endpoint on an alt-format switch and back; only a user-customized URL survives.

**Model discovery through the alt endpoint** ([internal/model/provider_catalog.go](internal/model/provider_catalog.go)): Anthropic-format providers have no `/models` endpoint, so DeepSeek's model list was always empty. Discovery now routes through the OpenAI-compatible `AltBaseURL` when the primary runtime can't discover — chat stays on the Anthropic format (clean streamed tool calls), listing uses the alt endpoint, same account/key. Verified live against api.deepseek.com.

**Dev loop:** [desktop/wails.json](desktop/wails.json) gained `reloaddirs: ../internal,../cmd` — backend edits outside `desktop/` now trigger the rebuild they always should have (the missing piece behind several "แก้แล้วหน้าจอเหมือนเดิม" sessions).

### 27.3 Interactive UI tools: `ask_user` + `todo_write` (desktop-side skills)

Two new skills in [desktop/ask_user.go](desktop/ask_user.go), registered with the workbench tools (same pattern as `browser_*`) because they talk to the UI over Wails events:

- **`ask_user`** — the Claude Code AskUserQuestion pattern: the model blocks mid-turn on a question + 2–4 options; the chat renders stacked A/B/C/D option cards; the composer doubles as free-text answer input while pending. One question in flight at a time (second concurrent ask fails loudly); turn cancel unblocks; exempt from the slow-tool deadline.
- **`todo_write`** — the Claude Code TodoWrite pattern: the model maintains a task checklist (full-list replace per call; ○/▸/✓ statuses) rendered live in the turn bubble. `ponytail:` frontend-state only — persist with the session if surviving reloads ever matters.

**UI test model** ([internal/model/noop.go](internal/model/noop.go)): `aetox-tools:test` added to the aetox picker — a stateless scripted turn (round derived from tool results in the transcript): todo_write → ask_user (really blocks) → todo_write all-completed → final text echoing the user's pick. `aetox-think:test` now streams ~6 numbered sections of long reasoning to exercise the unbounded panel.

### 27.4 Chat UX (owner-driven)

- **Thinking persists** — reasoning chunks are accumulated in `SendMessage` and saved on the session turn (`SessionMessage.Reasoning` + `ThinkSecs`, first→last chunk); finished messages show a collapsed "คิดเป็นเวลา Xs" toggle. Live panel unchanged; the 220px CSS max-height wall is gone (thinking renders at natural height).
- **Pinned auto-scroll** — while at the bottom, every live update (chunks, reasoning, tool steps, todos, ask panel) keeps the view pinned; scrolling up unpins, scrolling back re-pins. Reference behavior.
- **Stop button** — `CancelTurn` existed backend-side but *nothing in the UI called it*; the send button now becomes a red ■ during a turn.
- Copy button on AI replies; provider connection test (`TestProviderConnection`: a real 1-token completion through the same client chat uses — endpoint+key+wire format in one shot); chat model menu is picker-only (custom model ids live in Settings); sidebar projects fold their chat history by default.

**Not yet built (proposed, awaiting owner):** edit-previous-message + resend (needs history truncation), queued mid-turn send, retry button, `aetox-error:test`/`aetox-doomloop:test` UI test models.

---

## 28. Decision — External Review Response: Sandbox, Process and Read-Path Hardening (2026-07-25)

**Trigger:** an outside code review of the public repo (owner relayed it verbatim). Seven findings, all accepted, all fixed in one batch. Recorded here because three of them change security or tool-contract behavior, not just internals.

**1. `shell` output buffer had no ceiling** ([internal/skill/shell.go](internal/skill/shell.go)). `limitLines` trims *after* the process exits, so between spawn and the 60s slow-tool deadline (§27.1) a runaway producer (`yes`, a looping log tail) grew a plain `bytes.Buffer` without bound — gigabytes of RAM inside a desktop app. Replaced with `cappedWriter`: first 1 MiB kept, remainder dropped, `dropped` folded into the existing `Truncated` flag so the model is still told output was cut. No mutex — `os/exec` collapses Stdout and Stderr to one pipe and one copy goroutine when they hold the same interface value, which is how `Execute` wires them.

**2. Cancelling a command orphaned its children** ([internal/proc/tree_windows.go](internal/proc/tree_windows.go), [tree_other.go](internal/proc/tree_other.go)). `exec.CommandContext` kills only the process it spawned, so Stop-during-`npm install` left npm, node and their children running. §24's Job Object was the *app-exit* answer and is still correct there — it just fires far too late for a user who pressed Stop. New `proc.KillOnCancel(cmd)`: Windows shells out to `taskkill /T /F` (no Win32 kill-process-group primitive exists short of a Job Object per command); Unix sets `Setpgid` and signals `-pid`. Both set `WaitDelay`, without which `Run` blocks past the kill on an output pipe a surviving grandchild still holds open. Wired into `shell` only — the other `exec` sites are short-lived and were not the reported problem.

**3. Sandbox containment was lexical, so symlinks escaped it** ([internal/skill/list.go](internal/skill/list.go)). `resolveSandboxPath` cleaned the path and compared prefixes; a symlink inside the root pointing at `C:\Users` or `/etc` passed untouched, and the existing tests only covered `../`. Now both sides of the comparison run through `evalExistingSymlinks` — `EvalSymlinks` on the deepest *existing* prefix, since `write`/`edit` legitimately name a leaf that does not exist yet. The **returned** path stays lexical so callers and their output still show the path the user asked for. Same fix closes the reviewer's second-order bug: `withinRoot` lower-cases on Windows, because rejecting `C:\Work` under root `c:\work` on a case-insensitive filesystem is a false positive, not safety.

**4. `read` capped at 16KB — a correctness bug for a coding agent** ([internal/skill/read.go](internal/skill/read.go)). An 800-line Go file exceeded it, so the model reasoned about code it had never seen and the truncation notice gave it no way to continue. Replaced with line paging à la Claude Code/opencode (§20's reference rule): `offset` (1-based) + `limit` (default 2000 lines, 256KB hard ceiling), and the truncation marker now names the exact offset to resume from. `bufio.Reader.ReadString` not `Scanner` — a minified bundle blows past Scanner's 64KB token limit. `fs cat` carried an identical 16KB ceiling and now shares the same `readTextLines`/`looksBinary` helpers: one truncation rule in the codebase, not two.

**5. No CI ran the tests** ([.github/workflows/ci.yml](.github/workflows/ci.yml)). 71 test files and nothing executing them outside the owner's machine. `windows-latest` because that is what Aetox ships on. The frontend build step is mandatory, not decorative: `desktop/main.go` embeds `frontend/dist`, which is gitignored, so `go vet ./...` fails on a fresh checkout without it. Runs `go vet` + `go test ./...` + the vitest suite.

**6. The `engine/`/`providers/`/`cli/` module scaffold is deleted.** Four days of `go.mod`-only directories with zero source made first-time readers hunt for where the code lives, and §8's "migration drift risk" had no timeline behind it. Deleted along with `go.work`/`go.work.sum` — with the scaffold gone the workspace listed one module, i.e. nothing, and §7's open question about committing `go.work.sum` dissolves with it. The *reason* the split was proposed (`internal/model` importing `internal/provider`, §6.3) is unchanged and still recorded in [docs/architecture/module-split-2026-07-21.md](docs/architecture/module-split-2026-07-21.md); re-scaffold when the migration actually starts.

**7. API keys are plaintext JSON at `0600`** — accepted as-is for v0.4, but it now appears in [README.md](README.md)'s security table with the OS-keychain move (Windows DPAPI / macOS Keychain) named as the next step. The product pitch leads with "your data is yours"; the gap between that and the storage belongs in public docs, not only in a reviewer's notes.

**Also added:** [LICENSE](LICENSE) (MIT). README already claimed MIT and `scoop/aetox.json` said `TBD` — without the file, "no lock-in" was legally untrue, since no license means all rights reserved.

### 28.1 What CI found in its first hour (the fixes the review did not ask for)

Standing up CI immediately paid for itself — three defects the review never saw, two of them pre-existing and one of them serious:

- **Every quoted shell command was corrupted on Windows** ([internal/proc/shell_windows.go](internal/proc/shell_windows.go)). `exec.Command("cmd", "/C", line)` makes os/exec escape embedded quotes for the C runtime's argv convention, which **cmd.exe does not follow** — so `echo "hello world"` came back as `\"hello world\"`, and `git commit -m "msg"`, `python -c "..."`, `grep "a b"` were all silently mangled. Nothing caught it because every test used unquoted commands; it surfaced only when the new process-tree test tried to run a quoted path. Fixed by building the invocation through `SysProcAttr.CmdLine` as `cmd /S /C "<line>"` — `/S` is cmd's documented "strip the outer quotes, take the rest literally" rule. Unix is unaffected (execve takes an argv array) and now shares one `proc.ShellCommand` seam instead of an inline `runtime.GOOS` branch in [shell.go](internal/skill/shell.go). Regression test: `TestShellSkillPreservesQuotedArguments`.
- **`go test` downloaded a third-party binary from the internet** ([internal/rtk/install.go](internal/rtk/install.go)). §13.6's lazy auto-install fired inside the test suite on a clean CI runner — GitHub API call plus a multi-megabyte download nobody asked for, and network-flaky by construction. `resolve()` now returns empty under `testing.Testing()`; tests that need rtk skip.
- **`TestShellSkillRewritesToRTKWhenAvailable` guarded on the wrong thing** — `rtk.Available()` (binary resolvable) does not imply `rtk.Rewrite` has an equivalent for a given command, and `Rewrite` returning `ok=false` is a normal outcome, not a failure. It only ever passed because the owner's machine has rtk with a `git status` filter. Now guards on the rewrite itself.

- **The CLI did not build on macOS or Linux at all** — `internal/model/context_windows.go` holds model *context-window* token limits and has nothing to do with the OS, but Go reads a `_windows.go` suffix as an implicit build constraint, so every non-Windows build failed on `undefined: model.ContextWindowTokens`. Renamed to `context_window.go`. A cross-compile step now runs in CI, which also gives `internal/proc`'s `!windows` files (the process-group kill above) their first compile check — until now they had never been built by anything. This also answers §7's standing "is Aetox Windows-only by design?" question in the least flattering way: it was Windows-only by filename accident, not by decision.

**Verification beyond unit tests** (owner asked, and the answer was not "yes" until this was done):

- The process-tree fix has a real behavioral test, not a mocked one: `TestShellSkillCancelKillsGrandchild` runs the test binary itself as a grandchild through the shell, writing a heartbeat file every 150ms, then cancels and asserts the heartbeat stops. Checked for teeth by disabling `proc.KillOnCancel` — the test fails, `Execute` hangs on the output pipe the survivor holds, and the orphan locks its temp dir. That is precisely the reported bug, reproduced and then closed.
- **Measured, not assumed:** `resolveSandboxPath` costs **981µs/call** with symlink resolution vs **1.8µs** lexical (Windows; Defender scans every component open) — a 530× relative regression that is ~2ms per tool call in absolute terms, because it runs at most twice per call and never inside `grep`/`fs find`'s `WalkDir` (verified by reading both walk bodies). Left unoptimized with a `ponytail:` note naming the cache-the-root upgrade path.
- End-to-end through the real CLI binary (`aetox chat` + the built-in `aetox-tools:test` model), not just package tests: the `read` rewrite returns real file content through the actual dispatcher.

**Not done:** `proc.KillOnCancel` on the MCP/ffmpeg/tesseract `exec` sites (§24's Job Object still covers those at exit); per-command Job Objects instead of `taskkill` (needs a suspended-start to close the assignment race — real, but not worth it before someone hits it); the keychain migration itself.

---

## 29. Decision — Windows First, Recorded Not Pursued; and the Sibling-Bug Sweep (2026-07-25)

**Trigger:** owner, after §28's cross-platform findings — *"ผมยังไม่อยากรีบโดดไป linux หรือแม็ค แต่ถ้าโครงสร้างมันพร้อม ก็แจ้งในเอกสารไว้ ... เอาวินโด้ให้ทำงานจนเสถียรได้ก่อนดีกว่า"* plus *"เจออะไรอีก อันไหนไม่กระทบมากแก้ได้แก้เลยแต่สำรวจดีๆ"*.

**Decision 1 — Windows is the platform; portability is a record, not a roadmap item.** [PLATFORM-SUPPORT.md](PLATFORM-SUPPORT.md) states exactly what runs where and what each further step costs, and is explicitly *not* a plan in flight. This also closes §7's standing "is Aetox Windows-only by design?" question with evidence, in two parts: the CLI and engine were Windows-only **by accident** (a filename, fixed in §28.1) and now cross-compile under CI guard; `desktop/` is Windows-only **by construction** — it imports ConPTY and go-webview2, and `browser.go` calls Win32 with no build tag. Nothing beyond the cross-compile check was added for Unix; the `!windows` files still have never been executed anywhere.

**Decision 2 — chase the bug class, not the reported instance.** §28's review named the unbounded buffer in `shell`. Grepping every sibling that buffers a child process's output found the same defect elsewhere, and the sweep of every file tool against the real repository (not a synthetic temp dir) found three more:

1. **`git` had the identical unbounded buffer** ([internal/skill/git.go](internal/skill/git.go)) — `git log` with no range, `git diff` on a large change, `git show` of a commit that added a binary. `cappedWriter` moved out of `shell.go` into [output.go](internal/skill/output.go) next to `limitLines` and both tools now share it. `executeCommand` also reports whether the cap was hit, because a capped `git show` of a binary is a single very long line that `limitLines` would otherwise call complete.
2. **Every `git diff` on Windows was prefixed with CRLF warnings** — stdout and stderr were merged into one buffer, so `warning: LF will be replaced by CRLF` appeared once per touched file, glued to the front of the diff the model reads. On a large change that noise can push the real content past the 220-line limit. stderr is now separate and only surfaces when the command fails, which is the only time it carries the information.
3. **`edit` loaded any file entirely into memory** ([edit.go](internal/skill/edit.go)) — `data`, the string conversion, the `Replace` result and the write-back are four live copies, so a few hundred MB of generated log was enough to end the process. A 16 MiB stat guard refuses early with a message telling the model what to do instead. No source file, lockfile or config comes near it.
4. **`write` and `delete` echoed the resolved absolute path** while `edit` echoed the relative one. Made consistent — the absolute form is token noise that also nudges the model toward repeating the sandbox root back at the user, against `internal/prompt`'s environment rule.

Checked and deliberately left alone: `image_ocr` (tesseract stdout is one image's text), `video_ocr` (ffmpeg runs at `-loglevel error`), `rtk` (bounded by the input it filters), `computer` (a PowerShell result). Also verified the project's own "every `exec.Cmd` passes through `HideConsole`" invariant (§`917b550`) still holds at every one of the eleven spawn sites.

**Measured, not assumed:** `read` costs ~0.65ms and shell ~39ms per call end-to-end on Windows (200-iteration benchmarks). Sandbox path resolution dominates the file tools at ~0.6–1ms — the cost of §28's symlink containment — which is noise beside a model round trip, so the cache-the-root upgrade path stays a `ponytail:` note rather than code. Shell is dominated by cmd.exe process creation, unchanged by anything here.

**Verified by sweeping the real repo, not fixtures:** read/list/grep/fs/git against the actual working tree and write/edit/delete against scratch — sandbox escape refused, binary detection correct, paging correct at an offset deep in a large file, git output clean.

---

## 30. Decision — The CLI Is Not a Product: Unship and Unadvertise, Keep the Code (2026-07-25)

**Trigger:** owner — *"CLI ไม่เคยเทสเลย ลบออกไปก่อนได้ไหม ตัวติดตั้ง โฆษณาด้วย ที่ผ่านมาโฟกัสแค่ตัวเดสท็อปอยู่"*.

**The problem was honesty, not code.** `cmd/aetox` compiles, has tests, and passes CI — but it has never been exercised as a product, while [README.md](README.md) listed it as a shipped capability twice and the entire "เริ่มต้นใช้" section taught CLI flags as *the* way to start Aetox. The release attached `aetox-cli-windows-amd64.exe` to every GitHub Release. Users were being handed, and pointed at, something nobody had ever used end to end.

**Decision — stop shipping and stop advertising it; keep the source.** Owner picked this over deleting `cmd/aetox` outright, and it is the right call: the code costs nothing to keep, CI keeps it compiling and tested so it does not rot, and "ก่อน" (for now) stays literally true — bringing it back is a docs change, not a rebuild.

**Changed:**
- [.github/workflows/release.yml](.github/workflows/release.yml) — the `Build CLI` step, the checksum entry and the release asset are gone; a release is now installer + portable zip + `checksums.txt`. Header comment states why, so the next person does not "restore" it.
- [README.md](README.md) — the two feature-table rows claiming a shipped CLI are gone, and **"เริ่มต้นใช้" was rewritten around what actually ships**: installer link, scoop one-liner, portable zip, the SmartScreen note, and `wails build` for building it yourself. The old section was pure CLI flags, i.e. install instructions for a product that no longer exists. The `Flags` table went with it. `cmd/aetox` stays in the project tree, labelled as not-a-product.
- The closing "วันนี้คือ CLI ไม่กี่พันบรรทัด" line was cut whole (owner's call) rather than reworded — the paragraph reads fine without it.
- §23 above: shipped-assets list and release checklist corrected; the LICENSE open decision marked closed by §28.

**Explicitly unchanged:** `cmd/aetox`, `internal/app`'s terminal presentation, and `build.ps1`. §6.1's finding — that the GUI links CLI presentation code it never calls — is untouched and still open; unshipping does not make it worse, and deleting the CLI would have forced that cleanup in the same breath.

**What this costs:** `appVersion` in `cmd/aetox/main.go` is now the only version string nothing user-facing reads. Left in place rather than deleted, for the same reason as the rest of the CLI.

---

## 31. Decision — `audio_transcribe`: the Last Sense, and Why the Model File Is Not Bundled (2026-07-25)

**Trigger:** owner — a clip that is only speech, with nothing written on screen, returns an empty `video_ocr`. Aetox could read images and read text off video frames but could not *hear*.

**Shipped:** [internal/skill/audio_transcribe.go](internal/skill/audio_transcribe.go) — ffmpeg strips a 16kHz mono WAV (`-vn -ar 16000 -ac 1 -c:a pcm_s16le`, one command that covers audio files and video files alike), whisper.cpp transcribes it locally, segments come back as `[m:ss] text`, byte-identical in shape to `video_ocr` so both can run over one clip and read as a single transcript. Language is `-l auto`; consecutive repeats collapse (whisper loops phrases over silence) and `limitLines` caps the output like every other tool.

**Local binary, never a cloud API.** Owner's constraint, and the only one consistent with the product: uploading a user's audio to transcribe it would contradict the single promise Aetox makes. Same shape as tesseract and ffmpeg — an external binary reached through `exec`, no new Go dependency.

**The ggml model is not bundled and not auto-downloaded.** base is ~142MB against a ~12MB installer that [docs/index.html](docs/index.html) advertises by size — bundling would inflate the download twelvefold for a capability most sessions never use. Silently fetching it on first use was rejected too: spending 142MB of someone's bandwidth without asking is not a decision a tool gets to make. A missing model returns instructions instead — what to fetch, how big, and the exact path to drop it in (`<DataRoot>/models/`, the §14 data root). Any `ggml-*.bin` already present is accepted, so a user who chose tiny or small is not nagged for base.

**Safety: no new branch, on purpose.** `video_ocr` and `image_ocr` have no case in `AssessCommand` — they fall to the read-only default (`RiskLow`, no effects, never prompts). Adding a branch for `audio_transcribe` would have put it in a *different* tier than the tools it belongs with, so the change to [internal/safety/safety.go](internal/safety/safety.go) is deliberately zero lines. A test in [safety_test.go](internal/safety/safety_test.go) pins all three to the same assessment and the same `ShouldPrompt` answer in every approval mode, so a future edit cannot silently split them.

**Verification without the 142MB download.** The interesting failure mode is wrong whisper flags or a mis-parsed output format — neither of which unit tests over pure functions can catch, and the live test skips on any machine without whisper.cpp installed (including the dev box). So `TestMain` doubles as a stub whisper.cpp via the `os/exec` helper-process idiom: the test re-execs the test binary in the binary's place, and the stub **fails the test** unless it was handed an existing model, an existing converted `.wav`, `-l auto` and `-np`. ffmpeg conversion, argument construction, output parsing and temp-dir cleanup are all exercised for real on every machine that has ffmpeg. Proven to fail: flipping `-l auto` to `-l th` in the production path turns the test red.

**Status:** `Done 2026-07-25.` Full suite green. Not verified: whisper.cpp's own accuracy on Thai speech, and that the real binary's stdout matches the stubbed format — both need the binary + model on a real machine, which is a user decision, not a build step.

---

## 32. Decision — Assets: What a Tool Needs on Disk, Chosen and Fetched by the User (Proposed 2026-07-25)

**Trigger:** owner, immediately after §31 shipped — *"สกิลนี้ต้องเพิ่มทางเลือกแล้วครับ เขาจะเลือกโมเดลอะไรยังไง ... สกิลอื่นพร้อมให้โมเดลเรียกใช้ อันนี้มันต้องเป็นมากกว่านั้นแล้ว ... อนาคตเผื่อโมเดลอะไรใหม่ๆ มา ระบบต้องรองรับและขยายต่อได้ เราจะไม่ฝังโมเดลในระบบ เพราะมันจะหนักไฟล์ดาวน์โหลด"*.

**The distinction that forces a new concept.** Every skill in [internal/skill](internal/skill) has exactly one audience: the model. `grep`, `shell`, `video_ocr` — the model calls it, it runs, done; no human prepares anything first. `audio_transcribe` has **two** audiences: the model calls it, but a *human* must first choose which ggml model to run (30MB and fast but poor at Thai, 141MB balanced, 466MB best and slow) and get it onto the disk. Aetox has no place for that second audience at all. §31's answer was an error message telling the user to go download a file themselves — which is a capitulation, not a design.

**It was never one tool's problem.** Three tools already depend on something Aetox does not ship, and all three answer it differently: `tesseract` (NSIS installs it silently at Aetox install time), `ffmpeg` (an error with a `winget install` line), `whisper.cpp` + its model (an error with a URL and a path). None of it is visible anywhere in the app, so "why doesn't this work" is answered only after the user has already tried and failed. Assets is where those three converge.

**Naming (owner picked, from three candidates):** **Asset** in code — `internal/asset`, `skill.Provisioner` — and **"ไฟล์ที่ต้องมี"** in the UI. Chosen over "Capability Pack" (nicer in a UI, but couples 1 pack to 1 tool when tesseract already serves two tools) and "Runtime / Local Model" (clearest for Ollama/LM Studio users, but "runtime" already means the WebView2 runtime in this project's installer vocabulary). Asset covers both model weights and binaries, which is what the catalog actually holds.

**Shape — the skill declares, something else provisions:**

```go
// internal/skill
type Provisioner interface{ Requirements() []Requirement }

type Requirement struct {
    ID      string   // "whisper-model"
    Kind    string   // "model" | "binary"
    Options []Option // what the user may pick between
    Active  string   // Option.ID currently on disk, "" if none
}

type Option struct {
    ID, Label   string
    Bytes       int64  // real size, shown before the user commits to it
    URL, SHA256 string // empty URL = not fetchable, instructions only (winget, brew)
    Note        string // "เร็ว แต่ไทยมั่ว" — why you would pick this one
}
```

A skill never downloads anything and never learns how. `internal/asset` owns the catalog, the download, the checksum check and placement under the §14 data root; the UI enumerates the registry for skills implementing `Provisioner` and renders one generic panel. The payoff is exactly what the owner asked for: **a new model is a new line in `Options`** — no UI change, no new code path — and a new asset-hungry tool is one method.

**Only the user downloads (owner picked).** No `asset_install` tool, not even one gated behind approval. Three reasons: spending 141MB of someone's bandwidth is a human decision that needs a progress bar and a cancel button, neither of which exists inside a tool call; a model handed an install tool will "helpfully" fetch on every failure, which is precisely what §31 refused to do; and the failure path already works — an error that names what is missing and where to go. A *read-only* "what is installed" tool may earn its place later; the install path itself never becomes a tool call.

**Never bundled — the constraint that holds all of this up.** The installer stays ~12MB, the number [docs/index.html](docs/index.html) advertises. Sizes measured by HEAD request 2026-07-25, no download: `ggml-tiny-q5_1.bin` 30.7MB · `ggml-tiny.bin` 74.1MB · `ggml-base-q5_1.bin` 56.9MB · `ggml-base.bin` 141.1MB. Bundling base means a 12× larger download for a capability most sessions never touch; the quantized variants exist precisely so the catalog can offer a cheap first step.

**Checksum discipline is not new here.** [desktop/build/windows/installer/project.nsi](desktop/build/windows/installer/project.nsi) already downloads Tesseract with a pinned SHA256; `internal/asset` inherits that rule rather than inventing one — a fetched asset that fails its hash is deleted, not used.

**First slice, proposed:** catalog + verified download + the whisper entries + `Requirements()` on `audioTranscribeSkill` + one settings panel, hung off the existing settings surface ([SETTINGS-PARITY-PLAN.md](SETTINGS-PARITY-PLAN.md)). `tesseract` and `ffmpeg` join as instruction-only entries (`URL` empty) so the panel tells the whole truth from day one instead of showing a single lonely row.

**Status:** `Proposed 2026-07-25 — shape approved by owner (name + user-only downloads), not built.` Open: whether `Active` is stored in preferences or inferred from what is on disk (§31 currently infers — any `ggml-*.bin` wins), and what the panel does when two models are present.

---

## 33. Decision — `internal/stt`: One Language for Every Speech Engine (2026-07-25)

**Trigger:** owner — *"สถาปัตยกรรมเราต้องรับโมเดลอ่านเสียงได้หลายรูปแบบ ... ไม่ว่าจะใช้โมเดลจากที่ไหน สถาปัตยกรรมอะไร ควรจะถูกแปรเป็นภาษาเดียวกันเพื่อทำงาน ... เราพร้อมแค่ไหนครับ"* — with the follow-up that the skills settings page has to become somewhere a user can actually configure this.

**Readiness, answered with evidence before anything was written:**

| What it needs | State when asked |
|---|---|
| A proven translate-many-into-one pattern | ✅ [internal/model/factory.go](internal/model/factory.go) already collapses 13 providers into 4 runtimes behind one `model.Provider`. The pattern was not hypothetical — it shipped. |
| Precedent for "same endpoint, different wire format" | ✅ `ModelWireFormat` in [internal/config](internal/config/config.go). |
| A way to pass user configuration into a skill | ❌ **Zero.** `RegistryOptions` had exactly one field, `SandboxRoot`. Nothing could be configured per skill, by anyone, ever. This was the real blocker — not the engine work. |
| `audio_transcribe` accepting more than one engine | ❌ §31 hardcoded whisper.cpp: binary names, flags and output format all inline in the skill. |
| A settings surface to hang it on | 🟡 ~70%. [Settings.svelte](desktop/frontend/src/lib/Settings.svelte) already builds its nav from an array and has a "skills" section — but that section only lists, installs and removes *external* skills. No built-in skill has ever been configurable. |

**Decision — copy the pattern that already works.** [internal/stt](internal/stt) is `internal/model` in miniature: a `catalog` of `Descriptor`s (id, label, binary candidates, model glob, install hint), a `New(Options)` that switches on the descriptor and returns one `Engine` interface, and callers that never name a concrete engine. Adding faster-whisper, Vosk or sherpa-onnx is a catalog entry plus a constructor — no caller changes, no UI changes.

**The "one language" is `stt.Segment`** — `StartMs`, `EndMs`, `Text`. Every engine's output is translated into that, and nothing above the package knows that whisper prints `[HH:MM:SS.mmm --> ...]` on stdout while another engine will emit JSON. `audio_transcribe` shrank to what is genuinely its own job: sandboxing the path, producing a 16kHz mono WAV with ffmpeg, and formatting `[m:ss]` to match `video_ocr`. Milliseconds rather than a formatted string, because `[m:ss]` is one rendering choice, not the data.

**`RegistryOptions` gained `Speech stt.Options`** — the first configuration ever to reach a built-in skill from outside. That is the seam the settings UI plugs into; without it every UI design was impossible regardless of how it looked. `ponytail:` one field per configurable skill is fine at this count, becomes a map at three or four.

**Two model stores, never mixed** ([internal/stt/stores.go](internal/stt/stores.go)) — owner: *"ตำแหน่งโมเดลของระบบเราด้วย แบบแยกมา จะได้ไม่สับสน"*. **Managed** is `<DataRoot>/models`, the single directory Aetox downloads into and is allowed to delete from. **External** is everything else — Ollama, LM Studio, the HuggingFace cache, or any folder the user points at — discovered automatically (`KnownExternalStores`, env-var aware: `OLLAMA_MODELS`, `HF_HOME`) and **read-only to Aetox forever**. A user with 40GB of models already downloaded should not download again; Aetox still does not get to own someone else's directory. `InstalledModel.Managed` carries that distinction all the way to the UI, so a delete button can only ever appear on a file Aetox put there.

**Verified live, with real speech, on 2026-07-25.** whisper.cpp 1.9.1 installed via scoop, `ggml-base.bin` (141.1MB) downloaded into the managed store, and Windows SAPI used to synthesize a sentence. The full tool returned:

```
[0:00] Hello. This is Edak's testing the audio transcribe tool. The architecture gives every model a pair of ears.
```

Which closes §31's open item: the real binary's stdout format is exactly what the parser expects, and the flags are right. (base mishears the invented word "Aetox" as "Edak's" — a model-quality fact, not a bug.) It also showed whisper writes `load_backend:` / `read_audio_data:` chatter alongside the segments even under `-np`; the parser was already immune, since it ignores every line that does not start with `[`.

**Test structure follows the split.** The engine's real command line is pinned in `internal/stt` by a helper-process stub that fails unless it is handed an existing model, an existing `.wav`, `-l auto` and `-np`. The skill's own tests use a fake `stt.Engine` and assert what the skill owns — sandbox refusal, WAV conversion of an `.mp4`, `[m:ss]` formatting, temp cleanup. Neither needs a model downloaded.

**Status:** `Done 2026-07-25.` Full suite green; live-verified above. Next, proposed not built: the settings UI for this (owner: *"UI หน้าสกิลน่าจะต้องออกแบบใหม่ให้มันว้าว เผื่อสกิลอื่นอาจจะมีเคสแบบนี้ในอนาคต"*) and the §32 Asset download path that would make the model arrive from inside the app instead of a manual download.

---

## 34. Decision — A nil Go Slice Is a Frontend Crash: Fix It at the Boundary (2026-07-25)

**Trigger:** owner, with a screenshot — *"ทำไมกดไม่ได้อ่ะครับ"*. Clicking **Settings → สกิล** highlighted the nav item but the right-hand panel kept showing the previous section. Nothing looked broken; the page just looked dead.

**It was not a UI bug.** The chain, confirmed end to end on the owner's machine:

1. `~/.aetox/skills` does not exist (no external skills ever installed).
2. `scanSkills` declares `var found []DiscoveredSkill` and never appends — idiomatic Go, returns **nil**.
3. `ListExternalSkills` passed that straight through, and a nil slice marshals to JSON **`null`**, not `[]`.
4. `onMount` does `extSkills = await ListExternalSkills()` → the `$state<SkillRow[]>` now holds `null`.
5. Nothing renders it yet, so nothing fails — until the skills panel is opened, where `{#if extSkills.length === 0}` throws `TypeError`.
6. Svelte 5 aborts that update **mid-flush**. The nav button had already been updated in the same flush and keeps its `active` class; the panel never re-renders.

Which is why it reads as "the button doesn't work" rather than "something crashed" — the only visible evidence is a highlight that leads nowhere.

**Decision — convert at the last Go frame, not in `internal/`.** Returning nil for "nothing" is correct Go and every Go caller handles it; `internal/skill` and `internal/command` are not wrong. What cannot survive nil is the JSON boundary, so [desktop/jsonslice.go](desktop/jsonslice.go) adds `jsonSlice[T]` and the rule is: **a binding that hands a slice to the frontend returns it through `jsonSlice`.** Rejected: patching the six frontend load sites with `?? []`, which hides the next nil instead of fixing it.

**Swept the whole class, not the reported instance.** All 22 slice-returning `App` methods were checked. Exactly two were broken — `ListExternalSkills` and `ListCustomCommands` — and both are thin pass-throughs to an `internal/` function, added after the pattern was already understood elsewhere: `ListIdentityFiles` carries the comment *"non-nil so the frontend gets [] not null"* and every binding in `sessions.go` starts from `out := []T{}`. The knowledge existed in the codebase; nothing enforced it. So **Settings → คำสั่ง was one click away from the identical dead page** for any user with no custom commands.

**Enforcement is a test, not a convention.** [desktop/binding_slices_test.go](desktop/binding_slices_test.go) reflects over the bindings a test can safely call, in the environment that produces the bug — empty data root, empty home, an empty non-repo sandbox — and fails on any nil slice with the fix named in the message. It found both offenders before either fix was written, and it is what stops the third one.

**Status:** `Done 2026-07-25.` Full suite green; desktop rebuilt so the owner can confirm the page opens.

---

## Validation

1. **Claim traceability:** every claim above cites a file or an existing project doc; the two `Unverified`/`Inferred, Verify first: Yes` items are marked as such, not stated as fact.
2. **Scope alignment:** scope matches intake (whole-repo documentation at root); no expansion.
3. **Handoff readiness:** open questions (§7) and safe next actions (§6 "Direction" fields, all marked proposed/pending approval) are included.

**Artifact budget note:** this is a single consolidated file rather than the full multi-template package (overview/boundary/module-map/debt-register as separate files), because the request specified one location ("ที่ root") and the repo already has `docs/architecture/module-split-2026-07-21.md` and two ADRs covering deeper detail on the migration and cognition-engine designs — this file indexes and cross-links them instead of duplicating.
