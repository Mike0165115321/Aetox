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
| Bundled | [profiles/](profiles) via `//go:embed` — `explore`, `general`. Present on a fresh install with no folder created |
| User | `<DataRoot>/subagents/*.md`. A file named after a bundled one **wins**; deleting it restores the original — that is the "revert" |

## What consumes a profile

Nothing here executes anything. Three existing knobs read it:

- `cognitive.AgentConfig` — the brief as `SystemPrompt`, `Model`, `MaxToolCalls()`
- `safety.PermissionConfig` — `DenyRules()`, appended after the session's own so a profile's denial wins (`Resolve` is last-match-wins)
- the skill registry handed to the child — `FilterRegistry`, which is a **token budget**, not the safety gate: a tool the profile excludes is never sent to the model, while `Deny` blocks execution. Both apply.

`AllowsTool` also enforces the forced denials — `task`, `help`, `ask_user`, `todo_write` — and `FilterRegistry` always returns a copy, never the parent registry. Between them that is what makes sub-agent depth 1 structural instead of a counter: `task` is simply not in the child's registry.

## Tests

[profile_test.go](profile_test.go): the bundled two (description, a real brief, a step cap), `explore` read-only and unable to recurse, `general` inheriting tools but not `task`, deny rules reaching `PermissionConfig`, shadowing, path-traversal rejection, broken files still loading. [store_test.go](store_test.go): save/delete round-trip and revert, name validation, `SetModel` editing exactly one frontmatter line, `FilterRegistry` per profile.
