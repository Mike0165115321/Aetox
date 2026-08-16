# internal/subagent — sub-agent profiles

One markdown file = one sub-agent. See [ARCHITECTURE.md §44](../../ARCHITECTURE.md) for the decision and the build order; this file is the format reference.

**There is no profile for the main agent.** It is the assistant — one identity, configured by the identity files (`internal/prompt`, §11) and never chosen from a list. Profiles here answer "who does the assistant delegate to", and §44.0 records why the other reading was built and then cut.

## The file

```markdown
---
description: shown on the settings row
model: deepseek-v4      # optional; omit to use whatever model the chat is on
tools: grep, glob, read # optional; omit to inherit the whole registry
deny: write, shell      # optional; refused even if the tool reaches it
steps: 40               # optional; max tool-loop rounds (default: no ceiling)
desk: specialized       # optional; makes this a CHAIR in that desk's office (§84)
---

Everything after the frontmatter is the brief, handed to the sub-agent as its
system prompt when it is spawned.
```

Frontmatter is parsed by `skill.ParseFrontmatter` — one `key: value` per line, not YAML. Unknown keys are ignored, so a file with extra notes still loads; unterminated frontmatter is kept as prompt text rather than dropping the profile. `name:` is **not read** — the filename is the name.

| | Where |
|---|---|
| Bundled | [profiles/](profiles) via `//go:embed`, split by kind since 2026-08-05 — **the file's home is its kind** (owner's call: เอเจน/ซับเอเจน). [profiles/agents/](profiles/agents): `deck`, `doc`, `sheet`, `github`, `automation`, `research` — the chairs (ARCHITECTURE.md §84), one craft each, briefed once, each **a folder holding `AGENT.md`** so a shipped agent is laid out exactly like one the user writes. The first three hand back a .pptx / .docx / .xlsx; the rest are crafts rather than file types, which is the point of §91 — a remit is a profession, not an extension. Four of the six also carry their own `skills/` (`github` 4, `research` 3, `automation` 2, `doc` 1), which is what makes the folder the whole identity rather than just a prompt. [profiles/subagents/](profiles/subagents): `explore` (read-only searcher, 4 tools), `general` (the looper: a list of items is ONE job it works through itself, 48 steps), `plan` (§54 — inherits every reading tool, denies every writing one, answers in a fixed four-part shape) — flat `.md`, because there is nothing per-helper to package. Present on a fresh install with no folder created |
| User | `<DataRoot>/agents/<name>/` only — the sub-agents' home is **closed** (owner's call, 2026-08-06: the helpers are part of the system, never added to or edited; a user file in `<DataRoot>/subagents` is reported as a `Conflict` and never read). In the agents' home, a folder named after a bundled agent **wins**; deleting its `AGENT.md` restores the original — that is the "revert". A name the helpers own is refused at the save door (memory, jobs and chat history key on the bare name). An agents-home file that names no desk sits in the office. `Migrate()` still moves pre-split chair files home once at startup |

### One agent is one folder

Owner's call, 2026-08-06. Everything belonging to a worker lives under `<DataRoot>/agents/<name>/`, so "what is this agent" is a directory listing rather than a search across three trees:

```
<DataRoot>/agents/doc/AGENT.md       frontmatter + brief    (this package)
<DataRoot>/agents/doc/MEMORY.md      what it learned        (internal/learned)
<DataRoot>/agents/doc/STARTERS.md    how it opens a chat    (starters.go)
<DataRoot>/agents/doc/STARTERS.en.md the same, translated
<DataRoot>/agents/doc/skills/        what it knows          (skills.go)
<DataRoot>/agents/doc/mcp.json       the servers it brings  (planned)
```

The paths are owned by [`internal/config`](../config/agenthome.go), not here: `internal/subagent` imports `internal/learned`, so learned cannot ask this package where an agent's folder is, and a second copy of the path in the package that could not import the first is the kind of second answer this codebase treats as debt.

`AGENT.md` is what makes a folder a **definition the user owns**; the folder alone only means **this worker has state**. A folder holding just `MEMORY.md` is the normal shape of a shipped agent (or a helper) that has learned something, so the resolver keys on `AGENT.md` and skips the rest without reporting them.

Two consequences worth keeping straight, both pinned by tests in [homes_test.go](homes_test.go):

- **Revert ≠ delete.** Reverting a shipped agent removes `AGENT.md` and leaves `MEMORY.md` — memory belongs to the *name*, across every rewrite of the brief. Deleting an agent the user hired takes the whole folder, because the name is gone and a stranger's notes must not seed the next agent to claim it.
- **The pre-folder layout migrates itself** (`config.MigrateAgentHomes`), called by this package's resolver *and* by learned's path lookup, because there is no single entry point — the CLI, the desktop app and the tests all reach these files.

