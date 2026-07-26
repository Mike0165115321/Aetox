# Foreign Coding CLIs in Aetox — Consultants, Never the Engine

*Deferred 2026-07-27. Decision recorded in [ARCHITECTURE.md](../../ARCHITECTURE.md) §46; this file is the reasoning behind it. Nothing here is built.*

## Trigger

Owner — *"เราจะให้ Claude code opencode Codex หรืออะไรพวกนี้มาทำงานผ่าน Aetox ได้ไหม แบบเผื่อมีคนซับตัวอื่น แต่อยากลอง Aetox"*.

The motive matters more than the question: someone already paying for Claude Max or ChatGPT Plus will not also buy API credit to try a new app. So the real ask is *"can their existing subscription drive Aetox's main loop"*.

Then, on being shown the shape: *"นี้เป็นงานที่แตกออกไปเลย ทำเป็นแผนไว้ก็พอ เพราะผมจะเดินไปในทางที่จะให้ Aetox ทำงานแทนคนได้แบบออโตเมชั่น"* — parked in favour of the automation direction. Written down so the question is answered once.

## What already works, with no code at all

`shell` ([internal/skill/shell.go](../../internal/skill/shell.go)) exists, so the main agent can already run `claude -p "…"`, `codex exec "…"` or `opencode run "…"` today. All three are on the owner's PATH. That is **Aetox using them**, not them driving Aetox — it does not answer the question, but it is the baseline any richer design has to beat.

## Rejected outright — a foreign CLI as a `model.Provider`

Two independent reasons, either one sufficient.

### 1. Wrong shape: they are agents, not models

`Provider.Complete` ([internal/model/types.go](../../internal/model/types.go)) returns `Response.ToolCalls` so that **Aetox** dispatches them. Claude Code, Codex and OpenCode each carry their own system prompt, their own tool loop, their own tool set and their own permission layer. Slotting one into the provider seam ([internal/model/factory.go](../../internal/model/factory.go)) means it never returns a tool call — it runs them itself.

Everything Aetox is would then be bypassed rather than exercised:

| Layer | What happens |
|---|---|
| skill registry (30+ tools) | never offered — the foreign agent uses its own |
| `safety.PermissionConfig` | never consulted — the foreign agent has its own gate |
| rtk layer (§13) | never reached |
| `task` / `task_result` / `ask_main` (§44) | dead |
| summarizing compaction (§20.3), per-model windows (§20.2) | dead |
| prompt/identity layer (§11) | overridden |

The result is Claude Code running in an Aetox window. Someone who "wants to try Aetox" would not have tried Aetox — the motive defeats itself.

### 2. Subscription auth is bound to its own client

A user typing `claude -p` on their own machine is using Claude Code, which is what they bought. A product distributed through winget/Scoop (§23) that advertises *"plug in your Max subscription"* is proxying subscription auth into a third-party client, and the account at risk is the **user's**, not the author's.

This is a ship blocker, not a technical one. It does not get solved by better engineering.

## The shape that is allowed, if it is ever built

A **sub-agent profile** — `agent: claude-cli` — used as a *consultant*: second opinion, diff review, "why does this fail". Never as an implementer.

**User file, not bundled.** `<DataRoot>/subagents/claude-cli.md`, never `internal/subagent/profiles/*.md`. Bundling ships the ToS problem above to every install; a user file is the machine owner's own choice. This reuses the bundled-vs-user split §44 already defines, as a policy boundary rather than a convenience.

**No Go code.** A profile is data ([internal/subagent/profile.go](../../internal/subagent/profile.go)) — frontmatter plus a body that becomes the child's system prompt:

```markdown
---
description: ส่งงานต่อให้ Claude Code CLI — คืนคำตอบดิบกลับมา
tools: shell
steps: 4
---
Run exactly one command: claude -p "<the brief you were given>"
Return stdout verbatim. Never add --dangerously-skip-permissions.
One run — if it fails, report the error and stop.
```

Headless defaults are read-only, which is exactly the consultant contract. Codex and OpenCode are the same file with one line changed.

**Why it beats the plain `shell` baseline:** the consultant has its own Read/Grep/Glob, so a brief names paths instead of pasting code, and only the conclusion returns to the parent transcript — the same context argument that justifies `task` in the first place (§44.6). Plus background execution and a nested timeline for free.

**Costs, stated rather than hidden:**

- One Aetox model round-trip is spent deciding to call `shell` before the free CLI ever runs.
- The subscription's rate limit is the owner's own — a chatty consultant eats the quota they use directly.
- `tools: shell` is full shell, not just the `claude` binary; the only gate is the session's permission rules.
- `claude -p` sees none of Aetox's conversation and the sub-agent is only a relay, so a thin brief produces a confidently wrong answer that re-enters the transcript looking authoritative.

## What this is not

It is **not** the answer to *"let someone try Aetox without paying"*. The main loop still needs its own provider. That belongs to first-run onboarding over the free paths the catalog already carries — `aetox` (no key), Ollama, Gemini's free tier ([internal/provider/catalog.go](../../internal/provider/catalog.go)) — and is a separate decision that has not been taken.

## Status

`Deferred 2026-07-27.` No file exists. The profile written during the discussion was deleted so an unused delegate could not sit in `task`'s enum and quietly spend the owner's quota. Revisit only after the automation direction is settled, and only as a profile.
