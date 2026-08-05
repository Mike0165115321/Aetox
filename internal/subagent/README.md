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
steps: 24               # optional; max tool-loop rounds (default 24)
desk: specialized       # optional; makes this a CHAIR in that desk's office (§84)
---

Everything after the frontmatter is the brief, handed to the sub-agent as its
system prompt when it is spawned.
```

Frontmatter is parsed by `skill.ParseFrontmatter` — one `key: value` per line, not YAML. Unknown keys are ignored, so a file with extra notes still loads; unterminated frontmatter is kept as prompt text rather than dropping the profile. `name:` is **not read** — the filename is the name.

| | Where |
|---|---|
| Bundled | [profiles/](profiles) via `//go:embed`. **Delegates:** `explore` (read-only searcher, 4 tools), `general` (the looper: a list of items is ONE job it works through itself, 48 steps), `plan` (ARCHITECTURE.md §54 — inherits every reading tool, denies every writing one, and answers in a fixed four-part shape). **Chairs** (`desk: specialized`, ARCHITECTURE.md §84): `deck`, `doc`, `sheet` — one craft each, briefed once, handing back a .pptx / .docx / .xlsx. Present on a fresh install with no folder created |
| User | `<DataRoot>/subagents/*.md`. A file named after a bundled one **wins**; deleting it restores the original — that is the "revert" |

## How one runs — and why it does not block

Three tools, registered together by `NewTaskTools` and sharing one runner ([runner.go](runner.go)):

- **`task`** ([task.go](task.go)) starts a delegate and **returns a handle immediately** — the model goes on with its turn.
- **`task_result`** ([task_result.go](task_result.go)) redeems the handle, waiting only if that delegate has not finished. It takes several ids at once.
- **`task_answer`** ([ask.go](ask.go)) replies to a delegate that got stuck and asked.

One start does: pick the profile → decide which desk the job runs at (`ceilingFor`) → `FilterRegistry` for the child's tools → a fresh `cognitive.Agent` on the profile's brief and cap → a full turn through the real `turn.Executor`, in a goroutine → the collector gets the final text plus `[task <name>: N tool calls, X.Ys]`, and nothing else. Tool events are stamped with the `task` call's id (`turn.CallID`) so the UI shows them as the delegate's work.

Because starting never waits, N delegates started before the first collect run at the same time — parallelism is a property of the pair, not a separate mechanism. Four in flight per turn is the cap. A delegate's context descends from the turn's, so Stop kills every outstanding one and nothing outlives the reply.

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
