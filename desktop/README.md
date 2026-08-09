# desktop/ — Wails GUI front end

> Module map: [ARCHITECTURE.md §4.2](../ARCHITECTURE.md) · Deep dives: [desktop-app](../docs/architecture/desktop-app-2026-07-22.md) · [browser-security](../docs/architecture/browser-security-2026-07-21.md) · [tesseract bundling](../docs/architecture/tesseract-ocr-bundling-2026-07-22.md) · Tests: `TEST-REPORT.md` Module 5

**What it is:** the Wails v2 + Svelte 5 desktop app — chat, tabbed workbench (Review/Terminal/Files/Browser), SQLite session persistence, and an agent-driven native browser. Embeds the same engine as the CLI (`internal/app.NewApp` + `RunOnce`); everything else here is surface.

**Desks (ARCHITECTURE.md §83/§84, product picture in [COMPANY.md](../COMPANY.md)):** a session is opened at a desk and stays there. The desk decides what the dispatcher shows the model, what direction and which memory the system prompt carries, and the ceiling over sub-agents — all built at bootstrap from `App.desk`, so changing desks re-bootstraps the engine the same way switching projects does. `nil` is the full desk: every session from before v8, unchanged down to the byte.

## Dev commands

- `wails dev` (or [wails-dev.bat](wails-dev.bat)) — live dev, frontend hot reload; browser dev server at `http://localhost:34115`. The `.bat` also sets `AETOX_DATA_ROOT` so repeated dev runs don't grow Aetox's own data (preferences, sessions, WebView2 profiles, the downloaded rtk binary) in `%AppData%` without bound — see `internal/config.DataRoot` and [ARCHITECTURE.md §14](../ARCHITECTURE.md). Production builds don't set this env var, so shipped behavior is the normal `%AppData%\aetox` default.
- `wails build` — production `desktop.exe`.
- `go test ./...` here + `npx svelte-check` in `frontend/` — run both before calling a change done.
- Go's own `GOCACHE`/`GOMODCACHE` are machine-wide settings, not project config — if they're growing on `C:`, `go env -w GOCACHE=<path>` / `GOMODCACHE=<path>` moves them anywhere, for every Go project on the machine, not just this one.

## Go side (package main)

| File | Role |
|---|---|
| [app.go](app.go) | Wails-bound `App` (distinct from `internal/app.App`). `bootstrapFromConfig` wires engine + workbench tools + MCP (concurrent per-server connect, 8s-bounded — a slow `npx` server used to block the whole UI, see the timing checkpoints via `debuglog.Block` added 2026-07-23); provider/model/approval switching (a switch inherits the outgoing agent's real context — tool calls included, §27.1); project tree/file IO; chat attachments; `CancelTurn` (wired to the composer's ■ Stop button since 2026-07-25 — the brake for the unbounded tool loop); `TestProviderConnection` (1-token real completion = endpoint+key+wire-format check); `SendMessage` emits `agent:status`, `agent:tool`, `agent:chunk` (reply text), and `agent:reasoning` (live thinking tokens, when the provider streams them) to the frontend, and persists reasoning + think-duration on the session turn. |
| [ask_user.go](ask_user.go) | Interactive UI tools (§27.3): `ask_user` (model blocks on a question + option cards; `AnswerUserQuestion` delivers the pick; one in flight, cancel unblocks) and `todo_write` (model-maintained checklist → `todo:update` event). Registered with the workbench tools; `ask_user` is exempt from the turn executor's 60s slow-tool guard. |
| [sessions.go](sessions.go) / [db.go](db.go) | Per-project session persistence (SQLite + FTS5 search), transcript ↔ `model.Message` restore. Also the desk a session was born at (`sessions.mode`, schema v8): `NewSessionAt` opens one at a desk, `ListSessionsAt` filters the history behind a button, and `LoadSession` returns the engine to the desk that conversation was held at. There is deliberately no binding that changes a live session's desk (ARCHITECTURE.md §83). |
| [office.go](office.go) | The desk picker (`ListModes`), the office roster (`ListChairs` — sub-agent profiles that declare `desk: specialized`, listed with the tools they get *after* the ceiling) and the received-work feed (`ListReceivedJobs`, a query over `jobs` where `parent_ref` is set). No new state: a desk is a file, a chair is a file, the feed is the record §82 already writes. |
| [artifacts.go](artifacts.go) | `ListArtifacts` — the ผลงาน page, swept live from `<root>/output/<session>` across the unfocused root and every project opened. The disk is the index on purpose: an index that can disagree with the folder shows files that are gone and hides files that are there. |
| [browser.go](browser.go) | Native WebView2 tab host via raw Win32 syscalls — Windows-only. Read [browser-security](../docs/architecture/browser-security-2026-07-21.md) **before** touching `onMessage`/`metaScript`/`textScript`. `CloseAllBrowserTabs` (called once by the frontend on load) kills any native tab window orphaned by a previous frontend lifetime — a `wails dev` full reload wipes the JS-side `workbench` store without ever running `BrowserPane`'s `onDestroy`, so the Go-side window would otherwise float at its last position forever. |
| [workbench.go](workbench.go) | The workbench tools as `skill.Tool` (agent-facing) — registered `SourceExternal`. Holds the four browser implementations behind the ref-tagging pattern. |
| [browser_tool.go](browser_tool.go) | The one `browser` tool the model sees: actions `open`/`read`/`click`/`type`, gated per action on the old `browser_*` names so `tools:` and `categories:` still narrow. |
| [engine_server.go](engine_server.go) | `n8n_server_start` / `windmill_server_start` — the agent checks and starts its own engine with the command saved in Settings. |
| [terminal.go](terminal.go) | User-facing terminal pane (ConPTY) — independent of the agent's `shell` tool. |
| [main.go](main.go) | Wails bootstrap. |

