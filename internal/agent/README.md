# internal/agent — agent profiles

One markdown file = one profile. See [ARCHITECTURE.md §44](../../ARCHITECTURE.md) for the decision and the build order; this file is the format reference.

## Two layers, kept apart

| | An **agent** | A **sub-agent** |
|---|---|---|
| What it is | what the user talks to | what an agent hands work to |
| Bundled | [profiles/agents/](profiles/agents) — `build`, `plan` | [profiles/subagents/](profiles/subagents) — `explore`, `general` |
| User files | `<DataRoot>/agents/*.md` | `<DataRoot>/subagents/*.md` (a **sibling**, not nested) |
| Listed by | `List()` | `ListSubagents()` |
| Loaded by | `Load(name)` | `LoadSubagent(name)` |
| Step cap | none — a human is watching and can hit Stop | 24 by default (`steps:` overrides) |

**Neither lookup ever falls back to the other layer.** A sub-agent name does not resolve where the session's agent is chosen, and `LoadSubagent` cannot reach the agent the user is talking to. The same name may exist in both directories and they stay two different profiles — there is no tie-break rule to get wrong because nothing searches both.

**The directory is the only thing that records which layer a profile is in.** There is deliberately no `kind:`/`mode:` key: two places saying the same thing is a place they can disagree.

## The file

```markdown
---
description: shown on the settings row and in the palette
model: deepseek-v4      # optional; omit to use whatever model the chat is on
tools: grep, glob, read # optional; omit to inherit the whole registry
deny: write, shell      # optional; refused even if the tool reaches the agent
steps: 24               # optional; max tool-loop rounds
---

Everything after the frontmatter is the role, sent to the model as part of the
system prompt on every turn.
```

Frontmatter is parsed by `skill.ParseFrontmatter` — one `key: value` per line, not YAML. Unknown keys are ignored, so a file with extra notes still loads; unterminated frontmatter is kept as prompt text rather than dropping the profile. `name:` and `kind:` are **not read at all** — the filename and the directory decide those.

A user file shadows a bundled one of the same name **in the same layer**. Deleting it restores the bundled original; that is the "revert".

## What consumes a profile

Nothing here executes anything. Three existing knobs read it:

- `cognitive.AgentConfig` — the prompt (via `prompt.BuildWithRole`), `Model`, `MaxToolCalls()`
- `safety.PermissionConfig` — `DenyRules()`, appended **last** so a profile's denial wins over a user allow-rule (`Resolve` is last-match-wins)
- the skill registry handed to the agent — `FilterRegistry`, which is a **token budget**, not the safety gate: a tool the profile excludes is never sent to the model, while `Deny` blocks execution. Both apply.

`AllowsTool` also enforces the forced sub-agent denials (`task`, `help`, `ask_user`, `todo_write`) — that is what makes sub-agent depth 1 structural instead of a counter.

## Tests

[profile_test.go](profile_test.go) pins the bundled four, that the layers do not leak into each other, `plan` denying every mutator with `PermissionConfig` agreeing, `build` keeping everything, shadowing, the directory deciding the kind (a stray `kind:` key cannot contradict it), and path-traversal rejection. [store_test.go](store_test.go) pins the write half: save/delete round-trip, writes staying in their layer, `Save` honoring its `Kind`, `SetModel` editing exactly one frontmatter line without relocating the file, and `FilterRegistry` per profile.
