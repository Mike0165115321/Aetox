# internal/prompt — system prompt assembly

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Design decision: [ARCHITECTURE.md §11](../../ARCHITECTURE.md) (settled 2026-07-22)

**What it is:** the one place that builds the system prompt both front ends hand to `cognitive.NewAgent`. Replaces the two near-duplicate `buildSystemPrompt` copies that used to live in `cmd/aetox/main.go` and `desktop/app.go`.

## Key seams

| Seam | What it does |
|---|---|
| `Build(surface, sandboxRoot)` / `BuildWithReport(...)` ([prompt.go](prompt.go)) | Concatenates 4 layers, most-specific last so it wins on conflict: identity (per `Surface`) → environment (sandbox root + don't-leak-path rule) → user-global (`<UserConfigDir>/aetox/AETOX.md`) → project (`ProjectContextFile`). Missing files are skipped silently; each file layer is capped at `maxLayerBytes` (16KB). |
| `ProjectContextFile(root)` | Checks `AETOX.md` then falls back to `AGENTS.md` under root. Exposed so `desktop/app.go`'s `projectStatus` badge reports the same file this package would actually load — not a separate `os.Stat` that can drift from reality. |
| `Loaded` (`BuildWithReport`'s second return) | Which optional layers were actually found — for the same badge-honesty purpose. |

## Reload timing (settled, don't relitigate without checking ARCHITECTURE.md §11 first)

**Bootstrap-only.** `Build`/`BuildWithReport` are called where the agent is constructed: app start, project switch, model/provider switch — never per turn. Editing `AETOX.md` mid-session has no effect until one of those happens. This was a deliberate choice (owner: "หลายๆที่ทำก็แบบนั้น" — matches convention elsewhere), not an oversight — a per-turn mtime-check upgrade path is documented in §11 if it's ever needed, but isn't built.

## Rules of thumb

- New layer (e.g. a sub-agent profile prompt) = new function here, not a third copy in a front end.
- Keep layers append-only and ordered least-to-most-specific — that ordering is the actual conflict-resolution mechanism, not a stylistic choice.
