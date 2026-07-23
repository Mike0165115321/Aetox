# Browser Subsystem — Native Embedding, Failure Catalog, and the macOS/Linux Port Blueprint

> **Date:** 2026-07-24
> **Status:** Implemented on Windows ([desktop/browser.go](../../desktop/browser.go)); macOS/Linux are design-only (see last section)
> **Scope:** how a browser tab becomes a real native window glued over the workbench pane — window creation, threading, z-order, visibility, `file://` support — plus every failure mode we actually hit, so the next platform backend doesn't rediscover them.
> **Companion:** [browser-security-2026-07-21.md](browser-security-2026-07-21.md) covers the page↔Go message bridge's security model.

## Why native windows at all

Iframes can't render most of the real web (`X-Frame-Options`/CSP deny on
YouTube, Google, anything with bot checks), and the AI needs to read real page
content (`BrowserGetText`). So each tab is a full WebView2 (Chromium) control
in a **Win32 child window positioned over the pane's DOM rect** — a second
compositor stacked on top of the app's own webview, kept in sync by frontend
geometry reports.

## Architecture (Windows)

```
BrowserPane.svelte ──(rect, url, visible)──► wails bindings (BrowserOpen/Navigate/SetBounds/SetVisible/Close)
        ▲                                            │
        │ browser:meta events (title/url sync)       ▼
        └──────────────────────────────── browserHost (ONE dedicated STA thread)
                                                     │ command queue + PostThreadMessage(WM_APP) wake-up
                                                     ▼
                                        per-tab: Win32 child window + WebView2 controller
```

- **Threading:** WebView2 is COM/STA. Every webview lives on one OS thread
  running a Windows message pump (`browserHost.run`). All operations marshal
  onto it via `h.do(fn)`. Never touch `chromium.*` from any other thread.
- **Geometry contract:** the frontend sends **physical pixels**
  (`getBoundingClientRect × devicePixelRatio`), relative to the main window's
  client area. Any future backend must keep this contract or scale internally.
- **Data dirs:** `<DataRoot>/webview/app` is the app's own webview;
  `<DataRoot>/webview/browser` is shared by all tab webviews. Separate on
  purpose — clearing browsing state must never touch the app shell's state.
- **Lifecycle:** the native window is created on the tab's first non-empty URL,
  destroyed by `BrowserClose` (tab close / session switch). `CloseAllBrowserTabs`
  runs on every frontend mount because an HMR full-reload wipes JS state without
  running `onDestroy`, orphaning native windows.

## The z-order / visibility rule

Two WebView2 controllers in one top-level window composite independently — the
app's own webview can end up painted **above** a tab that is genuinely loaded,
leaving it invisible with no error anywhere.

**Rule: visibility is enforced natively in `NavigationCompletedCallback`**
(`SetWindowPos(HWND_TOP, SWP_SHOWWINDOW|SWP_NOACTIVATE)`, skipped for hidden
tabs). It must NOT depend on the page's own JS posting a message back: that
chain (`metaScript` → origin check → frontend → `BrowserSetBounds`) breaks for
any page whose meta message is dropped — which was every `file://` page until
`sameOrigin` learned that file URLs have no host. The meta chain still runs,
but only for title/URL sync, where losing it degrades gracefully.

## `file://` pages

- Address bar input is normalized in ONE place — `normalizeUrl` in
  [Workbench.svelte](../../desktop/frontend/src/lib/workbench/Workbench.svelte):
  drive paths (`E:\x\y.html`) → `file:///E:/x/y.html`, schemes pass through,
  bare hosts get `https://`. Never blindly prefix `https://` — that produced
  `https://file:///…`, a valid-looking URL for a host literally named "file",
  which loads a blank page forever with no error.
- `sameOrigin` special-cases `file↔file` (no host to compare; a file page
  "spoofing" another local path cannot impersonate a trusted site, which is
  what the check exists to prevent).
- WebView2 navigates `file://` URIs natively; no extra flag needed. (macOS
  WKWebView will need `loadFileURL:allowingReadAccessTo:` instead — see below.)

## Failure catalog — everything that actually broke (2026-07-24 debugging session)

Each of these was real, and each was **silent** until instrumented. Keep the
`debuglog` lines in `open()`/`start()`/`run()`: they turned a half-night of
guessing into a five-minute read.

