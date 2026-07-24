# cmd/aetox — CLI front end

> Module map: [ARCHITECTURE.md §4](../../ARCHITECTURE.md) · The other front end is [desktop/](../../desktop/README.md)

**What it is:** the terminal entry point. Parses flags, resolves config/model preference, bootstraps the provider, builds one `cognitive.Agent` + `internal/app.App`, then hands off to `app.RunInteractive` (REPL) or one-shot mode. All real behavior lives in `internal/*`; this package is wiring plus terminal-only concerns.

## Key seams

| Seam | What it does |
|---|---|
| `main()` → `run()` ([main.go](main.go)) | Flag parsing (`-y`, `-approval`, `-debug`, …), config load chain (`config.Load` → `LoadModelPreference` → env keys), `model.BootstrapProvider`, agent construction. |
| `buildSystemPrompt(root)` ([main.go](main.go)) | Hardcoded system prompt, near-duplicate of `desktop/app.go`'s copy — **scheduled for deletion** when the prompt layer ([ARCHITECTURE.md §11](../../ARCHITECTURE.md)) lands; both will call `internal/prompt` instead. |
| model-switch path (`app.ModelSwitchResult`) | `/model` mid-session rebuilds the agent. Anything added to `AgentConfig` at startup must be added here too, or it silently resets on switch (this bit us: `MaxToolCalls` was set at startup only — removed 2026-07-22 with the unbounded-loop decision). |

## Behavior notes

- Tool loop is **unbounded** (OpenCode-style); the CLI's brake is Ctrl+C (ctx cancel) plus the approval layer. See [model-control-layer doc §3](../../docs/architecture/model-control-layer-2026-07-22.md).
- Windows vs other OS terminal setup is split into `main_windows.go` / `main_other.go`.
- Model/approval preferences persist to the same file the desktop writes — switching in one surface affects the other's next start.
