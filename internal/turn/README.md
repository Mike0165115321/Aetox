# internal/turn — the turn pipeline (Executor)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Deep dive: [model-control-layer-2026-07-22.md](../../docs/architecture/model-control-layer-2026-07-22.md) §4 · Turn flow diagram: ARCHITECTURE.md §5

**What it is:** the layer that decides *how one user message gets answered* — pure conversation, model-driven tool calls, or regex-inferred tool execution — and enforces approval on every tool path. This package is the **safety chokepoint**: nothing reaches a tool without passing `resolveApproval`.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Executor` / `NewExecutor(ExecutorOptions)` ([executor.go](executor.go)) | Built per turn by `internal/app.wireStatusReporter`. Options carry the `Agent`, `Dispatcher`, approval func, `PermissionConfig`, `StatusReporter` (phase strings, Thai), `OnToolAction` (call/result feed → desktop tool timeline). |
| `Execute(ctx, line, intent, onChunk, onToolComplete)` | The 4-phase pipeline: ① high-priority inferred tools → ② model-driven tool loop (`Agent.RespondWithTools`, unbounded — see agent README) with inferred fallback → ③ inferred tools for non-tool-capable models → ④ streaming conversation. |
| `resolveApproval(...)` ([executor.go](executor.go)) | Permission rules (last-match-wins) checked first; only unmatched tools fall through to the coarse `ApprovalMode` gate + interactive prompt. **Every tool path routes through here** — inferred, model-driven, and direct skill dispatch alike. |

## Files

- [executor.go](executor.go) — pipeline + approval + tool-receipt JSON for the model (largest file in the repo).
- [infer.go](infer.go) — regex-based tool candidate inference (the fallback brain for models without tool calling).
- [record.go](record.go) / [result.go](result.go) — execution records and turn result shaping / summaries.

## Rules of thumb

- Adding a tool? Do it in `internal/skill`; this package finds it via the `Dispatcher` interface — no edits here.
- Changing what the *model sees* after a tool runs → `modelToolReceipt`. Changing what the *user sees* → the summarize/fallback-summary paths.
- Never add a tool-execution path that skips `resolveApproval`.
