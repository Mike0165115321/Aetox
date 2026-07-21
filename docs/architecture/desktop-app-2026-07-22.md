# Desktop App — Deep Dive

> **Date:** 2026-07-22 · **Status:** Direct (all files in `desktop/` and `desktop/frontend/src/{App.svelte,style.css,lib/workbench/*}` read in full this session)
> **Scope:** layer 5 of the 5-layer reading map in [ARCHITECTURE.md](../../ARCHITECTURE.md) — the Wails + Svelte 5 GUI. The most independently-developed layer as of this session: 3 real bug classes found and fixed here (native-window Z-order, `postMessage` forgery, resize-handle stuck-drag), plus one found-but-not-yet-fixed (`SearchSessions`).

This doc exists because `desktop/` is real, substantial code (7 Go files, several hundred lines of Svelte/CSS) that [ARCHITECTURE.md](../../ARCHITECTURE.md) can only summarize in a table row per file. Read this before touching session persistence, the browser tab, or the resizable panel layout.

---

## 1. Backend (`desktop/*.go`, package `main`)

| File | Owns | Depends on |
|---|---|---|
| `main.go` | Wails bootstrap: embeds `frontend/dist` (`//go:embed all:frontend/dist`), wires `app.startup`/`app.shutdown`. No logic to speak of. | — |
| `app.go` | The Wails-bound `App` struct — a different type from `internal/app.App` (§2). `bootstrapFromConfig` constructs one `internal/app.App` + one `cognitive.Agent` per process (not the orchestrator, see `ARCHITECTURE.md` §10). Also: project tree/file read-write for the Files pane (`ProjectTree`, `ReadFile`/`WriteFile`, sandboxed via `safeSandboxPath` — same traversal-guard pattern as `internal/skill`'s `resolveSandboxPath`), provider/model switching, `GitChangedFiles` (shells out to `git status --porcelain`). | `internal/app`, `internal/cognitive`, `internal/config`, `internal/model`, `internal/skill` |
| `db.go` | One process-wide SQLite handle (`sync.Once`), schema (`sessions`, `messages`, `messages_fts` — FTS5 trigram tokenizer, for substring search that works across Thai/English without word-boundary tokenization). `App.dbDir` overrides the default `<UserConfigDir>/aetox` directory — added this session purely as a test seam (empty = unchanged production path). | `modernc.org/sqlite` (pure Go, no CGO) |
| `sessions.go` | Session persistence on top of `db.go`: `appendTurn` (write), `ListSessions`/`LoadSession` (read), `SearchSessions` (**broken**, §3), keyed per-project via `projectKey` (basename + short SHA-1 hash, so two folders both named `app` don't collide). | `db.go`, `internal/model` (message role conversion) |
| `browser.go` | Native WebView2 tab: a real, separate Win32 child window overlaid on the dock's browser pane (not an iframe — iframes can't render sites with `X-Frame-Options` deny). Direct Win32 syscalls (`CreateWindowExW`, `SetWindowPos`, message pump on a dedicated OS thread). Security model (the `postMessage` bridge) is a separate document: [browser-security-2026-07-21.md](browser-security-2026-07-21.md). | `github.com/wailsapp/go-webview2` |
| `workbench.go` | `browser_open`/`browser_read` as `skill.Tool` implementations — the *agent's* way of driving the browser, distinct from the user-facing `BrowserOpen`/`BrowserNavigate` etc. in `browser.go`. This is the closest existing example of a non-trivial `skill.Tool` wrapping an external process/UI — cited in `MCP-SUPPORT-PLAN.md` as the pattern an MCP adapter should follow. | `browser.go`, `internal/skill` |
| `terminal.go` | Embedded shell sessions via ConPTY (`github.com/UserExistsError/conpty`) — a real pseudo-console per tab, output streamed to the frontend as `terminal:data:<id>` events. Independent of `internal/skill/shell.go` (that's the *agent's* `shell` tool; this is the *user's* terminal pane). | `github.com/UserExistsError/conpty` |

### 1.1 Two `App` types, same name, different jobs

`internal/app.App` (CLI orchestration + terminal presentation — see [ARCHITECTURE.md §6.1](../../ARCHITECTURE.md#61-internalapp-mixes-orchestration-with-cli-terminal-presentation)) and `desktop.App` (this layer, Wails-bound) are unrelated types that happen to share a name because they live in different packages. `desktop.App.SendMessage` calls exactly one method on the former (`a.chat.RunOnce`) — nothing else. Don't confuse a stack trace or a grep hit for one with the other.

---

## 2. Frontend (`desktop/frontend/src/`)

### 2.1 Layout: a 3-column, resizable CSS Grid

`App.svelte` defines the whole window as one grid: `sidebar | 6px handle | main (chat) | 6px handle | inspector (workbench)`. `main`'s column is `minmax(360px, 1fr)` — a hard floor enforced by the CSS grid track-sizing algorithm itself, independent of anything in JS.

Sidebar and inspector widths are user-resizable (drag the 6px handles), persisted to `localStorage`. Two real bugs were found and fixed here this session (full detail in [ARCHITECTURE.md §6.8](../../ARCHITECTURE.md#68-frontend-layout-bugs--fixed-2026-07-22), summarized):

1. **Stuck-drag / unbounded growth.** Dragging a handle across the native WebView2 browser window (§1, `browser.go`) let the OS deliver the drag-ending `pointerup` to that separate native window instead of back to the DOM — the drag state never cleared, and the panel kept growing on any later mouse movement. Fixed with `setPointerCapture` on the handle at drag start.
2. **No maximum width at all.** `clampSize` only enforced a floor (`Math.max(min, px)`) by original design, to avoid squeezing `main` below its own grid floor. Fixed by computing a real ceiling at drag time instead: `window.innerWidth − (other panel's current width) − 360 (main's floor) − 12 (two handles)` — same protection for `main`, without allowing runaway growth.

A third, unrelated CSS bug (blank-state text clipping instead of wrapping at narrow widths — `align-items:center` sizing text nodes to content width instead of the container) is also fixed, same ARCHITECTURE.md section.

### 2.2 `Workbench.svelte` / `BrowserPane.svelte` — bridging Svelte state to a native window

The workbench tab strip (`workbench.svelte.ts` store) is ordinary Svelte state — but the `browser` tab kind is not: its actual pixels come from the native WebView2 window in `browser.go`, not from anything in the DOM. `BrowserPane.svelte` is the bridge:

- On first URL, calls `BrowserOpen(tab.id, url, ...physRect(el))` — `physRect` converts the pane div's `getBoundingClientRect()` (CSS pixels) to physical pixels (`× window.devicePixelRatio`) for the Win32 `CreateWindowExW` call.
- A `ResizeObserver` on the pane div keeps the native window's bounds in sync (`BrowserSetBounds`) whenever the div resizes — including the resize-handle drag from §2.1, which is exactly why that bug was visible at all (Z-order made the tab visible; only then did its bounds tracking need to be correct).
- `BrowserSetVisible` hides the native window when its tab isn't active, or when the Settings overlay is open — because it's a real OS window, it would otherwise float above Svelte UI that's meant to be in front of it.

**Rule of thumb when debugging anything that looks like a browser-tab rendering glitch:** first determine whether the bug is in the Svelte/CSS layer (DOM, doesn't need Wails) or the native-window layer (`browser.go` + this bridge, needs an actual rebuild+run to observe — `svelte-check`/`go vet` can't catch native-window positioning or Z-order issues at all).

---

## 3. Known issues (see [ARCHITECTURE.md](../../ARCHITECTURE.md) for full evidence/severity)

| Issue | Status |
|---|---|
| `SearchSessions` FTS5 query errors on every call (`snippet()` used in a joined query — `modernc.org/sqlite` limitation), silently returns nothing | **Open** — root cause confirmed, fix verified, not yet applied. §6.7. |
| Browser tab Z-order (invisible-but-working tabs) | Fixed. §6.6. |
| `postMessage` forgery (address-bar spoofing, fake agent-read content) | Fixed. §6.6 + [browser-security-2026-07-21.md](browser-security-2026-07-21.md). |
| Resize-handle stuck drag / unbounded panel growth | Fixed. §6.8. |
| Blank-state text clipping | Fixed. §6.8. |

## 4. Test coverage

Full per-file breakdown lives in [TEST-REPORT.md](../../TEST-REPORT.md) Module 5 — not repeated here. One structural fact worth stating in this doc directly: `TerminalStart`/`TerminalClose` and all of `browser.go`'s Win32 window plumbing are **not unit-testable**, ever, as currently written — `wailsruntime.EventsEmit` calls `log.Fatalf` (`os.Exit(1)`, unrecoverable) when its context isn't a real Wails-bound one, which a `go test` process never has. Tests that exist for `terminal.go`/`browser.go` work around this by testing pure helpers (`sameOrigin`, `newMessageToken`, `nextTerminalID`) and by driving `conpty`/`onMessage` directly, bypassing the Wails-context-dependent entry points entirely.

## Related documents

- [ARCHITECTURE.md](../../ARCHITECTURE.md) — whole-repo map; §4.2 has the file table this doc expands on, §6.6/§6.7/§6.8 have full evidence for the issues in §3 above.
- [browser-security-2026-07-21.md](browser-security-2026-07-21.md) — the `postMessage` threat model in full; not repeated here.
- [TEST-REPORT.md](../../TEST-REPORT.md) Module 5 — test coverage per file.
