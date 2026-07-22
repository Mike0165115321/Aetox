# internal/app — engine wiring + CLI interactive loop

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Known debt: §6.1 (this package mixes two jobs)

**What it is:** the glue that turns the engine's parts (`cognitive.Agent` + `skill.Dispatcher` + `turn.Executor`) into a runnable chat app — *and*, historically, the CLI's entire terminal presentation. Both front ends construct one `App` each; the desktop uses ~2 of its ~35 exported methods, the CLI uses all of them.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Options` / `NewApp` ([app.go](app.go)) | The single entry point both front ends call. Takes `Agent`, `Dispatcher`, `ApprovalMode`, `Permissions`, `OnToolAction` (live tool-call feed → desktop timeline / inspector), `StatusReporter` (turn-phase strings → typing indicator). |
| `RunOnce(ctx, message)` / `RunOnceStream(ctx, message, onChunk)` | One chat turn, string in → reply out. **This is the desktop's whole surface** (`RunOnceStream` — same turn, plus live reply chunks for the streaming bubble). Internally builds a `turn.Executor` per call via `wireStatusReporter`. |
| `RunInteractive(ctx)` | The CLI REPL: prompt loop, slash-command handling, approval picker, spinner, banner/status bar. Desktop never calls this. |

## Files

- [app.go](app.go) — everything above, ~1,200 lines. The REPL/presentation half (banner, Thai approval prompts, `startThinkingIndicator`, status bars) is the §6.1 extraction candidate.
- [console.go](console.go) — `Console` I/O abstraction (`NewStdIO`), so tests and the desktop can run turns without a real terminal.
- [interactive_input.go](interactive_input.go) — raw-mode line editing / slash suggestions for the REPL.

## Rules of thumb

- Adding engine behavior? It probably belongs in `turn`/`skill`/`cognitive`, not here — this package only wires them.
- Adding CLI UX? It lands here today, but expect it to move to `cli/` when the [module split](../../docs/architecture/module-split-2026-07-21.md) migrates.
- The prompt/context layer ([ARCHITECTURE.md §11](../../ARCHITECTURE.md)) will change what callers pass as `SystemPrompt` — this package itself shouldn't need to know.
