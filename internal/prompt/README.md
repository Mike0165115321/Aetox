# internal/prompt — system prompt assembly

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Design decision: [ARCHITECTURE.md §11](../../ARCHITECTURE.md) (settled 2026-07-22)

**What it is:** the one place that builds the system prompt both front ends hand to `cognitive.NewAgent`. Replaces the two near-duplicate `buildSystemPrompt` copies that used to live in `cmd/aetox/main.go` and `desktop/app.go`.

## Key seams

| Seam | What it does |
|---|---|
| `Build(surface, scope)` / `BuildForDesk(...)` / `BuildWithReport(...)` ([prompt.go](prompt.go)) | Concatenates layers, most-specific last so it wins on conflict: identity (per `Surface`) → environment (paths are relative; repeat what a tool reported, never assemble a path) → **desk direction** → user-global (every `*.md` file in `config.IdentityDir()`, one layer block each) → learned memory (shared, then this desk's) → project (`ProjectContextFile`). Missing/empty files are skipped silently; each file layer is capped at `maxLayerBytes` (16KB). |
| `Desk{Name, Direction, Carries, Delegates}` ([prompt.go](prompt.go)) | The mode a session was opened at (ARCHITECTURE.md §83) — this package still must not import `internal/mode`, so the manifest arrives as its own `AllowsTool` closure plus one bool, never as a copied tool list that could disagree with it. A desk contributes **direction and capability**, never identity. The zero value is the pre-desks full desk: carries everything, may dispatch to anyone. Its position (with the engine text, before the user's files) is the precedence policy: what the user wrote outranks what the desk says. |
| Desk-gated layers (`fileEditing`, `longform`) | The two engine layers that name something not every desk has. `fileEditing` asks for `diagnostics` only where the desk carries it; `longform` teaches the handover to a deliverable agent only where the desk declares `dispatch:`. See §93 — a layer that names a tool has to be able to ask whether the tool is here. |
| `ProjectContextFile(root)` | Checks `AETOX.md`, then `AGENTS.md`, then `CLAUDE.md` under root. Exposed so `desktop/app.go`'s `projectStatus` badge reports the same file this package would actually load — not a separate `os.Stat` that can drift from reality. |
| `foldIdentityLayers(b)` | Reads `config.IdentityDir()`, folds every `*.md` file in it into `b` (sorted by filename), returns the paths that actually contributed content. |
| `Loaded` (`BuildWithReport`'s second return) | `UserGlobalPaths []string` (every identity file folded in) + `MemoryPath` + `DeskMemoryPath` + `ProjectPath` — for the same badge-honesty purpose. |

## Reload timing (settled, don't relitigate without checking ARCHITECTURE.md §11 first)

**Bootstrap-only.** `Build`/`BuildWithReport` are called where the agent is constructed: app start, project switch, model/provider switch — never per turn. Editing `AETOX.md` mid-session has no effect until one of those happens. This was a deliberate choice (owner: "หลายๆที่ทำก็แบบนั้น" — matches convention elsewhere), not an oversight — a per-turn mtime-check upgrade path is documented in §11 if it's ever needed, but isn't built.

## Before adding anything to a prompt (§93)

Every character here is paid on **every request, forever**, by every provider without prompt caching — which is most local runtimes, the case this project is built for. So the question is never "is this true and useful?" (almost everything is). It is **"does the model already do this?"**

Sort the sentence you are about to add into one of two buckets, out loud, in the comment above the function:

| Bucket | What it looks like | Where it goes |
|---|---|---|
| **The model already does this** | General competence: an edit is cheaper than a rewrite · a tool call costs a round · mental arithmetic on small numbers is fine · long answers should be organised · be concise | **Not in the prompt.** Delete it. If it feels necessary, what you actually noticed was a *failure*, and the failure has a specific cause — name that instead. |
| **Only Aetox knows this** | What a tool of ours does that its name does not say (`apply_patch` is atomic) · what a tool cannot reach (`calc` has no filesystem) · what this surface silently strips (`<foreignObject>`) · where a threshold sits for us · what is on this desk | **In the prompt**, as the shortest sentence that carries it. |

Three tests the sentence has to pass before it is written:

1. **Instruction, not argument.** Say what to do, not why it is a good idea. A model does not need to be convinced that rewriting an 800-line file is expensive; it needs to be told which tool is atomic. Rationale goes in the comment above the function — **comments are not sent.** That is the whole accounting, and it is why this package has more explanation above its strings than inside them.
2. **Principle, not case.** Never `when the user says "slides", ask about the format` — an if-else in prose answers one remembered failure and nothing else. State what the failure generalises to (`ask_user`'s test in [prompt_test.go](prompt_test.go) enforces this by rejecting hardcoded topics).
3. **Ask the desk first if it names a tool.** Take `Desk`, call `carries()`. An instruction to use something the desk does not hold costs the user a round and reads to them as the assistant being broken — see §93 for the two that shipped that way.

And when a layer already exists, the same test applies to *keeping* it: on 2026-08-09 the engine layers lost ~450 characters of argument with no instruction removed, which is what a whole audit of this file yielded. Nothing Aetox-specific was cut, because there was nothing Aetox-specific in it.

## Rules of thumb

- New layer (e.g. a sub-agent profile prompt) = new function here, not a third copy in a front end.
- **A layer that names a tool takes `Desk` and asks `carries()` first.** An instruction to use something the desk does not hold costs the user a wasted round and reads to them as the assistant being broken.
- **Say the rule, not the reasoning behind it.** The model does not need to be convinced that an edit is cheaper than a rewrite; it needs to be told which tool is atomic. Rationale belongs in the comment above the function — comments are not sent, prompt text is paid for on every request.
- A desk may add direction; nothing may add a second answer to *who the assistant is* (§44.0). The identity directory is the only one.
- Keep layers append-only and ordered least-to-most-specific — that ordering is the actual conflict-resolution mechanism, not a stylistic choice.