| # | Symptom | Root cause | Fix / rule |
|---|---|---|---|
| 1 | Every tab blank; no native webview process ever spawned; broke at some point and survived app restarts | `FindWindowW(nil, "Aetox Desktop")` — looking up the parent window **by title** — matched a foreign process's window that happened to carry that text (an `explorer.exe` taskbar-thumbnail host; a dev-URL browser tab titled by our own `<title>` can do it too). A parent HWND from another process makes `CreateWindowExW(WS_CHILD)` fail with `ERROR_ACCESS_DENIED`. | **Never identify our own window by title.** `findOwnMainWindow()` enumerates top-level windows and matches by `GetWindowThreadProcessId == os.Getpid()` + visible. Title-based lookup is banned in this subsystem. |
| 2 | (Same symptom as #1 — masked it for hours) | `open()` swallowed every failure: `CreateWindowExW == 0 → return`, `Embed() == false → return`, no log, and `BrowserOpen` resolves fine because the work is queued fire-and-forget. | Every native failure path logs to `debuglog` with the tab id. A silent `return` in native glue code is a bug by itself. |
| 3 | `file://` tab loads (in hindsight) but stays invisible; https pages fine | Visibility depended on the page's meta message surviving the origin check; file URLs have no host so the check dropped them (see z-order rule above). | Native-side `SetWindowPos` on `NavigationCompleted`; `sameOrigin` file↔file case. |
| 4 | Typed local paths never load at all | Address bar prefixed `https://` onto everything, including `file:///…` and bare drive paths. | `normalizeUrl` (single choke point, unit-checked). |
| 5 | Tabs die after vite HMR full-reload; window floats frozen at its last rect | JS `workbench` store wiped without `onDestroy` → orphaned native window | `CloseAllBrowserTabs` on frontend mount. |
| 6 | Drag-resize past the pane edge never ends; panel keeps growing | `pointerup` delivered to the native tab window (a different OS window!) instead of the app | `setPointerCapture` on the resize handle ([App.svelte](../../desktop/frontend/src/App.svelte)). |
| 7 | (Preventive) child creation can also fail cross-DPI-context | A raw goroutine thread runs on the process-default DPI awareness context, not necessarily the main window's | `run()` sets the thread's DPI context to the parent window's before creating windows. |

Debugging heuristics that paid off, in order: **read the subsystem's debug log
→ if empty, instrument the silent paths → check `GetLastError` verbatim → check
process identity of every HWND involved.** The winning probe was logging
`parentPid` next to `selfPid` — one line, instant diagnosis.

## macOS / Linux port blueprint

Everything **above** the native seam ports unchanged: BrowserPane/Workbench
frontend, the bindings' signatures, URL normalization, the message-bridge
security model (origin cross-check + per-request tokens), session-bound tab
restore. What each platform must reimplement is exactly the `browserHost`
surface:

```
open(id, url, x, y, w, h)   navigate(id, url)   setBounds(id, …)
setVisible(id, bool)        close(id)           eval(id, js)
+ a script-message callback feeding onMessage(id, tab, raw, sourceOrigin)
```

Extract that as a Go interface with the Windows implementation behind a
`//go:build windows` tag; each platform file provides its own host. The
`onMessage` envelope handling, token minting, and `sameOrigin` stay shared code.

**macOS (WKWebView):**
- No separate OS window and no z-order war: add the `WKWebView` as a plain
  **subview** of the wails `NSView`, frame = pane rect. Problems #1, #3, #6
  from the catalog structurally cannot happen. Use logical points, not physical
  pixels — divide the frontend's physical rect by `devicePixelRatio` (keep the
  wire contract, convert in the host).
- Bridge: `WKScriptMessageHandler` (`window.webkit.messageHandlers.aetox.postMessage`)
  replaces `window.chrome.webview.postMessage`; the injected scripts need a tiny
  shim choosing whichever bridge exists. Real origin comes from
  `WKScriptMessage.frameInfo.securityOrigin` — same trust property as
  WebView2's `GetSource()`.
- `file://`: `loadFileURL:allowingReadAccessTo:` (grant the file's parent dir),
  plain `load:` will refuse local files.
- Threading: all WKWebView calls on the main thread (GCD main queue) — the
  "one thread owns the webview" model carries over, it's just the main thread.

**Linux (WebKitGTK):**
- Wails v2 already renders through WebKitGTK, so the toolkit is present. Add a
  second `WebKitWebView` widget in a `GtkOverlay`/`GtkFixed` over the pane rect
  (again: a widget, not a separate window — same structural wins as macOS).
- Bridge: `WebKitUserContentManager` script messages; origin from the message's
  `WebKitFrame`/URI. `file://` loads need
  `webkit_web_view_load_uri` + per-context security settings if reading other
  local files from the page.
- Threading: GTK main loop thread only.

**Shared porting rules (the actual lessons):**
1. Never locate your own window/view by title or any ambient global — hold a
   direct handle from the toolkit.
2. Every native failure logs with tab id; no silent returns.
3. Page JS is never load-bearing for visibility/geometry — only for
   title/URL sync.
4. Keep the frontend→host geometry contract (physical px) fixed; convert
   inside each host.
5. One thread owns each webview; marshal everything.
