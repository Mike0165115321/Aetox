# Browser Subsystem — Security Model

> **Date:** 2026-07-21
> **Status:** Implemented ([desktop/browser.go](../../desktop/browser.go))
> **Scope:** the native WebView2 browser tab feature (`desktop/browser.go`, `desktop/workbench.go`), specifically the `window.chrome.webview.postMessage` bridge between a loaded web page and Go.

## Why this exists

Each browser tab is a real WebView2 control (Chromium), embedded as a native
child window over the dock's browser pane. The user can browse it directly,
and the AI agent can read its content via `BrowserGetText` (the `browser_open`/
`browser_read` skills in `desktop/workbench.go`). Getting data out of the page
back to Go requires `window.chrome.webview.postMessage` — and that bridge is
available to **any script running in the page**, not just the two scripts we
inject (`metaScript`, `textScript`). A malicious or compromised page can call
it directly with a forged envelope at any time.

## Threat model

| # | Attack | Consequence if unguarded |
|---|---|---|
| 1 | Page calls `postMessage({__aetox:"meta", url:"https://accounts.google.com/login", title:"Google"})` — a URL/title that has nothing to do with the page's real address. | The app's address bar shows a URL the tab isn't actually at. This breaks the one guarantee a browser chrome exists to provide (users trust the address bar to know what site they're on) — a direct phishing enabler. |
| 2 | Page calls `postMessage({__aetox:"text", text:"<attacker-authored instructions>"})` unprompted, or replays/races a stale response. | `BrowserGetText` (the AI agent's read path) returns attacker-controlled text instead of the real page content — a prompt-injection vector delivered through the browser feature specifically (distinct from #3 below). |
| 3 | Page's real, visible DOM content itself contains adversarial instructions aimed at the agent (e.g. hidden text: "ignore previous instructions and…"). | The agent may follow instructions embedded in a page it visited. |

**#3 is out of scope for this document.** It's inherent to any "AI reads a live web page" feature — no message-transport check can fix it, because the content is genuinely what's in the page's own DOM. It has to be handled at the agent/prompt layer (treat fetched page text as untrusted data, never as instructions) — not fixed here.

**#1 and #2 are transport-layer forgery, not content-trust problems, and are fixed by this change.**

## Defense (three checks, `desktop/browser.go`)

### 1. Origin cross-check (closes #1)

`args.GetSource()` on `ICoreWebView2WebMessageReceivedEventArgs` is provided by the WebView2 runtime itself — the sending frame's real origin, which page script cannot forge. `onMessage` now requires `sameOrigin(source, m.URL)` before trusting a `"meta"` message's claimed URL/title. A page can still freely set its own `document.title` (true in every browser — not a new risk), but it can no longer claim to *be* a different origin.

```go
func sameOrigin(a, b string) bool {
    ua, err1 := url.Parse(a)
    ub, err2 := url.Parse(b)
    if err1 != nil || err2 != nil || ua.Scheme == "" || ua.Host == "" {
        return false
    }
    return ua.Scheme == ub.Scheme && ua.Host == ub.Host
}
```

### 2. Per-request nonce (closes #2)

Every `BrowserGetText` call mints a random token (`newMessageToken`, `crypto/rand`, 16 bytes), embeds it in the `textScript` it evaluates, and only delivers a `"text"` message whose `token` matches the one just minted for *that specific call*. A page's own unsolicited or replayed `"text"` message — even a same-origin one — cannot be mistaken for the response to a request that hasn't been asked yet, or one that already completed.

### 3. No-pending-request rejection (pre-existing, extended)

`textCh` is only non-nil while a `BrowserGetText` call is in flight, and is cleared the instant a message is consumed (or the 5s timeout fires) — so a message arriving with nothing waiting is dropped. This existed before; the token check (#2) closes the gap it didn't cover (a forged message arriving *while* a real request is in flight would previously have been accepted as if it were the real answer).

## What changed

- `aetoxMsg` gained a `Token` field.
- `metaScript` unchanged; `textScript` became `textScript(token string) string`.
- `onMessage(id, tab, raw)` → `onMessage(id, tab, raw, source string)` — `source` comes from `args.GetSource()`, threaded through from `MessageCallback`.
- `browserTab` gained `textToken string`, guarded by the existing `textMu`.
- Tests: [desktop/browser_test.go](../../desktop/browser_test.go) — `sameOrigin`, `newMessageToken` uniqueness, and `onMessage` accept/reject cases for both spoofed-origin and wrong-token messages.

## Residual risk (explicitly not fixed here)

- Threat #3 above (agent trusting adversarial page content) — needs an agent-layer mitigation, tracked separately, not a browser-transport fix.
- `document.title` spoofing within a page's own real origin — this is normal, expected browser behavior (every browser lets a page set its own title), not a vulnerability.
- WebView2/Chromium's own CVE surface — mitigated by keeping the WebView2 Evergreen runtime updated (OS-managed), outside this codebase's control.
