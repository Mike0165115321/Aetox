# internal/turn — the turn pipeline (Executor)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Deep dive: [model-control-layer-2026-07-22.md](../../docs/architecture/model-control-layer-2026-07-22.md) §4 (pre-§17 snapshot) · Design: ARCHITECTURE.md §17

**What it is:** the layer that decides *how one user message gets answered* — explicit skill command, model-driven tool calls, or streaming conversation — and enforces approval on every tool path. This package is the **safety chokepoint**: nothing reaches a tool without passing `resolveApproval`.

Since §17 (2026-07-23) there is deliberately **no keyword/regex intent guessing** between the user and the model. Natural language always reaches the model verbatim; the model alone decides tool calls, and its answer is final.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Executor` / `NewExecutor(ExecutorOptions)` ([executor.go](executor.go)) | Built per turn by `internal/app.wireStatusReporter`. Options carry the `Agent`, `Dispatcher`, approval func, `PermissionConfig`, `StatusReporter` (phase strings, Thai), `OnToolAction` (call/result feed → desktop tool timeline). |
| `Execute(ctx, line, intent, onChunk, onToolComplete)` | Three paths, first match wins: ① explicit skill command (grammar-recognized token, e.g. `git status`, `/time`) → direct dispatch via `executeSkillTurn` · ② model-driven tool loop (`Agent.RespondWithTools`, unbounded — see agent README) when the provider supports tool calling · ③ streaming conversation (`Agent.RespondStream`). |
| `resolveApproval(...)` ([executor.go](executor.go)) | Permission rules (last-match-wins) checked first; only unmatched tools fall through to the coarse `ApprovalMode` gate + interactive prompt. **Every tool path routes through here** — model-driven and direct skill dispatch alike. |

## Files

- [executor.go](executor.go) — pipeline + approval + tool-receipt JSON for the model (`modelToolReceipt`, the RTK seam — §13).
- [result.go](result.go) — turn result shaping / summaries / secret redaction.

## Rules of thumb

- Adding a tool? Do it in `internal/skill`; this package finds it via the `Dispatcher` interface — no edits here.
- Changing what the *model sees* after a tool runs → `modelToolReceipt`. Changing what the *user sees* → the summarize/fallback-summary paths.
- Never add a tool-execution path that skips `resolveApproval`.
- Never add heuristics that pick tools from natural language on the model's behalf — that layer was deleted by design (§17); `TestExecute_ConversationTextNeverTriggersToolsDirectly` pins it.
