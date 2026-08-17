# Browser Subsystem — The Rules the Compiler Should Be Holding

> **Date:** 2026-08-17
> **Status:** All four defects **implemented** 2026-08-17.
> **Scope:** the native browser tab feature end to end — [desktop/browser.go](../../desktop/browser.go), [desktop/browser_windows.go](../../desktop/browser_windows.go), [desktop/browser_tool.go](../../desktop/browser_tool.go), [desktop/browser_shot.go](../../desktop/browser_shot.go), [desktop/browser_pick.go](../../desktop/browser_pick.go), the browser half of [desktop/workbench.go](../../desktop/workbench.go), and `normalizeUrl` in [workbench.svelte.ts](../../desktop/frontend/src/lib/stores/workbench.svelte.ts).
> **Does not revisit:** the postMessage threat model, which is settled in [browser-security-2026-07-21.md](browser-security-2026-07-21.md) and untouched by anything proposed here.

## Why this exists

Three bugs in two days. Not three mistakes — one, made three times.

| Bug | The rule that was broken | Where that rule was written down |
|---|---|---|
| The agent's `read`/`click`/`type` landed on tabs the **user** opened (§127.1) | `lastID` is "the tab in front", never "the agent's tab" | nowhere at all |
| Every page after the first in a session timed out at 20s (§127.8) | a webview method must be called on the thread that owns it | a comment on `tabView`, and the header of `browser_shot.go` |
| The agent opened tab after tab and stranded each one (2026-08-10) | the agent browses in one tab | a comment inside `workbenchOpenBrowser` |

Every one of these compiled. Every one passed review. Every one shipped. The rules are all correct and all written in prose that only helps a person who already went looking for it — which, by definition, is not the person about to break it.

The engine's own words for the second one are worth keeping, because they are what a violated convention sounds like from the outside:

```
browser tab web-agent-3 error: This method can only be called from the thread that created the object.
page did not finish loading: https://www.wikipedia.org
```

A sentence that reads like a slow website, produced by a call the engine threw away 20 seconds earlier.

**The thesis of this document: every rule in that table can be moved from a comment into a type, and the ones that can be, should be.**

## The subsystem as it stands

Three layers, and the split between them is good. Nothing below proposes changing it.

```
 the agent                the user
     │                        │
 browser_tool.go         Workbench.svelte  ← two callers
     │                        │
     └────────┬───────────────┘
              │
        App.Browser*         ← ~16 Wails bindings, desktop/browser.go
              │
        browserHost          ← portable: tabs map, lastID/agentID, message routing
              │
        hostBackend.do       ← the thread that owns webviews
              │
        tabView              ← one platform's live webview  (win32Tab today)
```

`hostBackend`/`tabView` keep every engine detail behind an interface, which is why the Linux and macOS ports in [PLATFORM-SUPPORT.md](../../PLATFORM-SUPPORT.md) are a file each rather than a rewrite. That boundary stays exactly as it is.

What follows are four defects **inside** this shape, none of which require moving it. The first three are about rules the compiler should hold. The fourth is about what this subsystem tells the outside world when one of them is broken, and it is the reason the other three cost what they did.

---

## Defect 1 — the thread rule is a comment, and the door has no wall beside it

> **Built 2026-08-17**, as written below. `browserTab.view` is gone, `browserHost.views` holds them, `withTab` became `onTab` and is the only way to reach one. Seventeen call sites moved; `tab.view.navigate(url)` no longer compiles.

`tabView` says so itself, in the only place it can:

```go
// tabView is one platform's live webview for one tab. Every method is called
// on the thread that owns the webview — that is, from inside hostBackend.do.
type tabView interface { navigate(url string); eval(js string); … }
```

And `browserHost.withTab` is the door that honours it:

```go
func (h *browserHost) withTab(id string, fn func(*browserTab)) {
	h.backend.do(func() { if t := h.tab(id); t != nil { fn(t) } })
}
```

The door is fine. The problem is that **`browserTab.view` is an ordinary field on an ordinary struct**, so the wall beside the door is missing: anything holding a `*browserTab` can reach the engine without going through `do`, and the compiler is content. Sixteen call sites walked through the door. One walked past it, and that one was invisible until an agent tried to open two pages in a row.

### What it should be

Take the view off `browserTab` entirely. Views live in a map the host owns, reachable only from inside a host-thread callback:

