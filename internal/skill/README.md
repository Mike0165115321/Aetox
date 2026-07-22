# internal/skill — tool registry + dispatcher (15 built-ins)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Deep dive: [model-control-layer-2026-07-22.md](../../docs/architecture/model-control-layer-2026-07-22.md) · MCP direction: [MCP-SUPPORT-PLAN.md](../../MCP-SUPPORT-PLAN.md)

**What it is:** everything the agent can *do*. Defines the `Skill`/`Tool` interfaces, the `Registry` (which skills exist, with source tracking), the `Dispatcher` (text command → skill, and model tool-call → skill), and all 15 built-in tools.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Skill` + `Tool` interfaces ([skill.go](skill.go)) | A skill optionally implements `Tool` to expose a JSON-schema `ToolDefinition` to the model. **Already MCP-shaped** — an MCP client would adapt remote tools to exactly this interface. |
| `Registry` + `Source` ([skill.go](skill.go), [defaults.go](defaults.go)) | `NewDefaultRegistry(RegistryOptions{SandboxRoot})` builds the 15 built-ins. `Register(skill, Source)` rejects name collisions (fixed 2026-07-21 — used to silently overwrite). `SourceExternal` marks desktop workbench tools / discovered / future MCP tools. |
| `Dispatcher` ([dispatcher.go](dispatcher.go)) | Two doors, same tools: `Execute(ctx, line)` for text commands, `ExecuteTool(ctx, name, args)` + `ToolDefinitions()` for the model loop. Snapshots the registry at construction — register everything *before* `NewDispatcher`. |
| `RegisterDiscovered` ([discovery.go](discovery.go)) | Loads user-dropped skill definitions from `DefaultDiscoveryPaths()`. |

## The 15 built-ins

File ops ([read.go](read.go), [write.go](write.go), [edit.go](edit.go) (exact search & replace, uniqueness-checked — ARCHITECTURE.md §15), [delete.go](delete.go), [list.go](list.go), [fs.go](fs.go)) · [grep.go](grep.go) (regex content search) · [shell.go](shell.go) · [git.go](git.go) · [github_tools.go](github_tools.go) (`github_repo_summary`, `plugin_install` — the half-finished plugin loader, see ARCHITECTURE.md §6.5) · [image_ocr.go](image_ocr.go) (tesseract — bundling: [tesseract doc](../../docs/architecture/tesseract-ocr-bundling-2026-07-22.md)) · [echo.go](echo.go), [time.go](time.go), [help.go](help.go), [input.go](input.go), [output.go](output.go)

Desktop-only browser tools (`browser_open/read/click/type`) are **not** here — they live in [desktop/workbench.go](../../desktop/workbench.go) and register as `SourceExternal`.

## Rules of thumb

- New tool = one file here implementing `Skill` (+ `Tool` if the model should call it), registered in [defaults.go](defaults.go). Approval/safety is **not** your job — `internal/turn` gates every call.
- Sandbox discipline: file tools resolve paths against `RegistryOptions.SandboxRoot` — keep it that way.
