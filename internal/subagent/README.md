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
---

Everything after the frontmatter is the brief, handed to the sub-agent as its
system prompt when it is spawned.
```

Frontmatter is parsed by `skill.ParseFrontmatter` — one `key: value` per line, not YAML. Unknown keys are ignored, so a file with extra notes still loads; unterminated frontmatter is kept as prompt text rather than dropping the profile. `name:` is **not read** — the filename is the name.

| | Where |
|---|---|
| Bundled | [profiles/](profiles) via `//go:embed` — `explore` (read-only searcher, 4 tools) and `general` (the looper: a list of items is ONE job it works through itself, 48 steps). Present on a fresh install with no folder created |
| User | `<DataRoot>/subagents/*.md`. A file named after a bundled one **wins**; deleting it restores the original — that is the "revert" |

## How one runs — and why it does not block

Two tools, registered together by `NewTaskTools` and sharing one runner ([runner.go](runner.go)):

- **`task`** ([task.go](task.go)) starts a delegate and **returns a handle immediately** — the model goes on with its turn.
- **`task_result`** ([task_result.go](task_result.go)) redeems the handle, waiting only if that delegate has not finished. It takes several ids at once.

One start does: pick the profile → `FilterRegistry` for the child's tools → a fresh `cognitive.Agent` on the profile's brief and cap → a full turn through the real `turn.Executor`, in a goroutine → the collector gets the final text plus `[task <name>: N tool calls, X.Ys]`, and nothing else. Tool events are stamped with the `task` call's id (`turn.CallID`) so the UI shows them as the delegate's work.

Because starting never waits, N delegates started before the first collect run at the same time — parallelism is a property of the pair, not a separate mechanism. Four in flight per turn is the cap. A delegate's context descends from the turn's, so Stop kills every outstanding one and nothing outlives the reply.

**Repeated work is one delegate looping**, never one per item: a delegate already runs its own tool loop, so twelve files is one brief with twelve items. `task`'s description says so, because one-delegate-per-item pays for twelve fresh contexts.

A loop that ends without the delegate choosing to — its step cap, or the doom-loop guard — comes back as a **failed** result naming the next action (split the batch / sharpen the brief), recognised via `cognitive.ToolLoopExhausted` and `cognitive.DoomLoopStopPrefix` rather than by matching their prose.

## What consumes a profile

Nothing here executes anything. Three existing knobs read it:

- `cognitive.AgentConfig` — the brief as `SystemPrompt`, `Model`, `MaxToolCalls()`
- `safety.PermissionConfig` — `DenyRules()`, appended after the session's own so a profile's denial wins (`Resolve` is last-match-wins)
- the skill registry handed to the child — `FilterRegistry`, which is a **token budget**, not the safety gate: a tool the profile excludes is never sent to the model, while `Deny` blocks execution. Both apply.

`AllowsTool` also enforces the forced denials — `task`, `help`, `ask_user`, `todo_write` — and `FilterRegistry` always returns a copy, never the parent registry. Between them that is what makes sub-agent depth 1 structural instead of a counter: `task` is simply not in the child's registry.

## Tests

[profile_test.go](profile_test.go): the bundled two (description, a real brief, a step cap), `explore` read-only and unable to recurse, `general` inheriting tools but not `task`, deny rules reaching `PermissionConfig`, shadowing, path-traversal rejection, broken files still loading. [store_test.go](store_test.go): save/delete round-trip and revert, name validation, `SetModel` editing exactly one frontmatter line, `FilterRegistry` per profile.