```go
type browserTab struct {
	// no view field
	alive bool        // what the nil-check used to answer
	navMu sync.Mutex
	…
}

type browserHost struct {
	tabs  map[string]*browserTab
	views map[string]tabView   // never handed out, never read off the host thread
}

// onTab is the ONLY way to reach a webview. fn runs on the owning thread; the
// view it is handed is not valid after fn returns.
func (h *browserHost) onTab(id string, fn func(tabView, *browserTab)) {
	h.backend.do(func() {
		if v, t := h.views[id], h.tabs[id]; v != nil && t != nil { fn(v, t) }
	})
}
```

That is `withTab` with one thing added: there is no longer a second way. The line that cost two hours yesterday —

```go
tab.view.navigate(url)
```

— stops being a bug that ships and becomes a compile error, because `tab.view` does not exist.

This does not stop somebody determined from copying the `tabView` out of the callback and calling it later. Nothing in Go will. It stops the accident, and the accident is what happened: nobody chose to call the engine from the wrong thread, they just reached for the nearest field.

### What it costs

Mechanical. `browserTab.view` has around a dozen readers; each becomes an `onTab` body, and most already sit inside a `do` that can be replaced outright. `browser_shot.go`'s channel-of-a-channel keeps its exact shape — asked inside, awaited outside — and simply asks through `onTab` instead of `backend.do` + `t.view`.

The liveness checks (`tab == nil || tab.view == nil`) become `alive`, which is more honest anyway: a tab with no view was never a tab, it was a failed `openTab` that got stored.

---

## Defect 2 — "which tab" is a `string`, so every tab is every other tab

> **Built 2026-08-17.** `AgentTabID` exists, `agentTab()` is its only constructor, and the two functions that used to answer this question — one with `""`, one with an error — are one.

Yesterday's fix split the host's one id field into two, because one field was answering two different questions:

- `lastID` — the tab in front. The frontend's question. Rewritten every time a tab is raised, **including by the user clicking their own**.
- `agentID` — the tab the agent is working. Nothing the user does can move it.

That fix is right and it is not enough, because both are `string`. `BrowserClickRef(id string, ref int)` will accept the user's tab id as happily as the agent's, and did. The split now lives in two functions that callers have to know to prefer, which is the same category of rule as the one in Defect 1: correct, written down, unenforced.

### What it should be

Ownership in the type:

```go
type TabID string        // any tab in the workbench
type AgentTabID TabID    // the agent's own tab

// The only constructor. Returns an error, not "", because every caller but
// `open` needs the refusal in words.
func (a *App) agentTab() (AgentTabID, error)
```

Go will not implicitly convert `TabID` to `AgentTabID`. So an agent-facing function signed `func (a *App) clickRef(id AgentTabID, ref int) error` cannot be handed the user's tab by accident, and the only route to an `AgentTabID` is the one function that knows the difference. Where the frontend genuinely acts on any tab — bounds, zoom, visibility, close — the binding keeps `TabID` and nothing changes.

An explicit `AgentTabID(someString)` conversion still compiles. That is fine and even desirable: it is one greppable token, and a reviewer seeing it knows to ask why.

### What it costs

A signature sweep over the `App.Browser*` bindings plus the agent-side skills. No behaviour changes; the compiler either agrees or names the place it does not.

---

## Defect 3 — two answers to "what did somebody type?"

> **Built 2026-08-17** as `desktop/address.go`. The TypeScript classifier is gone; the address bar searches, the agent's `open` refuses and names `web_search`. The engine is one Go constant, and surfacing it in Settings is a frontend job left for its own change.

The same question is answered twice, in two languages, with two different sets of rules:

- [`normalizeUrl`](../../desktop/frontend/src/lib/stores/workbench.svelte.ts) in TypeScript, for the address bar.
- `normalizeWorkbenchURL` in Go, for the agent's `open`.

Both end the same way: anything that is not a scheme and not a Windows path gets `https://` stamped on the front. Typing **ยูทูป** in the address bar therefore produces `https://ยูทูป`, which the engine punycodes to `xn--o3cit6gb` and DNS refuses. The tab strip then labels it `xn--o3cit6gb`, because `labelForUrl` hands back a raw hostname.

This is the third shape of the same defect. The rule "decide whether this is an address or a query" exists in the head of whoever wrote each function and in neither codebase.

### What it should be

One resolver in Go, one binding, both callers using it:

```go
type Address struct {
	URL   string // set when the input named a place
	Query string // set when it did not
}

func ResolveAddress(input string) Address
```

The classification is the boring part and every browser agrees on it: a scheme, a Windows or POSIX path, a leading `/`, a `host:port`, or a dot with no spaces means a place; anything else is a query.

**The policy is the caller's, and the two callers genuinely differ** — which is why this is one function with two users, not one behaviour:

- **The address bar** turns a `Query` into a search. A person who typed a word into an address bar wants to search; every browser they have ever used does this.
- **The agent's `open`** refuses a `Query`, and says to use `web_search`. The agent already has a search tool. An `open` that quietly searched would teach it that `open` is a search box, and it would keep reaching for the wrong tool while getting away with it.

The open question this leaves is which engine the address bar searches with, and whether that is a setting. That is a product decision, not an architectural one, and it is the only thing blocking this defect.

`labelForUrl` decoding punycode back to Unicode for display is a one-line rider on the same work.

---

## Defect 4 — the engine's diagnosis stopped at the log file

> **Built 2026-08-17.** Found by the owner, not by the code: *"เบาเซอร์ อาจจะพังเพราะมันไม่รายงานอะไรกับเอเจน เลยก็ได้ เอเจนเลยไม่รู้"*.

This is the defect that made the other three expensive, and it is not about threads or tabs at all.

`SetErrorCallback` in [browser_windows.go](../../desktop/browser_windows.go) was the entire fate of every complaint the engine ever made:

```go
chromium.SetErrorCallback(func(err error) {
	fmt.Fprintln(os.Stderr, "browser tab error:", err)
	debuglog.Msg("browser tab %s error: %v", id, err)
})
```

Two writes, no reader. `tabCallbacks` carried `onMessage` and `onNavDone` and nothing for this, so `browser.go` could not know an error had happened, so no tool result could mention one.

What the agent got instead, twenty seconds later, was `page did not finish loading` — a `statereport`, whose documented meaning is *a report about a page at a moment, slow tonight or down tonight, not behaviour to correct*. The agent read that correctly, concluded the network was unreliable, and told the user so. Three times. While WebView2 had been answering `This method can only be called from the thread that created the object` on every single attempt.

**The agent was not wrong. It was accurate about a sentence that was wrong**, and no amount of reasoning gets you from "the page did not load" to "this program is calling COM from the wrong apartment".

### What it is now

`tabCallbacks` gains `onEngineError`. `browserTab` keeps the last complaint, cleared by `armNavigation` so it always belongs to the navigation being waited on. And the timeout branch of `awaitNavigation` splits in two:

```go
if engErr := t.engineError(); engErr != nil {
	return fmt.Errorf("the browser engine refused what Aetox asked it to do: %w", engErr)
}
return statereport.New("page did not finish loading")
```

The two branches are different *kinds* of thing and are typed as such. A slow site is weather: a `statereport`, nothing to correct, and the learning loop is right to file it and move on. An engine that refused our call is a defect in this program, and filing that as weather is exactly how it survived a week of being logged every twenty seconds.

### The general rule this is an instance of

Anything an outside system tells us about **our own** request must be able to reach the caller. Not a log — a log is for the person already investigating, and the whole problem is that nobody knew there was anything to investigate.

The seam here is small: one field on `tabCallbacks`, one field on `browserTab`, one branch. The value is not in the code, it is that the next wrong-apartment call announces itself in the tool result the first time it happens, to whoever is standing there — including when whoever is standing there is the agent.

## What this does not change

- **The security model.** The origin check and the per-request token in [browser-security-2026-07-21.md](browser-security-2026-07-21.md) are untouched. `onTab` does not widen anything; it narrows.
- **The platform boundary.** `hostBackend`/`tabView` stay the seam the Linux and macOS ports plug into. Defect 1 makes that seam smaller, not different.
- **One tool, actions inside it.** The packed shape from 2026-08-10 and the per-action permission vocabulary stay exactly as they are.
- **The agent has one tab.** Still true, and Defect 2 is how it becomes true in the type system rather than in a comment.

## Order, and why this one

1. **Defect 1.** It is the one that has already cost real time, it is purely mechanical, and it is the only one whose success is verifiable by a test that does not run: `tab.view.navigate(url)` must fail to compile.
2. **Defect 2.** Signature sweep. Do it after 1 so the two touching passes over the same files do not collide.
3. **Defect 3.** Was blocked on the search-engine decision; shipped with Google as one named constant and the setting deferred, because the architecture is identical whichever engine wins and holding the plumbing hostage to a preference would have been the wrong trade.

**Defect 4 jumped the queue** and was built alongside Defect 1, because it is what turns the next one of these from a day into a sentence.

## How we will know it worked

Not by the tests passing. By this: after Defect 1, the exact line that caused §127.8 cannot be written. After Defect 2, the exact confusion that caused §127.1 cannot be written. A rule you can still break, and only find out about from a 20-second timeout and a log line in `%APPDATA%`, has not been fixed — it has been re-explained.
