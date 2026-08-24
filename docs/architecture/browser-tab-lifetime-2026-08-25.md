# Browser Tab Lifetime — One Hook Answering Two Questions

> **Date:** 2026-08-25
> **Status:** Implemented 2026-08-25.
> **Scope:** who is allowed to end a browser tab, and how the agent finds out. [desktop/browser.go](../../desktop/browser.go), the browser half of [desktop/workbench.go](../../desktop/workbench.go), [desktop/browser_tabs.go](../../desktop/browser_tabs.go), [BrowserPane.svelte](../../desktop/frontend/src/lib/workbench/BrowserPane.svelte) and `removeTab` in [workbench.svelte.ts](../../desktop/frontend/src/lib/stores/workbench.svelte.ts).
> **Continues:** [browser-subsystem-2026-08-17.md](browser-subsystem-2026-08-17.md), whose four defects are built and whose thesis this is a fifth instance of.

## Why this exists

The 17 August document opened on three bugs in two days and said they were one mistake made three times. This is that mistake a fourth time, in the one dimension that document did not type.

| Bug | The rule that was broken | Where that rule was written |
|---|---|---|
| The agent's `read`/`click`/`type` landed on the user's tabs (§127.1) | `lastID` is "the tab in front", never "the agent's tab" | nowhere |
| Every page after the first timed out at 20s (§127.8) | a webview method runs on the thread that owns it | a comment |
| The agent opened tab after tab (2026-08-10) | the agent browses in one tab | a comment |
| **The agent is told the user closed a page the user never touched** | **`BrowserClose` is the user's door; nothing else may call it** | **a comment on `BrowserClose`** |

Same category, same outcome. The rule is correct, it is written down, and it is enforced by nothing but prose that only helps somebody who already went looking.

## What actually happened

Three failures, one session, 24 August, forty seconds apart, while the agent was reading PostgreSQL documentation:

```
22:39:35  scroll   ok
22:39:42  scroll   FAIL  "the page you were working on was closed (the user closed the tab)"
22:39:50  open     ok          <- the agent reopens, as the message told it to
22:39:56  scroll   FAIL  same message, six seconds after a successful open
22:40:06  open     ok
22:40:13  scroll   FAIL  same message
22:40:22  open     ok
22:40:32  scroll   ok          <- it works itself loose
```

No human closed a tab three times inside the six-second gap between an `open` and a `scroll`. In 902 `browser` calls this message has fired three times in the life of the app, and all three are these. **Its true-positive rate is zero.**

### The loop

```
Go: closeTab(id)                          for any reason at all
  └─ emit workbench:close-browser         browser.go — added §171, 24 Aug
       └─ browserTabClosedByEngine → removeTab
            └─ BrowserPane unmounts
                 └─ onDestroy → BrowserClose(id)     BrowserPane.svelte — 21 Jul
                      └─ agentTabClosed = true       browser.go — added 22 Aug
```

### It degraded in two steps, and each step was right

- **21 Jul.** `BrowserPane.onDestroy` calls `BrowserClose`. At that date `BrowserClose` means one thing — end this tab — and calling it from a component teardown is correct.
- **22 Aug.** `BrowserClose` gains a second meaning: *and it was the user who did it*. Nobody re-read the caller from July, which silently inherited a claim it cannot make.
- **24 Aug.** §171 gives `closeTab` an event so a tab closed on the Go side stops haunting the strip. Correct, and it completes a circle: every engine-side close now returns through the frontend and back into the user's door.

This is what "it used to be good" means and it is worth stating precisely: **`BrowserClose` used to answer one question.** Nothing has been broken since; something has been *added* to it, twice, by people who had no reason to look at a caller written five weeks earlier.

## The defect

Two halves, and they compound.

### Half one — `removeTab` is one funnel with two meanings upstream

```
user clicks ×  ──┐
                 ├─→ removeTab(id) ─→ pane unmounts ─→ onDestroy ─→ BrowserClose ─→ "the user did it"
engine closed ───┘
```

`onDestroy` is a **lifecycle** event. It fires when the component goes away, and a component goes away for reasons that have nothing to do with intent. Using it as the single close path means the frontend cannot distinguish the user pressing × from the engine telling it a tab is already gone — and it reports both as the first.

This is §127.1 exactly: one thing answering two questions. There it was a field; here it is a hook.

### Half two — the reason is a global bool with no owner and no identity

```go
agentTabClosed bool
```

Anything may set it. The first reader takes it and clears it. It records no tab and no moment, so it can only answer *"did something get closed recently"* — and the question the agent is actually asking is *"what happened to the page I am holding"*. That is why the message survives a reopen and lands on the first action of a fresh, perfectly live tab.

