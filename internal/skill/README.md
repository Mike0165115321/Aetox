# internal/skill — tool registry + dispatcher (22 built-ins)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Deep dive: [model-control-layer-2026-07-22.md](../../docs/architecture/model-control-layer-2026-07-22.md) · MCP direction: [MCP-SUPPORT-PLAN.md](../../MCP-SUPPORT-PLAN.md)

**What it is:** everything the agent can *do*. Defines the `Skill`/`Tool` interfaces, the `Registry` (which skills exist, with source tracking), the `Dispatcher` (text command → skill, and model tool-call → skill), and all 22 built-in tools.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Skill` + `Tool` interfaces ([skill.go](skill.go)) | A skill optionally implements `Tool` to expose a JSON-schema `ToolDefinition` to the model. **Already MCP-shaped** — an MCP client would adapt remote tools to exactly this interface. |
| `RegistryOptions` ([defaults.go](defaults.go)) | How the host configures built-ins. `SandboxRoot` for everyone; `Speech` for `audio_transcribe` (engine + model file); `Digest` for `web_fetch` (a func, not a provider — §52 — and nil is supported, meaning the tool returns whole pages). **This is the only path user settings have into a built-in skill** — a new configurable skill adds a field here (ARCHITECTURE.md §33). |
| `Registry` + `Source` ([skill.go](skill.go), [defaults.go](defaults.go)) | `NewDefaultRegistry(RegistryOptions{...})` builds the 22 built-ins. `Register(skill, Source)` rejects name collisions (fixed 2026-07-21 — used to silently overwrite). `SourceExternal` marks desktop workbench tools / discovered / future MCP tools. |
| `Dispatcher` ([dispatcher.go](dispatcher.go)) | Two doors, same tools: `Execute(ctx, line)` for text commands, `ExecuteTool(ctx, name, args)` + `ToolDefinitions()` for the model loop. Snapshots the registry at construction — register everything *before* `NewDispatcher`. |
| `RegisterDiscovered` ([discovery.go](discovery.go)) | Loads user-dropped skill definitions from `DefaultDiscoveryPaths()`. |

## The 22 built-ins

File ops ([read.go](read.go), [write.go](write.go), [edit.go](edit.go) (exact search & replace, uniqueness-checked — ARCHITECTURE.md §15), [delete.go](delete.go), [list.go](list.go), [fs.go](fs.go)) · [grep.go](grep.go) (regex content search) · [shell.go](shell.go) · [git.go](git.go) · [github_tools.go](github_tools.go) (`github_repo_summary`, `github_search`, `github_read_file`, `github_list_files`, and `plugin_install` — the half-finished plugin loader, see ARCHITECTURE.md §6.5) · [web_fetch.go](web_fetch.go), [web_search.go](web_search.go) · [echo.go](echo.go), [time.go](time.go), [help.go](help.go), [input.go](input.go), [output.go](output.go)

**Senses** — the four tools that exist because most models have none, all shelling out to a local binary, all returning text the model can read: [image_ocr.go](image_ocr.go) (tesseract — bundling: [tesseract doc](../../docs/architecture/tesseract-ocr-bundling-2026-07-22.md)) · [video_ocr.go](video_ocr.go) (ffmpeg frame sampling → the same tesseract path) · [pdf_read.go](pdf_read.go) (poppler's pdftotext; `read` refuses a PDF, it is a binary container) · [audio_transcribe.go](audio_transcribe.go) (ffmpeg → any engine in [internal/stt](../stt), ARCHITECTURE.md §31/§33 — the skill knows no engine by name, and the model is downloaded by the user, never bundled or auto-fetched; which model it runs on is the user's choice, pinned from Settings via `RegistryOptions.Speech` and stored as an absolute path since models legitimately live in Ollama's and LM Studio's folders too). The last two both emit `[m:ss] text`, deliberately: one clip can go through both and read as one transcript.

**Binaries that ship with Aetox** ([bundled.go](bundled.go)) — poppler and ffmpeg arrive as plain archives with no installer, so nothing puts them on PATH. The Windows installer unpacks each into the install directory ([project.nsi](../../desktop/build/windows/installer/project.nsi)) and `bundledBinary` looks there before falling back to PATH. Tesseract is not in this list: its own installer registers itself, so PATH finds it.

Desktop-only browser tools (`browser_open/read/click/type`) are **not** here — they live in [desktop/workbench.go](../../desktop/workbench.go) and register as `SourceExternal`.

## Rules of thumb

- New tool = one file here implementing `Skill` (+ `Tool` if the model should call it), registered in [defaults.go](defaults.go). Approval/safety is **not** your job — `internal/turn` gates every call.
- **`Source` is not `Tool`.** Registering as `SourceBuiltin` puts a name on Settings' Tools page; only implementing `Tool` puts it in front of the model. `shell` and `git` sat on the wrong side of that gap for the life of the product (ARCHITECTURE.md §49) — `cliOnlySkills` in [desktop/tool_coverage_test.go](../../desktop/tool_coverage_test.go) pins whatever is deliberately hidden.
- Sandbox discipline: file tools resolve paths against `RegistryOptions.SandboxRoot` — keep it that way.
