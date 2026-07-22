# internal/grammar — input classification (the real one behind `command`)

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · First documented 2026-07-22 (was the least-documented 1,000-line file in the repo)

**What it is:** the single-file rules engine that decides what a raw input line *is* — conversation vs. skill command, slash command vs. plain text — before any model or tool sees it. **Nobody imports this package directly except `internal/command`**, which is a thin alias facade (`command.Kind = grammar.Kind`, delegating functions) — the rest of the engine imports `command`. Change behavior here; change the facade only when adding new exports.

## Key seams

| Seam | What it does |
|---|---|
| `Kind` (`KindConversation` / `KindSkill`) + `Intent` ([grammar.go](grammar.go)) | The classification result carried through the whole turn pipeline (`turn.Executor` branches on it — phases 1–4). `Intent` holds `Raw`, `Command`, `Args`, `IsSlash`, `IsMeta`. |
| `Parse(input, split, knownCommands)` | The classifier: tokenizes (via `SplitFunc`, normally `ParseTokens`), matches against the known-command set, handles `/slash` and meta/colon commands. |
| `BuildCommandSet(names)` | Skill names → the `knownCommands` set. Built from the dispatcher's registered skills (`app.buildCommandSetFromDispatcher`) — so what parses as a "command" tracks what's actually registered. |
| Slash helpers (`SlashSuggestions`, `SlashSuggestionCandidates`, `IsMetaSlashCommand`, `SlashMetaDescription/Legend`) | Power the CLI REPL's autocomplete and `/help` legend. |

## Rules of thumb

- Adding a skill does **not** require edits here — the command set is built from the registry at runtime.
- Adding a new *meta* slash command (a `/command` that isn't a skill) does — the meta lists live in this file.
- This layer runs before safety/approval; it must stay dumb-and-fast (string ops only, no I/O, no model calls).