The flag being consumable is also load-bearing debt of its own: `agentTabPeek` exists only because `agentTab` *takes* the flag, and the busy indicator would otherwise eat a sentence meant for the model. A whole function and its warning comment are there to work around a mutable global.

## What it should be

Three changes. The first deletes the loop, the second makes the remaining callers unable to lie, the third makes the answer specific to the tab it is about.

### 1. Intent is stated where it happens, and a lifecycle hook is not intent

The × handler closes the tab. `onDestroy` stops closing anything.

```ts
// the user's ×
export async function closeTab(tab: WorkbenchTab): Promise<void> {
  if (tab.kind === 'terminal') await TerminalClose(tab.id)
  if (tab.kind === 'browser') await BrowserClose(tab.id)   // says who
  removeTab(tab.id)
}
```

An unmount that is not a close has nothing to clean up: the engine already dropped a tab it closed itself, and a frontend reload — the one case where a native window is genuinely orphaned — does not run `onDestroy` at all and is already swept by `CloseAllBrowserTabs`. The hook was covering a case that the sweep covers better.

### 2. The reason travels with the close

```go
// closeReason says who ended a tab. There is deliberately no zero value:
// a close that does not say who must not compile.
type closeReason int

const (
	closedByUser  closeReason = iota + 1 // the × on the strip
	closedByAgent                        // the agent's own `browser tabs close`
	closedByApp                          // sweeps, teardown, a view that died
)

func (a *App) closeTab(id string, why closeReason)
```

`BrowserClose` becomes the Wails binding that means `closedByUser` and nothing else. `closeAgentTab` passes `closedByAgent` — today it takes the long way round to say that, through a comment explaining why it must not use the other door. `CloseAllBrowserTabs` passes `closedByApp`.

### 3. The answer is keyed to the tab, not to the clock

> **Built as two fields rather than the map first drafted here.** A map wants an id to look up, and at the moment this is read there is no live `agentID` left to look one up by — the tab is gone, which is the whole reason anybody is asking. So the record names the tab it is about:

```go
goneID  string
goneWhy closeReason
```

Written only when a tab was actually removed, so the second pass over an already-closed id writes nothing and **re-entrancy is a no-op by construction rather than by a guard somebody has to remember**. Cleared the moment the agent has a page again, in `open` — which is where the staleness actually ends, and is what stops a sentence about a closed tab being delivered against a live one six seconds later.

**"Said once" is kept.** The record is still taken by the call that reports it, so a reason is told to one call and not repeated for the rest of the turn. That was decided on 22 August with a test behind it, and nothing found here argues against it — the bug was never that the message repeated, it was that the message was false and outlived its subject. `agentTabPeek` therefore stays: while reading is destructive, the busy indicator must go on asking without consuming. Merging the two is a separate change with a separate argument, and it is not this one.

Reading is now consumed only on the branch that actually says something, rather than at the top of the function, so an unrelated successful call can no longer eat a sentence meant for the call that has no page.

## What it costs

One line deleted in `BrowserPane.svelte`, one line added in the store, a `closeReason` argument threaded through four call sites, and `agentTabClosed bool` becoming a small map. No behaviour changes for the user. The compiler names every place that has not been updated, which is the point.

## What this does not change

- **The three layers.** `App.Browser*` / `browserHost` / `hostBackend` stay exactly as they are. This is a defect inside the shape, like the previous four.
- **`AgentTabID`.** Untouched, and this is its sibling: that typed *which* tab, this types *what happened to it*. (Noted in passing: Defect 2 shipped half its types — `AgentTabID` exists, the `TabID` beside it never landed, so an ordinary tab id is still a bare `string`. Nothing here depends on that being finished, and the signatures above are written for the code as it is.)
- **§171.2's two-step teardown.** Dropping `tabs` now and `views` after the destroy is correct and stays.
- **The message itself.** The agent still learns that a user action took its page, and still gets told to open again and carry on. It just stops being told that when it is not true.

## What it opens

§171.4 left one case with no home: a webview that dies some other way leaves a chip the window believes in. Under `closeReason` that case has a name — `closedByApp` — and the pane noticing its own view is gone becomes a third caller of an existing door rather than a new mechanism.

## How we will know it worked

Not by the tests passing.

- `a.closeTab(id)` **fails to compile**. Ending a tab without saying who is the mistake, and it is now impossible to write.
- `onDestroy` no longer appears in any stack that reaches a close.
- The loop is pinned in `desktop/browser_close_reason_test.go` and the window's half in `browserTabClosed.test.ts`. **All of them were watched failing first**, against the old code, and one of them was a false instrument on the first attempt: the pane test passed either way until it was made to open a real page, because a pane that never opened would not have closed anything under the old code either. §180.5, one day later.

A rule you can still break, and only find out about from three false accusations in forty seconds, has not been fixed. It has been re-explained.