## Frontend (`frontend/src/`)

- **The five buttons** ([lib/desks.ts](frontend/src/lib/desks.ts)) — the nav row at the top of the sidebar. Two desks open a session (`openDesk` → `NewSessionAt`, and re-clicking the desk you are already at is a no-op on the conversation), two are full-window pages, ทำงานอัตโนมัติ is present and disabled. The pages ([lib/Office.svelte](frontend/src/lib/Office.svelte), [lib/Artifacts.svelte](frontend/src/lib/Artifacts.svelte)) render through the same `.settings-overlay` seam Settings has always used, so the chat layout underneath is untouched — the whole surface is additive. The list is fixed rather than built from `ListModes`: a fourth mode file is a fourth desk in the engine, not a fourth button in the product.
- [App.svelte](frontend/src/App.svelte) — layout, panel resize/collapse, Wails event subscriptions (`agent:status`, `agent:tool`, `agent:chunk`, `agent:reasoning`), file drop, `CloseAllBrowserTabs()` on mount.
- `lib/stores/cockpit.svelte.ts` — the single reactive state tree (chat, tool timeline steps, model info); seeds `cockpit.model` from a `localStorage` cache synchronously on load so first paint shows real dropdowns instead of an empty/loading state, then corrects it once the real `GetModelInfo()` call resolves. `lib/stores/workbench.svelte.ts` — workbench tabs.
- `lib/Chat.svelte` — chat, live tool timeline, live reply + reasoning streaming (unbounded height; finished messages keep their thinking collapsed under a "คิดเป็นเวลา Xs" toggle), pinned auto-scroll (follows the stream at the bottom, unpins when the user scrolls up), ask_user option cards (composer doubles as free-text answer), todo checklist panel, per-message copy button, and the ■ Stop button (send button morphs while a turn runs). A `file`/`browser` workbench tab can be dragged into the composer to inline its content as context (`attachTabContext`).
- `lib/workbench/` — Review/Files/Browser panes; drag source for the context-attach feature above (`Workbench.svelte`'s tab strip, `draggable` on file/browser tabs).
- `lib/locales/{th,en}.ts` — i18n (add keys to **both**).
- Generated Wails bindings live in `frontend/wailsjs/` — regenerated by `wails dev`; if you add a Go method mid-session, mirror it there by hand or rerun `wails dev`.

## Rules of thumb

- Engine behavior does not belong here — if it isn't presentation, persistence, or a native-window concern, put it in `internal/*`.
- `wailsruntime.EventsEmit` requires a real Wails context — code paths that tests reach must guard `a.ctx != nil` (see `TEST-REPORT.md` on what's structurally untestable).
- **A binding that returns a slice returns it through `jsonSlice()`** ([jsonslice.go](jsonslice.go)). A nil Go slice marshals to JSON `null`, the frontend does `.length` on it, and Svelte aborts the render mid-flush — the page looks unclickable rather than broken (ARCHITECTURE.md §34). `binding_slices_test.go` enforces it.
