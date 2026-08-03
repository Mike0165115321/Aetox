# internal/skill — tool registry + dispatcher (35 built-ins, 32 model-facing)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Deep dive: [model-control-layer-2026-07-22.md](../../docs/architecture/model-control-layer-2026-07-22.md) · MCP direction: `MCP-SUPPORT-PLAN.md` (local working note, not published)

**What it is:** everything the agent can *do*. Defines the `Skill`/`Tool` interfaces, the `Registry` (which skills exist, with source tracking), the `Dispatcher` (text command → skill, and model tool-call → skill), and all 35 built-in tools — 32 of which implement `Tool` and are sent to the model; `echo`, `fs` and `help` are console-only.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Skill` + `Tool` interfaces ([skill.go](skill.go)) | A skill optionally implements `Tool` to expose a JSON-schema `ToolDefinition` to the model. **Already MCP-shaped** — an MCP client would adapt remote tools to exactly this interface. |
| `RegistryOptions` ([defaults.go](defaults.go)) | How the host configures built-ins. `SandboxRoot` for everyone; `Speech` for `audio_transcribe` (engine + model file); `Digest` for `web_fetch` (a func, not a provider — §52 — and nil is supported, meaning the tool returns whole pages); `Vision` for `read`, which decides whether it hands back the picture or names `image_ocr` instead (ARCHITECTURE.md §51). **This is the only path user settings have into a built-in skill** — a new configurable skill adds a field here (ARCHITECTURE.md §33). |
| `Registry` + `Source` ([skill.go](skill.go), [defaults.go](defaults.go)) | `NewDefaultRegistry(RegistryOptions{...})` builds the 35 built-ins. `Register(skill, Source)` rejects name collisions (fixed 2026-07-21 — used to silently overwrite). `SourceExternal` marks desktop workbench tools / discovered / future MCP tools. |
| `Dispatcher` ([dispatcher.go](dispatcher.go)) | Two doors, same tools: `Execute(ctx, line)` for text commands, `ExecuteTool(ctx, name, args)` + `ToolDefinitions()` for the model loop. Snapshots the registry at construction — register everything *before* `NewDispatcher`. |
| `RegisterDiscovered` ([discovery.go](discovery.go)) | Loads user-dropped skill definitions from `DefaultDiscoveryPaths()`. |

## The 35 built-ins

File ops ([read.go](read.go) (numbers every line like `cat -n`, `offset`/`limit` paging, and returns the image itself when `Vision` is on — ARCHITECTURE.md §50/§53.2), [write.go](write.go), [edit.go](edit.go) (exact search & replace, uniqueness-checked, `replace_all` — ARCHITECTURE.md §15/§50), [apply_patch.go](apply_patch.go), [notebook.go](notebook.go) (`notebook_edit`; `read` renders an `.ipynb` as the numbered cells this tool takes — ARCHITECTURE.md §54), [delete.go](delete.go), [list.go](list.go), [fs.go](fs.go)) · search ([grep.go](grep.go) regex content search with `output_mode`/`head_limit`/`offset`, [glob.go](glob.go) paths by pattern, newest first, with `head_limit`/`offset` — ARCHITECTURE.md §50) · what a language server knows ([diagnostics.go](diagnostics.go) — errors, takes a file or a folder; [symbol.go](symbol.go) — what an identifier is and where it is declared, by name rather than by line and column, ARCHITECTURE.md §55) · [shell.go](shell.go) and [shell_background.go](shell_background.go) (`run_in_background` plus `shell_output`/`shell_kill`, because a dev server or a watch build cannot finish inside a tool deadline — ARCHITECTURE.md §53.1) · [git.go](git.go) · [github_tools.go](github_tools.go) (`github_repo_summary`, `github_search`, `github_read_file`, `github_list_files`, and `plugin_install` — the half-finished plugin loader, see ARCHITECTURE.md §6.5) · [web_fetch.go](web_fetch.go), [web_search.go](web_search.go) · [echo.go](echo.go), [time.go](time.go), [help.go](help.go)


**The Office writers** — the three tools that hand back a file instead of a description of one, and the only ones here that emit binary: [sheet_write.go](sheet_write.go) (a real `.xlsx` — typed cells, so the numbers can be summed), [slides_write.go](slides_write.go) (a real `.pptx` — title, bullets, an embedded picture, speaker notes) and [doc_write.go](doc_write.go) (a real `.docx` — headings, paragraphs, bullet and numbered lists, tables). All three assemble parts through [internal/ooxml](../ooxml), which is one hand-written ZIP+XML container shared by all three formats — no dependency, and no AGPL question ever (ARCHITECTURE.md §75/§77). They are also the only tools that set `Output.Artifacts`, which is what puts an open button under the answer instead of leaving the file to be hunted for in the file tree (§76).

**Senses** — the four tools that exist because most models have none, all shelling out to a local binary, all returning text the model can read: [image_ocr.go](image_ocr.go) (tesseract; the fallback for a model that cannot see rather than the only way in, since ARCHITECTURE.md §51 — bundling: [tesseract doc](../../docs/architecture/tesseract-ocr-bundling-2026-07-22.md)) · [video_ocr.go](video_ocr.go) (ffmpeg frame sampling → the same tesseract path) · [pdf_read.go](pdf_read.go) (poppler's pdftotext; `read` refuses a PDF, it is a binary container) · [audio_transcribe.go](audio_transcribe.go) (ffmpeg → any engine in [internal/stt](../stt), ARCHITECTURE.md §31/§33 — the skill knows no engine by name, and the model is downloaded by the user, never bundled or auto-fetched; which model it runs on is the user's choice, pinned from Settings via `RegistryOptions.Speech` and stored as an absolute path since models legitimately live in Ollama's and LM Studio's folders too). The last two both emit `[m:ss] text`, deliberately: one clip can go through both and read as one transcript.

**Binaries that ship with Aetox** ([bundled.go](bundled.go)) — poppler and ffmpeg arrive as plain archives with no installer, so nothing puts them on PATH. The Windows installer unpacks each into the install directory ([project.nsi](../../desktop/build/windows/installer/project.nsi)) and `bundledBinary` looks there before falling back to PATH. Tesseract is not in this list: its own installer registers itself, so PATH finds it.

Desktop-only browser tools (`browser_open/read/click/type`) are **not** here — they live in [desktop/workbench.go](../../desktop/workbench.go) and register as `SourceExternal`. So the count the model actually sees in the desktop app is 41: these 32 (the other 3 built-ins — `echo`, `fs`, `help` — are CLI-only, with no `ToolDefinition`), plus 6 workbench (the four browser tools, `ask_user`, `todo_write`), plus 3 sub-agent (`task`, `task_result`, `task_answer`), plus whatever a connected MCP server brings.

## Rules of thumb

- New tool = one file here implementing `Skill` (+ `Tool` if the model should call it), registered in [defaults.go](defaults.go).
- **Five places, and forgetting each one breaks something different.** Learned the hard way adding `sheet_write`, `slides_write` and `doc_write` in one day; two of the five are guarded by tests and three are not.

  | wire it into | what happens if you don't |
  |---|---|
  | [defaults.go](defaults.go) | the model never sees the tool at all |
  | [safety.go](../safety/safety.go) | **the approval gate is skipped** — an unrecognised name is assessed `RiskLow` with no effects (see the rule below) |
  | [executor.go](../turn/executor.go) `toolCallToArgs` | the approval prompt has nothing to show the user |
  | [result.go](../turn/result.go) `shouldUseDeterministicToolSummary` | the receipt goes back through the LLM for a summary instead of being used as written |
  | [defaults_test.go](defaults_test.go) · [tool_coverage_test.go](../../desktop/tool_coverage_test.go) | tests fail — these two are the deliberate tripwires, and they work |
- **A tool that writes needs an entry in [safety.go](../safety/safety.go).** `internal/turn` gates every call, but it asks `AssessCommand` what the call *is*, and that function answers `RiskLow` with no effects for any skill name it does not recognise. A new file-producing tool without an entry therefore skips the approval prompt that `write`, `edit` and `delete` all pass through — silently, and only in the built product. This line used to say approval was not your job; it was wrong, and `sheet_write` was nearly the proof (ARCHITECTURE.md §75).
- **`Source` is not `Tool`.** Registering as `SourceBuiltin` puts a name on Settings' Tools page; only implementing `Tool` puts it in front of the model. `shell` and `git` sat on the wrong side of that gap for the life of the product (ARCHITECTURE.md §49) — `cliOnlySkills` in [desktop/tool_coverage_test.go](../../desktop/tool_coverage_test.go) pins whatever is deliberately hidden.
- Sandbox discipline: file tools resolve paths against `RegistryOptions.SandboxRoot` — keep it that way.