### `STARTERS.md` — how a worker opens a conversation

Owner's call, 2026-08-10. An empty chat shows a question and a few cards that fill the composer when clicked. Those used to be a list inside the desktop app, one set per room — which meant the one kind of worker a user can actually add was the one kind that could never have its own: hiring a bookkeeper gave you four generic cards forever, because giving it four real ones meant editing the product.

So it is a file in the package, read by [starters.go](starters.go). Markdown that happens to parse, rather than a format with a spec — the heading is the question, each list item is a card, fields split on `|`:

```markdown
# จะให้ทำเอกสารอะไรดี?

- ร่างรายงานหรือจดหมาย | ช่วยร่างเอกสารเรื่องนี้ให้หน่อย: | pencil
- ตรวจว่าเอกสารนี้ขาดอะไรไป | ช่วยตรวจเอกสารนี้ว่าขาดอะไรบ้าง: | search
```

Title, then the sentence that lands in the composer, then optionally the mark it wears — an icon *name* from the app's own set, same as the profile's `icon:`, because the file is a `.md` somebody edits by hand. A prompt ending in `:` is the deliberate half-sentence; the space after the colon cannot survive being written in a file, so the parser puts it back.

Four rules worth keeping straight, all pinned by [starters_test.go](starters_test.go):

- **The home decides before the language.** A user who took a shipped worker over gets *their* file in every language — falling back to the bundled English of a definition they replaced would answer as an agent that no longer exists. Within the winning home, `STARTERS.<lang>.md` wins and `STARTERS.md` fills in.
- **A bad line costs that line.** Prose with no `|` is a note the author left themselves, not a card missing a half. Nothing here is ever an error: no file, an unreadable file, a file of prose — all of them mean "no opening of my own", and the window falls back to the cards it draws for any colleague.
- **The cap is a POOL, not a hand.** `maxStarters` is 24 and the window draws **four** of whatever it is handed, dealt from a shuffled bag with a "show me another four" button under the grid (`starters.ts`). The two were the same number until 2026-08-15, which meant a worker could only gain a fifth card by giving up one of its four; splitting them is what lets an agent get deeper without the empty chat getting busier. A pool below four is what must never ship — the grid is two columns, and three cards deal a widow onto the second row.
- **The settings form stops where this reader stops.** `STARTER_MAX` in `Settings.svelte` mirrors `maxStarters`, because a form that accepted a 25th card would let a user save one that `parseStarters` silently drops on read. Two files, two languages, no import between them — so `TestSettingsFormStopsWhereTheReaderStops` fails the build the day they disagree.
- **This package decides nothing about the window.** It reads a file and hands back what was in it. That line is what lets a worker the app has never heard of ship its own opening.

## How one runs — and why it does not block

**One tool, four actions**, built by `NewTaskTools` and sharing one runner ([runner.go](runner.go)). Packed since 2026-08-16 ([packed_task.go](packed_task.go), §99): outside the tool block it is `task`, inside every gate still judges the per-action name through `skill.Unpack`.

- **`start`** ([task.go](task.go)) — the default action — starts a delegate and **returns a handle immediately**, so the model goes on with its turn.
- **`collect`** ([task_result.go](task_result.go)) redeems the handle, waiting only if that delegate has not finished. It takes several ids at once.
- **`answer`** ([ask.go](ask.go)) replies to a delegate that got stuck and asked.
- **`plan`** ([task_plan.go](task_plan.go)) declares a **run**: a name, a brief for the user, and the phases the work goes through in order ([run.go](run.go)). It starts nothing. What it buys is the phase that has *not* happened yet — declared before the findings exist, a checking round left undone sits at zero on the user's card instead of being invisible. There is no token ceiling on a run (owner, 16 ส.ค.); the card shows the spend and Stop ends it.

Packing was not tidiness. The block was at 10,004 of its 10,100-token budget with 2,277 of them spent on these four, and `plan` did not fit — the family now costs 1,568 with an action more in it. The prose the model reads lives in `packed_task.go` alone; the four implementations describe nothing.

One start does: pick the profile → decide which desk the job runs at (`ceilingFor`) → `FilterRegistry` for the child's tools → a fresh `cognitive.Agent` on the profile's brief and cap → a full turn through the real `turn.Executor`, in a goroutine → the collector gets the final text plus `[task <name>: N tool calls, X.Ys]`, and nothing else. Tool events are stamped with the `task` call's id (`turn.CallID`) so the UI shows them as the delegate's work.

Because starting never waits, N delegates started before the first collect run at the same time — parallelism is a property of the pair, not a separate mechanism. Four in flight is the cap.

**A delegate's life is the session's, not the turn's** (§105). The turn that started it ending is not an event it hears about: an uncollected delegate keeps working and can be collected in a later turn by the same id, and a question it parked on can be answered then too. The register lives in `Delegations`, which the **host** owns (`NewDelegations`, handed in through `TaskOptions`) for one reason — the only thing that ends a delegate early is the user pressing Stop, and that is a fact no code in this package can observe. `StopAll` is that door. The same argument [shell_background.go](../skill/shell_background.go) already made about commands: work that dies with the answer that started it was never background work.

**Repeated work is one delegate looping**, never one per item: a delegate already runs its own tool loop, so twelve files is one brief with twelve items. `task`'s description says so, because one-delegate-per-item pays for twelve fresh contexts.

A loop that ends without the delegate choosing to — its step cap, or the doom-loop guard — comes back as a **failed** result naming the next action (split the batch / sharpen the brief), recognised via `cognitive.ToolLoopExhausted` and `cognitive.DoomLoopStopPrefix` rather than by matching their prose.

## When a delegate gets stuck: `ask_main`

A delegate blocked on a decision only the main agent can make calls **`ask_main`** ([ask.go](ask.go)), which **parks its goroutine inside the tool call**. The next `task_result` finds a question instead of an answer; `task_answer` supplies the reply, which becomes the return value of the parked call, and the delegate carries on in the same loop with everything it had already done. Parking rather than returning is the point — a delegate that ended its run to ask would be re-briefed from scratch and read the same ten files twice.

`ask_main` is injected into every child's registry regardless of its `tools:` allowlist (it touches nothing); `task_answer` is force-denied to children like the rest of the pair.

**The deadlock this design has to avoid:** the delegate waits on the parent, so a parent that waited on the delegate would leave both parked until Stop. `runner.collect` therefore checks for an outstanding question *before* selecting on `done`, and returns the same question every time until it is answered — collecting a stuck delegate never blocks.

## What consumes a profile

Nothing here executes anything. Three existing knobs read it:

- `cognitive.AgentConfig` — the brief as `SystemPrompt`, `Model`, `MaxToolCalls()`
- `safety.PermissionConfig` — `DenyRules()`, appended after the session's own so a profile's denial wins (`Resolve` is last-match-wins)
- the skill registry handed to the child — `FilterRegistry`, which is a **token budget**, not the safety gate: a tool the profile excludes is never sent to the model, while `Deny` blocks execution. Both apply.

## Chairs, and the ceiling over every delegate

A profile with `desk:` is a **chair** in that desk's office rather than a delegate of whoever called it (COMPANY.md §4). Two rules follow, and `ceilingFor` in [task.go](task.go) is where both live:

- **Every delegate runs under a desk's manifest**, intersected with its own `tools:`/`deny:`. A delegate that could reach what its parent cannot would make the desk a façade (§83).
- **A chair runs under *its own* desk's manifest, not the caller's** — the one carve-out (§84), and only if the calling desk names that desk in `dispatch:`. Work may cross desks only as a single brief returning a file; the file crosses the counter, the tools never do. A chair that writes `tools: shell` into itself therefore does not get shell, because the office has none.

The roster (`Chairs`) is read off the folder every time, so hiring is dropping one more file — there is no registration step to forget. A desk that names nobody in `dispatch:` hands work to no one, and chairs it cannot reach are not even listed in its `task` schema.

`AllowsTool` also enforces the forced denials — `task`, `help`, `ask_user`, `todo_write` — and `FilterRegistry` always returns a copy, never the parent registry. Between them that is what makes sub-agent depth 1 structural instead of a counter: `task` is simply not in the child's registry.

## Tests

[profile_test.go](profile_test.go): the bundled two (description, a real brief, a step cap), `explore` read-only and unable to recurse, `general` inheriting tools but not `task`, deny rules reaching `PermissionConfig`, shadowing, path-traversal rejection, broken files still loading. [store_test.go](store_test.go): save/delete round-trip and revert, name validation, `SetModel` editing exactly one frontmatter line, `FilterRegistry` per profile. Desk behaviour is tested where it is visible — [desktop/desk_test.go](../../desktop/desk_test.go) runs it through a real engine: a chair capped by the office ceiling, a coding desk refused a dispatch, and an assistant-desk session handing a job to a chair and getting a file back.
